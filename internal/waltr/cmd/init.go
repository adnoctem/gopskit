package cmd

import (
	"context"
	"fmt"

	"github.com/adnoctem/gopskit/internal/waltr/app"
	cmdutil "github.com/adnoctem/gopskit/internal/waltr/util"
	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/adnoctem/gopskit/pkg/core"
	"github.com/adnoctem/gopskit/pkg/tools"
	"github.com/hashicorp/hcl/v2/gohcl"
	"github.com/hashicorp/hcl/v2/hclparse"
	"github.com/hashicorp/vault-client-go/schema"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewInitCommand // assure type compatibility

// initOptions are the settings of `waltr init` that the initialisation itself depends on.
type initOptions struct {
	highAvailability bool
	threshold        int
	shares           int
	secretFile       string
}

// NewInitCommand creates the Option which injects the 'init' subcommand into
// the 'waltr' CLI application
func NewInitCommand(app *app.State) *cobra.Command {
	var (
		vf   vaultFlags
		opts initOptions
	)

	cmd := &cobra.Command{
		Use:     "initialize",
		Short:   "Initialize and unseal Vault",
		Aliases: []string{"init"},
		Long: `Initialize Vault and unseal every Vault Pod, storing the root token and keys for later commands.

The Pods are found by --label (in --namespace, or the whole cluster) and each is reached at its own
DNS name - e.g. vault-0.vault-internal.vault.svc - with the scheme and port of --vault-addr. Vault
must therefore run as a StatefulSet with a headless Service, as the official chart does. Unsealing
is skipped when the chart configures auto-unseal.`,
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			ctx := cmd.Context()

			// finding, preparing and addressing the Vault Pods needs the cluster
			if _, err := app.KubeClient(); err != nil {
				return err
			}

			environment, err := environmentOf(cmd)
			if err != nil {
				return err
			}
			label, _ := cmd.Flags().GetString("label")
			namespace, _ := cmd.Flags().GetString("namespace")

			if vf.addr == "" {
				return fmt.Errorf("vault's address is required to know its scheme and port: set --vault-addr, $VAULT_ADDR or vault-addr in the configuration file")
			}

			pods, err := cmdutil.Pods(app, namespace, label)
			if err != nil {
				return fmt.Errorf("could not retrieve Vault pods for label %s: %w", label, err)
			}

			vaultNamespace := namespace
			if vaultNamespace == "" {
				if vaultNamespace, err = cmdutil.EnsureNamespace(pods); err != nil {
					return fmt.Errorf("%w - set --namespace", err)
				}
			}

			needsUnseal, err := prepareVaultPods(ctx, app, pods, vaultNamespace)
			if err != nil {
				return err
			}

			leader, err := cmdutil.LeaderPod(app, pods, vaultNamespace, label)
			if err != nil {
				return err
			}

			reach := func(ctx context.Context, pod corev1.Pod) (func(), error) {
				addr, err := podAddress(pod, vf.addr)
				if err != nil {
					return nil, err
				}

				app.Log.Infof("reaching Vault Pod %s at %s", pod.Name, addr)
				return vf.dial(ctx, app, addr)
			}

			return initializeAndUnseal(ctx, app, environment, opts, *leader, pods, needsUnseal, reach)
		},
	}

	vf.add(cmd, false)
	cmd.Flags().BoolVar(&opts.highAvailability, "high-availability", false, "Ensure Vault is running in HA mode")
	cmd.Flags().IntVar(&opts.threshold, "threshold", 4, "The threshold of recovery keys required to unlock Vault")
	cmd.Flags().IntVar(&opts.shares, "shares", 7, "The amount of total recovery key shares Vault emits")
	cmd.Flags().StringVar(&opts.secretFile, "secret-file", "",
		"A Helm secrets plugin-encrypted file to inject the token into")

	return cmd
}

// prepareVaultPods waits for the Vault Pods, disables shell history in them and reports whether
// they need unsealing, which they don't when the chart configures auto-unseal.
func prepareVaultPods(ctx context.Context, app *app.State, pods []corev1.Pod, namespace string) (bool, error) {
	var configMap string

	for _, p := range pods {
		app.Log.Debugf("discovered Vault Pod: %s in namespace %s", p.Name, p.Namespace)
		for _, vol := range p.Spec.Volumes {
			if vol.Name == "config" && vol.ConfigMap != nil {
				configMap = vol.ConfigMap.Name
				break
			}
		}

		if err := cmdutil.WaitUntilRunning(app, p); err != nil {
			return false, err
		}
		if err := cmdutil.DisableAshHistory(app, p); err != nil {
			app.Log.Errorf("could not disable Ash Shell history for Pod: %s. Error: %v", p.Name, err)
		} else {
			app.Log.Infof("disabled Ash Shell history for Pod: %s", p.Name)
		}
	}

	// The official chart mounts the ConfigMap as 'config' (if the Helm values are set)
	// ref: https://github.com/hashicorp/vault-helm/blob/main/templates/_helpers.tpl#L187
	if configMap == "" {
		app.Log.Info("found no configuration ConfigMap for Vault - proceeding with default steps")
		return true, nil
	}

	cm, err := app.Kube.Client.CoreV1().ConfigMaps(namespace).Get(ctx, configMap, metav1.GetOptions{})
	if err != nil {
		return false, fmt.Errorf("vault has a 'config' volume but its ConfigMap %s can't be read: %w", configMap, err)
	}

	// auto-unseal cannot be configured without this key
	val, ok := cm.Data["extraconfig-from-values.hcl"]
	if !ok {
		return true, nil
	}

	var cfg cmdutil.VaultConfig
	// the parser needs a file name for its diagnostics
	f, diags := hclparse.NewParser().ParseHCL([]byte(val), "extraconfig-from-values.hcl")
	if diags.HasErrors() {
		return false, fmt.Errorf("cannot parse Vault HCL configuration: %v", diags)
	}
	if diags := gohcl.DecodeBody(f.Body, nil, &cfg); diags.HasErrors() {
		return false, fmt.Errorf("invalid Vault configuration: %v", diags)
	}

	if cfg.Seal != nil {
		app.Log.Info("found custom chart configuration enabling Auto-Unseal! skipping unseal steps")
		return false, nil
	}

	return true, nil
}

// initializeAndUnseal initialises Vault through leader if it isn't yet, storing the credentials,
// and unseals every Pod unless auto-unseal is configured. reach points app.Vault at a Pod and
// returns a function releasing the connection.
func initializeAndUnseal(
	ctx context.Context,
	app *app.State,
	environment core.Environment,
	opts initOptions,
	leader corev1.Pod,
	pods []corev1.Pod,
	needsUnseal bool,
	reach func(context.Context, corev1.Pod) (func(), error),
) error {
	release, err := reach(ctx, leader)
	if err != nil {
		return err
	}

	status, err := app.Vault.SealStatus(ctx)
	if err != nil {
		release()
		return fmt.Errorf("could not get Vault status: %w", err)
	}

	var creds *apivault.Credentials
	if !status.Initialized {
		creds, err = initialize(ctx, app, environment, opts)
		release()
		if err != nil {
			return err
		}
		app.Log.Infof("initialized Vault through Pod %s", leader.Name)
	} else {
		release()
		app.Log.Info("Vault is already initialized")

		if creds, err = cmdutil.ReadCredentials(app, environment); err != nil {
			return fmt.Errorf("could not read the Vault credentials: %w. Was Vault initialized with 'waltr'?", err)
		}
	}

	if !needsUnseal && status.Initialized {
		app.Log.Info("skipping Vault unseal - Auto-Unseal is enabled")
		return nil
	}

	if len(creds.Keys) < opts.threshold {
		return fmt.Errorf("the stored credentials hold %d unseal keys, fewer than the threshold of %d", len(creds.Keys), opts.threshold)
	}

	for _, p := range pods {
		if err := unsealPod(ctx, app, p, creds, opts.threshold, reach); err != nil {
			return err
		}
	}

	app.Log.Info("successfully initialized Vault")
	return nil
}

func initialize(ctx context.Context, app *app.State, environment core.Environment, opts initOptions) (*apivault.Credentials, error) {
	req := schema.InitializeRequest{
		SecretShares:    int32(opts.shares),
		SecretThreshold: int32(opts.threshold),
	}

	// HA requires the use of recovery keys, Shamir doesn't support it however
	if opts.highAvailability {
		req = schema.InitializeRequest{
			RecoveryShares:    int32(opts.shares),
			RecoveryThreshold: int32(opts.threshold),
		}
	}

	res, err := app.Vault.Initialize(ctx, req)
	if err != nil {
		return nil, fmt.Errorf("could not initialize Vault: %w", err)
	}

	creds := &apivault.Credentials{
		Keys:    res.Keys,
		KeysB64: res.KeysB64,
		Token:   res.RootToken,
	}

	// always keep a copy at the credential path, which later commands read
	if err := cmdutil.WriteCredentials(app, environment, creds); err != nil {
		return nil, fmt.Errorf("vault was initialized, but its credentials could not be stored: %w", err)
	}

	if opts.secretFile != "" {
		_, err := tools.AddSecretValue(opts.secretFile, map[string]interface{}{
			"vault": map[string]interface{}{"token": res.RootToken},
		}, false)
		if err != nil {
			app.Log.Errorf("could not add the Vault token to %s: %v", opts.secretFile, err)
		}
	} else {
		app.Log.Info("secret-file unset. only writing Vault Token to cache path!")
	}

	return creds, nil
}

func unsealPod(
	ctx context.Context,
	app *app.State,
	pod corev1.Pod,
	creds *apivault.Credentials,
	threshold int,
	reach func(context.Context, corev1.Pod) (func(), error),
) error {
	if err := cmdutil.WaitUntilRunning(app, pod); err != nil {
		return err
	}

	release, err := reach(ctx, pod)
	if err != nil {
		return err
	}
	defer release()

	for i := range threshold {
		status, err := app.Vault.SealStatus(ctx)
		if err != nil {
			return fmt.Errorf("could not get the seal status of Pod %s: %w", pod.Name, err)
		}

		if !status.Sealed {
			app.Log.Infof("Vault Pod %s is unsealed", pod.Name)
			return nil
		}

		if _, err := app.Vault.Unseal(ctx, creds.Keys[i]); err != nil {
			return fmt.Errorf("could not unseal Vault Pod %s: %w", pod.Name, err)
		}

		app.Log.Infof("unsealed Vault Pod %s with key %d of threshold %d", pod.Name, i+1, threshold)
	}

	status, err := app.Vault.SealStatus(ctx)
	if err != nil {
		return fmt.Errorf("could not get the seal status of Pod %s: %w", pod.Name, err)
	}
	if status.Sealed {
		return fmt.Errorf("vault Pod %s is still sealed after %d keys; is --threshold right?", pod.Name, threshold)
	}

	return nil
}
