package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestDeepMergeMap(t *testing.T) {
	t.Run("overwrites and adds primitive keys", func(t *testing.T) {
		asrt := assert.New(t)

		dst := map[string]interface{}{"a": 1, "b": "keep"}
		src := map[string]interface{}{"a": 2, "c": "new"}

		asrt.NoError(DeepMergeMap(dst, src))
		asrt.Equal(map[string]interface{}{"a": 2, "b": "keep", "c": "new"}, dst)
	})

	t.Run("merges nested maps without dropping unrelated keys", func(t *testing.T) {
		asrt := assert.New(t)

		dst := map[string]interface{}{
			"nested": map[string]interface{}{"x": 1, "keep": true},
		}
		src := map[string]interface{}{
			"nested": map[string]interface{}{"x": 2},
		}

		asrt.NoError(DeepMergeMap(dst, src))
		asrt.Equal(map[string]interface{}{
			"nested": map[string]interface{}{"x": 2, "keep": true},
		}, dst)
	})

	t.Run("creates a nested map when dst has none", func(t *testing.T) {
		asrt := assert.New(t)

		dst := map[string]interface{}{}
		src := map[string]interface{}{"nested": map[string]interface{}{"x": 1}}

		asrt.NoError(DeepMergeMap(dst, src))
		asrt.Equal(map[string]interface{}{"nested": map[string]interface{}{"x": 1}}, dst)
	})

	t.Run("concatenates slice values instead of nesting them", func(t *testing.T) {
		asrt := assert.New(t)

		dst := map[string]interface{}{"list": []interface{}{1, 2}}
		src := map[string]interface{}{"list": []interface{}{3, 4}}

		asrt.NoError(DeepMergeMap(dst, src))
		asrt.Equal(map[string]interface{}{"list": []interface{}{1, 2, 3, 4}}, dst)
	})

	t.Run("sets a slice value directly when dst has no existing slice", func(t *testing.T) {
		asrt := assert.New(t)

		dst := map[string]interface{}{}
		src := map[string]interface{}{"list": []interface{}{1, 2}}

		asrt.NoError(DeepMergeMap(dst, src))
		asrt.Equal(map[string]interface{}{"list": []interface{}{1, 2}}, dst)
	})
}

func TestReplaceRecursive(t *testing.T) {
	t.Run("templates flat primitive values with their dotted key path", func(t *testing.T) {
		asrt := assert.New(t)

		input := map[string]interface{}{"foo": "bar"}
		out := make(map[string]interface{})

		ReplaceRecursive(input, nil, out, "")
		asrt.Equal(`{{ .Values | get "foo" "bar" }}`, out["foo"])
	})

	t.Run("recurses into nested maps building a dotted key path", func(t *testing.T) {
		asrt := assert.New(t)

		input := map[string]interface{}{
			"outer": map[string]interface{}{"inner": "value"},
		}
		out := make(map[string]interface{})

		ReplaceRecursive(input, nil, out, "")

		nested, ok := out["outer"].(map[string]interface{})
		asrt.True(ok)
		asrt.Equal(`{{ .Values | get "outer.inner" "value" }}`, nested["inner"])
	})

	t.Run("treats an empty nested map as a leaf value", func(t *testing.T) {
		asrt := assert.New(t)

		input := map[string]interface{}{"empty": map[string]interface{}{}}
		out := make(map[string]interface{})

		ReplaceRecursive(input, nil, out, "")
		// an empty map isn't nil, so it falls through to %v formatting rather
		// than the nil-to-empty-string substitution below
		asrt.Equal(`{{ .Values | get "empty" "map[]" }}`, out["empty"])
	})

	t.Run("treats a nil value as an empty string", func(t *testing.T) {
		asrt := assert.New(t)

		input := map[string]interface{}{"nullish": nil}
		out := make(map[string]interface{})

		ReplaceRecursive(input, nil, out, "")
		asrt.Equal(`{{ .Values | get "nullish" "" }}`, out["nullish"])
	})

	t.Run("uses a custom template when given one", func(t *testing.T) {
		asrt := assert.New(t)

		input := map[string]interface{}{"foo": "bar"}
		out := make(map[string]interface{})

		ReplaceRecursive(input, nil, out, `%s=%v`)
		asrt.Equal("foo=bar", out["foo"])
	})
}

func TestSanitizeSlice(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal([]string{"a", "b"}, SanitizeSlice([]string{"a", "", "b", ""}))
	asrt.Equal([]string{}, SanitizeSlice([]string{"", ""}))
}

func TestCopyMap(t *testing.T) {
	asrt := assert.New(t)

	src := map[string]interface{}{
		"a": 1,
		"nested": map[string]interface{}{
			"b": 2,
		},
	}
	cp := CopyMap(src)
	asrt.Equal(src, cp)

	// mutating the copy's nested map must not affect the original
	cp["nested"].(map[string]interface{})["b"] = 99
	asrt.Equal(2, src["nested"].(map[string]interface{})["b"])
}
