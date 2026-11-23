package config

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func loadNamespaces(t *testing.T, declaration string) (*Config, error) {
	t.Helper()
	path := filepath.Join(t.TempDir(), "backbone.yaml")
	require.NoError(t, os.WriteFile(path, []byte("charts: {example: {namespace: undeclared}}\n"+declaration), 0600))
	return Load(path)
}

func TestNamespaceMetadata(t *testing.T) {
	cfg, err := loadNamespaces(t, `namespaces:
  - name: cert-manager
    labels: {adnoctem.co/env: dev, enabled: "true"}
    annotations: {description: "", example.org/note: development}
  - name: traefik
`)
	require.NoError(t, err)
	require.Len(t, cfg.Namespaces, 2)
	assert.Equal(t, "cert-manager", cfg.Namespaces[0].Name)
	assert.Equal(t, "true", cfg.Namespaces[0].Labels["enabled"])
	assert.Contains(t, cfg.Namespaces[0].Annotations, "description")
	assert.Equal(t, "development", cfg.Namespaces[0].Annotations["example.org/note"])
	assert.Equal(t, "traefik", cfg.Namespaces[1].Name)
	assert.Equal(t, "undeclared", cfg.Charts["example"].Namespace)
}

func TestNamespacesOptional(t *testing.T) {
	for _, declaration := range []string{"", "namespaces: []", "namespaces: null"} {
		cfg, err := loadNamespaces(t, declaration)
		require.NoError(t, err)
		assert.Empty(t, cfg.Namespaces)
	}
	for _, maps := range []string{"", "labels: {}", "labels: null", "annotations: {}", "annotations: null"} {
		cfg, err := loadNamespaces(t, "namespaces:\n  - name: example\n    "+maps+"\n")
		require.NoError(t, err)
		assert.Empty(t, cfg.Namespaces[0].Labels)
		assert.Empty(t, cfg.Namespaces[0].Annotations)
	}
}

func TestNamespacesRejectInvalidDeclarations(t *testing.T) {
	for _, input := range []string{
		"namespaces: example", "namespaces: {name: example}", "namespaces: [example]", "namespaces: [null]",
		"namespaces: [{}]", "namespaces: [{name: null}]", "namespaces: [{name: 1}]", "namespaces: [{name: ''}]",
		"namespaces: [{name: Uppercase}]", "namespaces: [{name: example.org}]", "namespaces: [{name: -bad}]",
		"namespaces: [{name: '" + strings.Repeat("a", 64) + "'}]",
		"namespaces: [{name: example}, {name: example}]", "namespaces: [{name: one, name: two}]",
		"namespaces: [{name: example, labels: {key: first, key: second}}]",
		"namespaces: [{name: example, labels: {key: true}}]", "namespaces: [{name: example, labels: {key: null}}]",
		"namespaces: [{name: example, annotations: {key: 123}}]", "namespaces: [{name: example, labels: []}]",
		"namespaces: [{name: example, labels: {1: value}}]", "namespaces: [{name: example, annotations: text}]",
		"namespaces: [{name: example, labels: {'bad/key/again': value}}]",
		"namespaces: [{name: example, labels: {key: 'has spaces'}}]",
		"namespaces: [{name: example, annotations: {'bad/key/again': value}}]",
		"namespaces: [{name: example, annotations: {note: '" + strings.Repeat("x", 262145) + "'}}]",
	} {
		t.Run(input[:min(len(input), 100)], func(t *testing.T) {
			_, err := loadNamespaces(t, input)
			require.Error(t, err)
		})
	}
	for _, key := range []string{"apiVersion", "kind", "metadata", "spec", "status", "namespace", "generateName",
		"uid", "resourceVersion", "generation", "creationTimestamp", "deletionTimestamp", "deletionGracePeriodSeconds",
		"managedFields", "ownerReferences", "finalizers", "unexpected"} {
		t.Run(key, func(t *testing.T) {
			_, err := loadNamespaces(t, "namespaces: [{name: example, "+key+": null}]")
			require.ErrorContains(t, err, "namespaces[0]."+key)
		})
	}
}
