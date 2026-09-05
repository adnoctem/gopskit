package util

import (
	"strings"

	"github.com/adnoctem/gopskit/internal/waltr/app"
	v1 "k8s.io/api/core/v1"
)

// DisableAshHistory does what it says...
func DisableAshHistory(a *app.State, pod v1.Pod) error {
	// remove existing history file
	deleteCmd := []string{"rm", "-rf", "/home/vault/.ash_history"}
	_, _, err := a.Kube.Exec(strings.Join(deleteCmd, " "), pod)
	if err != nil {
		return err
	}

	// link history to /dev/null
	linkCmd := []string{"ln", "-s", "/dev/null", "/home/vault/.ash_history"}
	_, _, err = a.Kube.Exec(strings.Join(linkCmd, " "), pod)
	if err != nil {
		return err
	}

	return nil
}
