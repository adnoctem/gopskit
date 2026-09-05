package kube

import (
	"context"
	"fmt"

	apierrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
)

type ApplyOptions struct {
	Name          string
	GetOptions    *metav1.GetOptions
	CreateOptions *metav1.CreateOptions
	UpdateOptions *metav1.UpdateOptions
}

func (c *Client) Apply(schema schema.GroupVersionResource, resource *unstructured.Unstructured, opts *ApplyOptions) error {
	var result *unstructured.Unstructured
	var err error

	// scope to the resource's own namespace when it has one - dc.Resource(schema) alone only
	// works for cluster-scoped resources (e.g. StorageClass); a namespaced one (the common case
	// for rendered manifests) needs the namespaced ResourceInterface or every call below 404s
	var ri dynamic.ResourceInterface = c.Dynamic.Resource(schema)
	if ns := resource.GetNamespace(); ns != "" {
		ri = c.Dynamic.Resource(schema).Namespace(ns)
	}

	getOpts := metav1.GetOptions{}
	if opts.GetOptions != nil {
		getOpts = *opts.GetOptions
	}

	createOpts := metav1.CreateOptions{}
	if opts.CreateOptions != nil {
		createOpts = *opts.CreateOptions
	}

	updateOpts := metav1.UpdateOptions{}
	if opts.UpdateOptions != nil {
		updateOpts = *opts.UpdateOptions
	}

	result, err = ri.Get(context.Background(), opts.Name, getOpts)
	if err != nil && !apierrors.IsNotFound(err) {
		return err
	}

	exists := err == nil && result.Object["metadata"] != nil
	if !exists {
		result, err = ri.Create(context.Background(), resource, createOpts)
		if err != nil {
			return err
		}

		fmt.Printf("Created Kubernetes Resource: %v named %v\n", result.GetKind(), result.GetName())
		return nil
	}

	result, err = ri.Update(context.Background(), resource, updateOpts)
	if err != nil {
		return err
	}

	fmt.Printf("Updated Kubernetes Resource: %v named %v\n", result.GetKind(), result.GetName())
	return nil
}
