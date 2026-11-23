package cmd

import (
	"encoding/json"
	"fmt"
	"io"

	"github.com/adnoctem/gopskit/internal/carto/config"
	"gopkg.in/yaml.v3"
	applyv1 "k8s.io/client-go/applyconfigurations/core/v1"
)

// previewNamespaces shows intent only; it makes no Kubernetes requests and cannot
// determine live drift or ownership conflicts. Render remains values-only.
func previewNamespaces(out io.Writer, namespaces config.Namespaces) error {
	if len(namespaces) == 0 {
		return nil
	}
	if _, err := fmt.Fprintln(out, "# Declared namespaces reconciled before apply (desired configuration; not a live namespace diff)"); err != nil {
		return err
	}
	for _, meta := range namespaces {
		intent := applyv1.Namespace(meta.Name).WithLabels(meta.Labels).WithAnnotations(meta.Annotations)
		// Honor Kubernetes JSON tags and then emit sorted YAML, without the empty
		// spec/status/server metadata that marshaling a full Namespace would add.
		raw, err := json.Marshal(intent)
		if err != nil {
			return err
		}
		var document map[string]any
		if err := yaml.Unmarshal(raw, &document); err != nil {
			return err
		}
		rendered, err := yaml.Marshal(document)
		if err != nil {
			return err
		}
		if _, err := fmt.Fprintf(out, "---\n%s", rendered); err != nil {
			return err
		}
	}
	_, err := fmt.Fprintln(out, "# Helm chart manifest diff")
	return err
}
