package cmd

import (
	"fmt"

	"github.com/fmjstudios/gopskit/internal/carto/app"
	"github.com/fmjstudios/gopskit/internal/carto/config"
	"github.com/fmjstudios/gopskit/pkg/proc"
	"github.com/spf13/cobra"
	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

var _ app.CLIOpt = NewApplyCommand // assure type compatibility

// NewApplyCommand creates the Option which injects the 'apply' subcommand into the 'carto' CLI
func NewApplyCommand(a *app.State) *cobra.Command {
	cmd := &cobra.Command{
		Use:              "apply <chart>",
		Short:            "Render, then install or upgrade a chart",
		Long:             "Runs the full Render -> pre-apply hooks -> Upgrade/Install -> first-install/post-apply hooks loop for a chart",
		Args:             cobra.ExactArgs(1),
		TraverseChildren: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			name := args[0]
			configPath := proc.Must(cmd.Flags().GetString("config"))
			chartsDir := proc.Must(cmd.Flags().GetString("charts-dir"))
			environment := proc.Must(cmd.Flags().GetString("environment"))

			cfg, err := config.Load(configPath)
			if err != nil {
				return err
			}

			cc, err := chartConfig(cfg, name)
			if err != nil {
				return err
			}

			vals, err := chartValues(cfg, chartsDir, name, environment)
			if err != nil {
				return err
			}

			hc, err := a.HelmClient(cc.Namespace)
			if err != nil {
				return err
			}

			ch, err := hc.LoadChart(name, cc.Repository, cc.Version)
			if err != nil {
				return err
			}

			history, _ := hc.History(name, 1)
			firstInstall := len(history) == 0

			pod, hasPod := releasePod(a, cc.Namespace, name)

			if err := runHookSet(cc, config.HookPreApply, a, pod, hasPod); err != nil {
				return fmt.Errorf("pre-apply hooks failed: %w", err)
			}

			rel, err := hc.Apply(name, ch, vals)
			if err != nil {
				return fmt.Errorf("could not apply chart %q: %w", name, err)
			}
			a.Log.Infof("successfully applied chart %q (revision %d)", name, rel.Version)

			if firstInstall {
				// the pod may not have existed yet when we looked it up above
				pod, hasPod = releasePod(a, cc.Namespace, name)
				if err := runHookSet(cc, config.HookFirstInstall, a, pod, hasPod); err != nil {
					return fmt.Errorf("first-install hooks failed: %w", err)
				}
			}

			pod, hasPod = releasePod(a, cc.Namespace, name)
			if err := runHookSet(cc, config.HookPostApply, a, pod, hasPod); err != nil {
				return fmt.Errorf("post-apply hooks failed: %w", err)
			}

			return nil
		},
	}

	return cmd
}

// releasePod finds a pod belonging to name's release, for use by "exec" hooks. It returns
// hasPod=false if none could be found (e.g. before first install) rather than erroring, since
// hook sets without an "exec" action don't need a pod at all.
func releasePod(a *app.State, namespace, name string) (corev1.Pod, bool) {
	pods, err := a.Kube.Pods(namespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", name),
	})
	if err != nil || len(pods) == 0 {
		return corev1.Pod{}, false
	}

	return pods[0], true
}

// runHookSet runs the named hook's actions from cc, if any are declared
func runHookSet(cc config.ChartConfig, hook string, a *app.State, pod corev1.Pod, hasPod bool) error {
	actions := cc.Hooks[hook]
	if len(actions) == 0 {
		return nil
	}

	for _, action := range actions {
		if action.Exec != "" && !hasPod {
			return fmt.Errorf("hook %q needs an 'exec' action but no release pod was found", hook)
		}
	}

	return config.RunHooks(actions, a.Exec, a.Kube, pod, a.Log)
}
