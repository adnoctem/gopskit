package helm

import (
	"fmt"

	fs "github.com/fmjstudios/gopskit/pkg/fsi"
	"github.com/google/go-cmp/cmp"
	"github.com/imdario/mergo"
	"gopkg.in/yaml.v3"
)

// MergeValues deep-merges the given YAML values files in order, with later files overriding
// earlier ones - implementing the base+overlay precedence (globals, then a chart's base
// values.yaml, then an optional per-environment overlay). Callers are responsible for only
// passing files that exist; a missing optional overlay should simply be omitted from files.
func MergeValues(files ...string) (map[string]interface{}, error) {
	merged := map[string]interface{}{}

	for _, f := range files {
		raw, err := fs.Read(f)
		if err != nil {
			return nil, fmt.Errorf("could not read values file %q: %w", f, err)
		}

		var vals map[string]interface{}
		if err := yaml.Unmarshal(raw, &vals); err != nil {
			return nil, fmt.Errorf("could not parse values file %q: %w", f, err)
		}

		if err := mergo.Merge(&merged, vals, mergo.WithOverride); err != nil {
			return nil, fmt.Errorf("could not merge values file %q: %w", f, err)
		}
	}

	return merged, nil
}

// DiffValues renders a human-readable delta between two values maps
func DiffValues(oldVals, newVals map[string]interface{}) string {
	diff := cmp.Diff(oldVals, newVals)
	if diff == "" {
		return ""
	}

	return "Values mismatch (-old +new):\n" + diff
}
