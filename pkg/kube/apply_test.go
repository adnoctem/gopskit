package kube

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic/fake"
	"k8s.io/client-go/kubernetes/scheme"
	clienttesting "k8s.io/client-go/testing"
)

var configMapGVR = schema.GroupVersionResource{Group: "", Version: "v1", Resource: "configmaps"}

func configMapUnstructured(name string, data map[string]interface{}) *unstructured.Unstructured {
	return &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "v1",
			"kind":       "ConfigMap",
			"metadata": map[string]interface{}{
				"name":      name,
				"namespace": "ns",
			},
			"data": data,
		},
	}
}

func TestApplyCreatesAMissingResource(t *testing.T) {
	asrt := assert.New(t)

	dc := fake.NewSimpleDynamicClient(scheme.Scheme)
	c := &Client{Dynamic: dc}

	resource := configMapUnstructured("cm-a", map[string]interface{}{"k": "v1"})
	err := c.Apply(configMapGVR, resource, &ApplyOptions{Name: "cm-a"})
	asrt.NoError(err)

	got, err := dc.Resource(configMapGVR).Namespace("ns").Get(t.Context(), "cm-a", metav1.GetOptions{})
	asrt.NoError(err)
	asrt.Equal("cm-a", got.GetName())
}

func TestApplyUpdatesAnExistingResource(t *testing.T) {
	asrt := assert.New(t)

	existing := configMapUnstructured("cm-a", map[string]interface{}{"k": "old"})
	dc := fake.NewSimpleDynamicClient(scheme.Scheme, existing)
	c := &Client{Dynamic: dc}

	updated := configMapUnstructured("cm-a", map[string]interface{}{"k": "new"})
	err := c.Apply(configMapGVR, updated, &ApplyOptions{Name: "cm-a"})
	asrt.NoError(err)

	got, err := dc.Resource(configMapGVR).Namespace("ns").Get(t.Context(), "cm-a", metav1.GetOptions{})
	asrt.NoError(err)
	data, _, _ := unstructured.NestedMap(got.Object, "data")
	asrt.Equal("new", data["k"])
}

func TestApplyPropagatesARealGetError(t *testing.T) {
	asrt := assert.New(t)

	dc := fake.NewSimpleDynamicClient(scheme.Scheme)
	dc.PrependReactor("get", "configmaps", func(action clienttesting.Action) (bool, runtime.Object, error) {
		return true, nil, errors.New("connection refused")
	})
	c := &Client{Dynamic: dc}

	err := c.Apply(configMapGVR, configMapUnstructured("cm-a", nil), &ApplyOptions{Name: "cm-a"})
	asrt.Error(err)
	asrt.Contains(err.Error(), "connection refused")
}
