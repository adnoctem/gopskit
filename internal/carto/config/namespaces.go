package config

import (
	"encoding/json"
	"fmt"

	"gopkg.in/yaml.v3"
	apivalidation "k8s.io/apimachinery/pkg/api/validation"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	metavalidation "k8s.io/apimachinery/pkg/apis/meta/v1/validation"
	"k8s.io/apimachinery/pkg/util/validation/field"
)

// Namespaces is a list of native Kubernetes metadata objects. Only name, labels and
// annotations are writable; Carto supplies the Namespace resource envelope itself.
type Namespaces []metav1.ObjectMeta

// UnmarshalYAML retains Kubernetes' JSON field names without allowing yaml.v3's
// string coercion to turn a boolean or number into a label/annotation value.
func (n *Namespaces) UnmarshalYAML(node *yaml.Node) error {
	var entries []map[string]any
	if err := node.Decode(&entries); err != nil {
		return fmt.Errorf("namespaces must be a list of metadata objects: %w", err)
	}
	result := make(Namespaces, 0, len(entries))
	seen := make(map[string]bool)
	for i, entry := range entries {
		path := field.NewPath("namespaces").Index(i)
		if entry == nil {
			return fmt.Errorf("%s must be a metadata object", path)
		}
		for key, value := range entry {
			switch key {
			case "name":
				if _, ok := value.(string); !ok {
					return fmt.Errorf("%s.name must be a string", path)
				}
			case "labels", "annotations":
				if value == nil {
					continue
				}
				values, ok := value.(map[string]any)
				if !ok {
					return fmt.Errorf("%s.%s must be a map of strings", path, key)
				}
				for k, v := range values {
					if _, ok := v.(string); !ok {
						return fmt.Errorf("%s.%s[%q] must be a string", path, key, k)
					}
				}
			default:
				return fmt.Errorf("%s.%s is not supported; use name, labels and annotations", path, key)
			}
		}
		// ObjectMeta uses JSON tags, not yaml.v3 tags. All input values have already
		// been checked above, including fields whose zero values would otherwise vanish.
		raw, err := json.Marshal(entry)
		if err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		var meta metav1.ObjectMeta
		if err := json.Unmarshal(raw, &meta); err != nil {
			return fmt.Errorf("%s: %w", path, err)
		}
		var errs field.ErrorList
		for _, message := range apivalidation.ValidateNamespaceName(meta.Name, false) {
			errs = append(errs, field.Invalid(path.Child("name"), meta.Name, message))
		}
		errs = append(errs, metavalidation.ValidateLabels(meta.Labels, path.Child("labels"))...)
		errs = append(errs, apivalidation.ValidateAnnotations(meta.Annotations, path.Child("annotations"))...)
		if err := errs.ToAggregate(); err != nil {
			return err
		}
		if seen[meta.Name] {
			return fmt.Errorf("%s.name: duplicate namespace %q", path, meta.Name)
		}
		seen[meta.Name] = true
		result = append(result, meta)
	}
	*n = result
	return nil
}
