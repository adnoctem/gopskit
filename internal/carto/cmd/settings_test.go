package cmd

import (
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestRenderSettingsPaths(t *testing.T) {
	for _, tt := range []struct {
		name, settings   string
		args             []string
		backbone, charts string
	}{
		{
			name:     "relative settings paths",
			settings: "backbone: infra/backbone.yaml\ncharts-dir: overlays\n",
			backbone: "config/infra/backbone.yaml", charts: "config/overlays",
		},
		{
			name:     "explicit default values are file relative",
			settings: "backbone: backbone.yaml\ncharts-dir: charts\n",
			backbone: "config/backbone.yaml", charts: "config/charts",
		},
		{
			name:     "unconfigured defaults remain invocation relative",
			settings: "environment: dev\n",
			backbone: "backbone.yaml", charts: "charts",
		},
		{
			name:     "unconfigured charts directory stays invocation relative",
			settings: "backbone: infra/backbone.yaml\n",
			backbone: "config/infra/backbone.yaml", charts: "charts",
		},
		{
			name:     "unconfigured backbone stays invocation relative",
			settings: "charts-dir: overlays\n",
			backbone: "backbone.yaml", charts: "config/overlays",
		},
		{
			name:     "CLI relative paths override settings",
			settings: "backbone: wrong.yaml\ncharts-dir: wrong\n",
			args:     []string{"--backbone", "cli/backbone.yaml", "--charts-dir", "cli-values"},
			backbone: "cli/backbone.yaml", charts: "cli-values",
		},
		{
			name:     "CLI backbone and file charts directory",
			settings: "backbone: wrong.yaml\ncharts-dir: overlays\n",
			args:     []string{"-b", "cli/backbone.yaml"},
			backbone: "cli/backbone.yaml", charts: "config/overlays",
		},
		{
			name:     "file backbone and CLI charts directory",
			settings: "backbone: infra/backbone.yaml\ncharts-dir: wrong\n",
			args:     []string{"--charts-dir", "cli-values"},
			backbone: "config/infra/backbone.yaml", charts: "cli-values",
		},
		{
			name:     "CLI explicit default values stay invocation relative",
			settings: "backbone: wrong.yaml\ncharts-dir: wrong\n",
			args:     []string{"-b", "backbone.yaml", "--charts-dir", "charts"},
			backbone: "backbone.yaml", charts: "charts",
		},
		{
			name:     "command-scoped settings override top level",
			settings: "backbone: wrong.yaml\ncharts-dir: wrong\nrender:\n  backbone: infra/backbone.yaml\n  charts-dir: overlays\n",
			backbone: "config/infra/backbone.yaml", charts: "config/overlays",
		},
		{
			name:     "command-scoped setting and top-level fallback",
			settings: "backbone: infra/backbone.yaml\ncharts-dir: wrong\nrender:\n  charts-dir: overlays\napply:\n  backbone: unused.yaml\n",
			backbone: "config/infra/backbone.yaml", charts: "config/overlays",
		},
		{
			name:     "CLI overrides command-scoped settings",
			settings: "render:\n  backbone: wrong.yaml\n  charts-dir: wrong\n",
			args:     []string{"-b", "cli/backbone.yaml", "--charts-dir", "cli-values"},
			backbone: "cli/backbone.yaml", charts: "cli-values",
		},
		{
			name:     "other command settings do not rebase defaults",
			settings: "apply:\n  backbone: unused.yaml\n  charts-dir: unused\n",
			backbone: "backbone.yaml", charts: "charts",
		},
		{
			name:     "parent paths in settings",
			settings: "backbone: ../infra/backbone.yaml\ncharts-dir: ../overlays\n",
			backbone: "infra/backbone.yaml", charts: "overlays",
		},
	} {
		t.Run(tt.name, func(t *testing.T) {
			a := newState(t)
			write(t, "config/carto.yaml", tt.settings)
			writeRenderInputs(t, tt.backbone, tt.charts)
			root := NewRootCommand(a)
			root.SetArgs(append([]string{"render", "example", "--config", "config/carto.yaml", "-o", "rendered.yaml"}, tt.args...))
			require.NoError(t, root.Execute())
			assertRenderInputs(t)
			command, _, err := root.Find([]string{"render"})
			require.NoError(t, err)
			backbone, err := command.Flags().GetString("backbone")
			require.NoError(t, err)
			charts, err := command.Flags().GetString("charts-dir")
			require.NoError(t, err)
			assert.Equal(t, tt.backbone, backbone)
			assert.Equal(t, tt.charts, charts)
			assert.Nil(t, a.Kube, "path resolution must stay offline")
		})
	}
}

// The backbone is in a different directory from both settings and values, so the
// global file proves that backbone-relative resolution still applies after rebasing.
func writeRenderInputs(t *testing.T, backbone, charts string) {
	t.Helper()
	write(t, backbone, "globals:\n  - path: globals.yaml\n    applies: ['*']\ncharts: {example: {namespace: example}}\n")
	write(t, filepath.Join(filepath.Dir(backbone), "globals.yaml"), "fromGlobal: true\n")
	write(t, filepath.Join(charts, "example", "values.yaml"), "fromBase: true\n")
	write(t, filepath.Join(charts, "example", "values.dev.yaml"), "fromOverlay: true\n")
}

func assertRenderInputs(t *testing.T) {
	t.Helper()
	output, err := os.ReadFile("rendered.yaml")
	require.NoError(t, err)
	assert.Equal(t, "fromBase: true\nfromGlobal: true\nfromOverlay: true\n", string(output))
}

func TestRenderAbsoluteSettingsAndCLIPaths(t *testing.T) {
	for _, cli := range []bool{false, true} {
		t.Run(fmt.Sprintf("CLI=%t", cli), func(t *testing.T) {
			a := newState(t)
			dir := t.TempDir()
			backbone, charts := filepath.Join(dir, "infra", "backbone.yaml"), filepath.Join(dir, "values")
			writeRenderInputs(t, backbone, charts)
			settings := fmt.Sprintf("backbone: %q\ncharts-dir: %q\n", backbone, charts)
			var flags []string
			if cli {
				settings = "backbone: wrong.yaml\ncharts-dir: wrong\n"
				flags = []string{"--backbone", backbone, "--charts-dir", charts}
			}
			settingsPath := write(t, filepath.Join(t.TempDir(), "carto.yaml"), settings)
			root := NewRootCommand(a)
			root.SetArgs(append([]string{"render", "example", "--config", settingsPath, "-o", "rendered.yaml"}, flags...))
			require.NoError(t, root.Execute())
			assertRenderInputs(t)
			command, _, err := root.Find([]string{"render"})
			require.NoError(t, err)
			assert.Equal(t, backbone, command.Flags().Lookup("backbone").Value.String())
			assert.Equal(t, charts, command.Flags().Lookup("charts-dir").Value.String())
		})
	}
}

func TestRenderDiscoveredSettingsPaths(t *testing.T) {
	for _, source := range []string{"local", "user", "explicit external", "none"} {
		t.Run(source, func(t *testing.T) {
			a := newState(t)
			var dir string
			switch source {
			case "local":
				dir = "."
				// Local discovery must win over a user file with unusable paths.
				write(t, filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "carto", "carto.yaml"), "backbone: wrong.yaml\n")
			case "user":
				dir = filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "carto")
			case "explicit external":
				dir = t.TempDir()
			}
			if source == "none" {
				writeRenderInputs(t, "backbone.yaml", "charts")
			} else {
				write(t, filepath.Join(dir, "carto.yaml"), "backbone: infra/backbone.yaml\nrender:\n  charts-dir: overlays\n")
				writeRenderInputs(t, filepath.Join(dir, "infra", "backbone.yaml"), filepath.Join(dir, "overlays"))
			}
			root := NewRootCommand(a)
			args := []string{"render", "example", "-o", "rendered.yaml"}
			if source == "explicit external" {
				args = append(args, "--config", filepath.Join(dir, "carto.yaml"))
			}
			root.SetArgs(args)
			require.NoError(t, root.Execute())
			assertRenderInputs(t)
		})
	}
}
