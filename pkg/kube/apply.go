package kube

import (
	"context"
	"fmt"

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

	dc, err := dynamic.NewForConfig(c.Config)
	if err != nil {
		return err
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

	result, err = dc.Resource(schema).Get(context.Background(), opts.Name, getOpts)
	if err != nil {
		return err
	}

	exists := result.Object["metadata"] != nil
	if !exists {
		result, err = dc.Resource(schema).Create(context.Background(), resource, createOpts)
		if err != nil {
			return err
		}

		fmt.Printf("Created Kubernetes Resource: %v named %v\n", result.GetKind(), result.GetName())
		return nil
	}

	result, err = dc.Resource(schema).Update(context.Background(), resource, updateOpts)
	if err != nil {
		return err
	}

	fmt.Printf("Updated Kubernetes Resource: %v named %v\n", result.GetKind(), result.GetName())
	return nil
}
