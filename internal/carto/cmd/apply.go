package cmd

import (
	"fmt"
	"sort"

	"github.com/adnoctem/gopskit/internal/carto/app"
	"github.com/adnoctem/gopskit/internal/carto/config"
	"github.com/adnoctem/gopskit/pkg/proc"
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
			configPath := proc.Must(cmd.Flags().GetString("backbone"))
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

			installed, err := hc.Installed(name)
			if err != nil {
				return fmt.Errorf("could not determine whether %q is installed: %w", name, err)
			}
			firstInstall := !installed

			pod, hasPod, err := releasePod(a, cc.Namespace, name)
			if err != nil {
				return err
			}

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
				if pod, hasPod, err = releasePod(a, cc.Namespace, name); err != nil {
					return err
				}
				if err := runHookSet(cc, config.HookFirstInstall, a, pod, hasPod); err != nil {
					return fmt.Errorf("first-install hooks failed: %w", err)
				}
			}

			if pod, hasPod, err = releasePod(a, cc.Namespace, name); err != nil {
				return err
			}
			if err := runHookSet(cc, config.HookPostApply, a, pod, hasPod); err != nil {
				return fmt.Errorf("post-apply hooks failed: %w", err)
			}

			return nil
		},
	}

	return cmd
}

// releasePod finds a running pod belonging to name's release, for "exec" hooks and the exec and
// logs commands - preferring a ready one, and never one that is shutting down. It returns
// hasPod=false if there is none (e.g. before first install) rather than erroring, since hook sets
// without an "exec" action don't need a pod at all.
func releasePod(a *app.State, namespace, name string) (corev1.Pod, bool, error) {
	kc, err := a.KubeClient()
	if err != nil {
		return corev1.Pod{}, false, err
	}

	pods, err := kc.Pods(namespace, metav1.ListOptions{
		LabelSelector: fmt.Sprintf("app.kubernetes.io/instance=%s", name),
	})
	if err != nil {
		return corev1.Pod{}, false, fmt.Errorf("could not list the pods of release %q: %w", name, err)
	}

	pod, ok := pickPod(pods)
	return pod, ok, nil
}

// pickPod chooses the pod to work with: a ready one if there is any, else a running one, and
// never one being deleted. Ties go to the alphabetically first name, so the choice is stable.
func pickPod(pods []corev1.Pod) (corev1.Pod, bool) {
	var ready, running []corev1.Pod
	for _, p := range pods {
		if p.DeletionTimestamp != nil || p.Status.Phase != corev1.PodRunning {
			continue
		}

		running = append(running, p)
		for _, c := range p.Status.Conditions {
			if c.Type == corev1.PodReady && c.Status == corev1.ConditionTrue {
				ready = append(ready, p)
				break
			}
		}
	}

	for _, candidates := range [][]corev1.Pod{ready, running} {
		if len(candidates) > 0 {
			sort.Slice(candidates, func(i, j int) bool { return candidates[i].Name < candidates[j].Name })
			return candidates[0], true
		}
	}

	return corev1.Pod{}, false
}

// runHookSet runs the named hook's actions from cc, if any are declared
func runHookSet(cc config.ChartConfig, hook string, a *app.State, pod corev1.Pod, hasPod bool) error {
	actions := cc.Hooks[hook]
	if len(actions) == 0 {
		return nil
	}

	for _, action := range actions {
		if len(action.Exec) > 0 && !hasPod {
			return fmt.Errorf("hook %q needs an 'exec' action but no release pod was found", hook)
		}
	}

	kc, err := a.KubeClient()
	if err != nil {
		return err
	}

	return config.RunHooks(actions, a.Exec, kc, pod, a.Log)
}
