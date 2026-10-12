package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

const twoContexts = `apiVersion: v1
kind: Config
current-context: one
clusters:
- {name: one, cluster: {server: "https://one.example"}}
- {name: two, cluster: {server: "https://two.example"}}
users:
- {name: user, user: {token: t}}
contexts:
- {name: one, context: {cluster: one, user: user}}
- {name: two, context: {cluster: two, user: user}}
`

func TestKubeFlagsReachClient(t *testing.T) {
	path := filepath.Join(t.TempDir(), "kubeconfig")
	require.NoError(t, os.WriteFile(path, []byte(twoContexts), 0o600))

	api := &API{}
	cmd := &cobra.Command{Use: "cli", RunE: func(*cobra.Command, []string) error { return nil }}
	api.AddKubeFlags(cmd)
	cmd.SetArgs([]string{"--kubeconfig", path, "--context", "two"})
	require.NoError(t, cmd.Execute())

	kc, err := api.KubeClient()
	require.NoError(t, err)
	assert.Equal(t, "https://two.example", kc.Config.Host)
}
