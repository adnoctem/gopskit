package cmd

import (
	"context"
	"fmt"

	"github.com/fmjstudios/gopskit/internal/waltr/app"
	cmdutil "github.com/fmjstudios/gopskit/internal/waltr/util"
	apivault "github.com/fmjstudios/gopskit/pkg/api/vault"
	"github.com/fmjstudios/gopskit/pkg/core"
	"github.com/fmjstudios/gopskit/pkg/proc"
	"github.com/fmjstudios/gopskit/pkg/tools"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewInitCommand // assure type compatibility

// NewInitCommand creates the Option which injects the 'init' subcommand into
// the 'waltr' CLI application
func NewInitCommand(app *app.State) *cobra.Command {
	var (
		highAvailability bool
		threshold        int
		shares           int
		secretFile       string
		credentialFile   string
	)

	cmd := &cobra.Command{
		Use:              "initialize",
		Short:            "Initialize Vault",
		Aliases:          []string{"init"},
		Long:             "Initialize Vault runs in High Availability mode",
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			envF := proc.Must(cmd.Flags().GetString("environment"))
			label := proc.Must(cmd.Flags().GetString("label"))
			namespace := proc.Must(cmd.Flags().GetString("namespace"))
			environment := proc.Must(core.EnvFromString(envF))

			var needsUnseal, hasCustomConfig bool
			var vaultNamespace, customConfigName string
			var vaultLeaderPod *corev1.Pod
			var creds *apivault.Credentials

			// we unseal by default
			needsUnseal = true

			// look across the entire cluster
			pods, err := cmdutil.Pods(app, "", label)
			if err != nil {
				return fmt.Errorf("could not retrieve Vault pods for label: %s. Error: %v", label, err)
			}

			// ensure we're using a single namespace, or it's passed in
			vaultNamespace, err = cmdutil.EnsureNamespace(pods)
			if err != nil && namespace == "" {
				return fmt.Errorf("found multiple possible Vault pods. the namespace option is unset and %v", err)
			}

			for _, p := range pods {
				app.Log.Debugf("discovered Vault Pod: %s in namespace %s", p.Name, p.Namespace)
				for _, vol := range p.Spec.Volumes {
					if vol.Name == "config" {
						hasCustomConfig = true
						customConfigName = vol.ConfigMap.LocalObjectReference.Name
						break
					}
				}

				// pod has to run first
				if err := cmdutil.WaitUntilRunning(app, p); err != nil {
					return err
				}
				if err := cmdutil.DisableAshHistory(app, p); err != nil {
					app.Log.Errorf("could not disable Ash Shell history for Pod: %s. Error: %v", p.Name, err)
				}

				app.Log.Infof("successfully disabled Ash Shell history for Pod: %s", p.Name)
			}

			// The official chart mounts the ConfigMap as 'config' (if the Helm values are set)
			// ref: https://github.com/hashicorp/vault-helm/blob/main/templates/_helpers.tpl#L187
			if hasCustomConfig {
				cm, err := app.Kube.ConfigMap(vaultNamespace, customConfigName, metav1.GetOptions{
					TypeMeta: metav1.TypeMeta{
						APIVersion: "v1",
						Kind:       "ConfigMap",
					},
				})

				if err != nil {
					return fmt.Errorf("vault has custom 'config' volume but cannot find corresponding ConfigMap."+
						"Error: %v", err)
				}

				// auto-unseal cannot be configured without this key
				val, ok := cm.Data["extraconfig-from-values.hcl"]
				if ok {
					var cfg cmdutil.VaultConfig
					hcp := hclparse.NewParser()

					// ignore the 'example.hcl' it somehow needs a file name
					f, err := hcp.ParseHCL([]byte(val), "example.hcl")
					if err != nil {
						return fmt.Errorf("cannot parse Vault HCL configuration. Error: %v", err)
					}

					err = gohcl.DecodeBody(f.Body, nil, &cfg)
					if err != nil {
						return fmt.Errorf("invalid Vault configuration. Error: %v", err)
					}

					needsUnseal = cfg.Seal == nil
					if !needsUnseal {
						app.Log.Info(
							"found custom chart configuration enabling Auto-Unseal! skipping unseal steps")
					}
				}
			} else {
				app.Log.Info("found no configuration ConfigMaps for Vault - proceeding with default steps")
			}

			// wait until the pod is running
			vaultLeaderPod, err = cmdutil.LeaderPod(app, pods, vaultNamespace, label)
			if err != nil {
				return err
			}
			if err := cmdutil.WaitUntilRunning(app, *vaultLeaderPod); err != nil {
				return err
			}

			// port-forward the (leader)
			app.Log.Infof("Port-forwarding Vault Leader: %s", vaultLeaderPod.Name)
			ctx, cancel := context.WithCancel(context.Background())
			readyChan := make(chan struct{})
			go func(rc chan struct{}) {
				err := app.Kube.PortForward(ctx, *vaultLeaderPod, rc)
				if err != nil {
					panic(err)
				}
			}(readyChan)

			app.Log.Info("Waiting for port-forwarded Vault API to become available...")
			<-readyChan

			// get current status
			status, err := app.Vault.SealStatus(context.Background())
			if err != nil {
				cancel()
				return fmt.Errorf("could not get Vault status: %v", err)
			}

			// check for initialization
			if !status.Initialized {
				req := schema.InitializeRequest{
					SecretShares:    int32(shares),
					SecretThreshold: int32(threshold),
				}

				// HA requires the use of recovery keys, Shamir doesn't support it however
				if highAvailability {
					req = schema.InitializeRequest{
						RecoveryShares:    int32(shares),
						RecoveryThreshold: int32(threshold),
					}
				}

				initRes, err := app.Vault.Initialize(context.Background(), req)
				if err != nil {
					cancel()
					return fmt.Errorf("could not initialize Vault instance: %v", err)
				}

				app.Log.Infof("successfully initialized Vault instance: %s", vaultLeaderPod.Name)

				// determine Vault credentials
				creds = &apivault.Credentials{
					Keys:    initRes.Keys,
					KeysB64: initRes.KeysB64,
					Token:   initRes.RootToken,
				}

				// write Token somewhere where we can retrieve it
				if secretFile != "" {
					_, err := tools.AddSecretValue(secretFile, map[string]interface{}{
						"vault": map[string]interface{}{
							"token": initRes.RootToken,
						},
					}, false)

					if err != nil {
						app.Log.Errorf("could not add secret value to file: %s. Error: %v", secretFile, err)
					}
				} else {
					app.Log.Info("secret-file unset. only writing Vault Token to cache path!")
				}

				// always write backup json file to cache path
				err = cmdutil.WriteCredentials(app, environment, creds)
				if err != nil {
					cancel()
					return err
				}
			} else {
				app.Log.Info("skipping Vault initialization")
			}
			app.Log.Info("Shutting down Port-forward for Vault Leader Pod")
			cancel()

			// re-read credentials if we skipped initialization
			if status.Initialized {
				creds, err = cmdutil.ReadCredentials(app, environment)
				if err != nil {
					return fmt.Errorf("could not read Vault credentials: %v. Did you initialize Vault with 'waltr'",
						err)
				}
			}

			if err := app.Vault.SetToken(creds); err != nil {
				return fmt.Errorf("could not set Vault token: %v", err)
			}

			// unseal the pod(s) - if auto-unseal is not enabled or if we're not initialized yet
			if needsUnseal || !status.Initialized {
				for _, p := range pods {
					err := func() error {
						if err := cmdutil.WaitUntilRunning(app, p); err != nil {
							return err
						}

						ctx, loopCancel := context.WithCancel(context.Background())
						defer loopCancel()

						app.Log.Infof("starting Vault port-forward for pod: %s", p.Name)
						readyChan := make(chan struct{})
						go func(rc chan struct{}) {
							err := app.Kube.PortForward(ctx, p, rc)
							if err != nil {
								panic(err)
							}
						}(readyChan)

						app.Log.Info("waiting for port-forwarded Vault API to become available...")
						<-readyChan

						// unseal
						for i := 0; i < threshold; i++ {
							sealed, err := app.Vault.SealStatus(context.Background())
							if err != nil {
								return fmt.Errorf("could not get Vault status: %v", err)
							}

							if !sealed.Sealed {
								app.Log.Infof("skipping Vault unseal iteration: %d - Vault is unsealed", i)
								continue
							}

							_, err = app.Vault.Unseal(context.Background(), creds.Keys[i])
							if err != nil {
								return fmt.Errorf("could not unseal Vault instance: %s", p.Name)
							}

							app.Log.Infof("successfully unsealed Vault instance: %s with key %d of threshold %d",
								p.Name, i+1, threshold)
						}

						return nil
					}()

					if err != nil {
						return err
					}
				}
			} else {
				app.Log.Info("Skipping Vault unseal - Auto-Unseal is enabled.")
			}

			app.Log.Info("successfully initialized Vault")
			return nil
		},
	}

	cmd.PersistentFlags().BoolVar(&highAvailability, "high-availability", false, "Ensure Vault is running in HA mode")
	cmd.PersistentFlags().IntVar(&threshold, "threshold", 4, "The threshold of recovery keys required to unlock Vault")
	cmd.PersistentFlags().IntVar(&shares, "shares", 7, "The amount of total recovery key shares Vault emits")
	cmd.PersistentFlags().StringVar(&secretFile, "secret-file", "",
		"A Helm secrets plugin-encrypted file to inject the token into")
	cmd.PersistentFlags().StringVar(&credentialFile, "credential-file", "", "A custom filepath to store Vault credentials obtained via initialization")

	return cmd
}
