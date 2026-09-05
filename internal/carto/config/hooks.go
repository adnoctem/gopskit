package config

import (
	"fmt"
	"strings"

	"github.com/fmjstudios/gopskit/pkg/kube"
	"github.com/fmjstudios/gopskit/pkg/log"
	"github.com/fmjstudios/gopskit/pkg/proc"
	corev1 "k8s.io/api/core/v1"
)

// RunHooks executes each of the given hook actions in order, dispatching "local" actions through
// exec (the project's subprocess runner) and "exec" actions into pod via kc. logger may be nil.
func RunHooks(hooks []HookAction, exec *proc.Executor, kc *kube.Client, pod corev1.Pod, logger *log.Logger) error {
	for _, h := range hooks {
		if err := h.Validate(); err != nil {
			return err
		}

		switch {
		case h.Local != "":
			out, err := exec.Execute([]string{h.Local})
			if err != nil {
				return fmt.Errorf("hook %q failed: %w", h.Local, err)
			}

			if logger != nil && len(out) > 0 && out[0] != "" {
				logger.Debugf("hook %q output: %s", h.Local, strings.TrimSpace(out[0]))
			}
		case h.Exec != "":
			stdout, stderr, err := kc.Exec(h.Exec, pod)
			if err != nil {
				return fmt.Errorf("hook %q failed: %w (stderr: %s)", h.Exec, err, strings.TrimSpace(stderr))
			}

			if logger != nil && stdout != "" {
				logger.Debugf("hook %q output: %s", h.Exec, strings.TrimSpace(stdout))
			}
		}
	}

	return nil
}
