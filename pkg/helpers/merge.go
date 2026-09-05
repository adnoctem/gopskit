package helpers

import (
	"fmt"
	"strings"
)

type Primitive interface {
	bool | string | int | int8 | int16 | int32 | int64 | uint | uint8 | uint16 | uint32 | uint64 | float32 | float64 | complex64 | complex128
}

func DeepMergeMap(dst, src map[string]interface{}) error {
	for srcK, srcV := range src {

		// check if interface is another map
		if srcMap, ok := srcV.(map[string]interface{}); ok {
			if dstVal, ok := dst[srcK]; ok {
				if dstMap, ok := dstVal.(map[string]interface{}); ok {
					err := DeepMergeMap(dstMap, srcMap)
					if err != nil {
						return err
					}
					continue
				}
			} else {
				dst[srcK] = make(map[string]interface{})
			}
			err := DeepMergeMap(dst[srcK].(map[string]interface{}), srcMap)
			if err != nil {
				return err
			}
			// check if interface is a slice
		} else if srcSl, ok := srcV.([]interface{}); ok {
			if dstV, ok := dst[srcK]; ok {
				if dstSl, ok := dstV.([]interface{}); ok {
					dst[srcK] = append(dstSl, srcSl)
					continue
				}
			}
			dst[srcK] = srcSl
			// handle primitives
		} else {
			dst[srcK] = srcV
		}
	}
	return nil
}

const (
	Delimiter       string = "."
	ReplaceTemplate string = "{{ .Values | get \"%s\" \"%v\" }}"
)

var (
	BumpValues   = []string{"annotation", "label", "securitycontext", "affinity"}
	BumpTemplate = `  {{- if index .Values "%s" "chartValues"  | get "%s" "" }}
  {{ with index .Values "%s" "chartValues" | get "%s" }}
  annotations:
  {{- toYaml . | nindent %d }}
  {{- end }}
  {{- end }}`
)

// TODO(FMJdev): add bump stop values which act as base case so we do not template Kubernetes labels etc.
// Relying on recursion until we hit a primitive is highly error prone.
func ReplaceRecursive(input map[string]interface{}, keys []string, output map[string]interface{}, template string) {
	if template == "" {
		template = ReplaceTemplate
	}

	for k, v := range input {
		kp := make([]string, 0)
		kp = append(kp, keys...)
		if k != "" {
			kp = append(kp, k)
		}

		tpl := func(args ...any) string {
			return fmt.Sprintf(template, args...)
		}

		switch cur := v.(type) {
		case map[string]interface{}:
			// handle empty map
			if len(cur) == 0 {
				key := strings.Join(kp, Delimiter)
				// handle null values
				value := v
				if v == nil {
					value = ""
				}
				output[k] = tpl(key, value)
				continue
			}

			// it's a map, so  recurse
			output[k] = make(map[string]interface{})
			mRef := output[k].(map[string]interface{})
			ReplaceRecursive(cur, kp, mRef, template)
		default:
			key := strings.Join(kp, Delimiter)
			// handle null values
			value := v
			if v == nil {
				value = ""
			}
			output[k] = tpl(key, value)
		}
	}
}

func SanitizeSlice(slice []string) []string {
	result := make([]string, 0, len(slice))
	for _, v := range slice {
		if v != "" {
			result = append(result, v)
		}
	}

	return result
}

func CopyMap(m map[string]interface{}) map[string]interface{} {
	cp := make(map[string]interface{})
	for k, v := range m {
		vm, ok := v.(map[string]interface{})
		if ok {
			cp[k] = CopyMap(vm)
		} else {
			cp[k] = v
		}
	}

	return cp
}
