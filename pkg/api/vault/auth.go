package vault

import (
	"context"

	vault "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

// KubernetesConfigureAuth configures the Kubernetes auth method's connection to the Kubernetes
// API server, where mountPath is the path the Kubernetes auth method was enabled at
func (c *Client) KubernetesConfigureAuth(ctx context.Context, mountPath string, req schema.KubernetesConfigureAuthRequest) error {
	_, err := c.api.Auth.KubernetesConfigureAuth(ctx, req, vault.WithMountPath(mountPath))
	return err
}

// KubernetesWriteAuthRole creates or updates a Kubernetes auth role
func (c *Client) KubernetesWriteAuthRole(ctx context.Context, name string, req schema.KubernetesWriteAuthRoleRequest) error {
	_, err := c.api.Auth.KubernetesWriteAuthRole(ctx, name, req)
	return err
}

// KubernetesListAuthRoles lists every configured Kubernetes auth role
func (c *Client) KubernetesListAuthRoles(ctx context.Context) ([]string, error) {
	resp, err := c.api.Auth.KubernetesListAuthRoles(ctx)
	if err != nil {
		return nil, err
	}

	return resp.Data.Keys, nil
}
