// Package config implements parsing for carto's backbone.yaml configuration file, which declares
// the set of Helm charts carto manages ("backbone components") along with their global values
// overlays and lifecycle hooks.
package config

import (
	"fmt"
	"path/filepath"

	fs "github.com/adnoctem/gopskit/pkg/fsi"
	"gopkg.in/yaml.v3"
)

// GlobalValues is a values file applied to every chart matched by Applies
type GlobalValues struct {
	// Path is the filesystem path to the global values file
	Path string `yaml:"path"`

	// Applies lists the chart names this file is merged into. "*" applies it to every chart.
	Applies []string `yaml:"applies"`
}

// AppliesTo reports whether this GlobalValues entry applies to the named chart
func (g GlobalValues) AppliesTo(chart string) bool {
	for _, a := range g.Applies {
		if a == "*" || a == chart {
			return true
		}
	}

	return false
}

// HookAction is a single lifecycle hook action - exactly one of Local or Exec must be set
type HookAction struct {
	// Local is a path to a local script to execute
	Local string `yaml:"local,omitempty"`

	// Exec is a command to run inside the chart's release pod
	Exec string `yaml:"exec,omitempty"`
}

// Validate ensures the HookAction has exactly one of Local or Exec set
func (h HookAction) Validate() error {
	if (h.Local == "") == (h.Exec == "") {
		return fmt.Errorf("hook action must set exactly one of 'local' or 'exec', got local=%q exec=%q", h.Local, h.Exec)
	}

	return nil
}

// Hook names recognized in a ChartConfig's Hooks map
const (
	HookPreApply     = "pre-apply"
	HookFirstInstall = "first-install"
	HookPostApply    = "post-apply"
)

// ChartConfig describes a single backbone component chart
type ChartConfig struct {
	// Version is the chart version to install/upgrade to
	Version string `yaml:"version"`

	// Repository is the chart's source - a traditional repo URL, an "oci://" registry
	// reference, or empty for a local chart path
	Repository string `yaml:"repository"`

	// Namespace is the Kubernetes namespace this chart is installed into
	Namespace string `yaml:"namespace"`

	// Hooks maps a hook name (HookPreApply/HookFirstInstall/HookPostApply) to the actions run
	// at that point in the apply lifecycle
	Hooks map[string][]HookAction `yaml:"hooks,omitempty"`
}

// Config is the parsed contents of a backbone.yaml file
type Config struct {
	// Globals are values files merged into charts matching their Applies list
	Globals []GlobalValues `yaml:"globals,omitempty"`

	// Charts maps a chart name to its configuration
	Charts map[string]ChartConfig `yaml:"charts"`
}

// Load reads and parses a backbone.yaml file at path
func Load(path string) (*Config, error) {
	raw, err := fs.Read(path)
	if err != nil {
		return nil, fmt.Errorf("could not read backbone config %q: %w", path, err)
	}

	var cfg Config
	if err := yaml.Unmarshal(raw, &cfg); err != nil {
		return nil, fmt.Errorf("could not parse backbone config %q: %w", path, err)
	}

	// resolve relative global values paths against the config file's own directory, not the
	// process's current working directory, so backbone.yaml stays portable regardless of where
	// it's invoked from
	dir := filepath.Dir(path)
	for i, g := range cfg.Globals {
		if !filepath.IsAbs(g.Path) {
			cfg.Globals[i].Path = filepath.Join(dir, g.Path)
		}
	}

	for name, chart := range cfg.Charts {
		for hook, actions := range chart.Hooks {
			for _, a := range actions {
				if err := a.Validate(); err != nil {
					return nil, fmt.Errorf("chart %q hook %q: %w", name, hook, err)
				}
			}
		}
	}

	return &cfg, nil
}

// GlobalsFor returns the paths of every global values file that applies to the named chart, in
// declaration order.
func (c *Config) GlobalsFor(chart string) []string {
	var paths []string
	for _, g := range c.Globals {
		if g.AppliesTo(chart) {
			paths = append(paths, g.Path)
		}
	}

	return paths
}
