package cmd

import (
	"fmt"
	"path/filepath"

	"github.com/fmjstudios/gopskit/internal/carto/config"
	fs "github.com/fmjstudios/gopskit/pkg/fsi"
	apihelm "github.com/fmjstudios/gopskit/pkg/helm"
)

// DefaultChartsDir is the default directory carto looks in for per-chart values files
const DefaultChartsDir = "charts"

// chartValues resolves and deep-merges the values for a chart: matching global values files
// first (in backbone.yaml declaration order), then the chart's base values.yaml, then an
// optional values.<environment>.yaml overlay - each later file overrides the earlier ones.
func chartValues(cfg *config.Config, chartsDir, name, environment string) (map[string]interface{}, error) {
	files := cfg.GlobalsFor(name)

	base := filepath.Join(chartsDir, name, "values.yaml")
	if fs.CheckIfExists(base) {
		files = append(files, base)
	}

	if environment != "" {
		overlay := filepath.Join(chartsDir, name, fmt.Sprintf("values.%s.yaml", environment))
		if fs.CheckIfExists(overlay) {
			files = append(files, overlay)
		}
	}

	return apihelm.MergeValues(files...)
}

// chartConfig looks up name's configuration in cfg, returning a clear error if it's not declared
func chartConfig(cfg *config.Config, name string) (config.ChartConfig, error) {
	cc, ok := cfg.Charts[name]
	if !ok {
		return config.ChartConfig{}, fmt.Errorf("chart %q is not declared in the backbone configuration", name)
	}

	return cc, nil
}
