package vault

import (
	"context"

	vault "github.com/hashicorp/vault-client-go"
	"github.com/hashicorp/vault-client-go/schema"
)

// KvV2Read reads a KV-v2 secret at path, mounted at mountPath
func (c *Client) KvV2Read(ctx context.Context, mountPath, path string) (map[string]interface{}, error) {
	resp, err := c.api.Secrets.KvV2Read(ctx, path, vault.WithMountPath(mountPath))
	if err != nil {
		return nil, err
	}

	return resp.Data.Data, nil
}

// KvV2Write writes (or overwrites) a KV-v2 secret at path, mounted at mountPath
func (c *Client) KvV2Write(ctx context.Context, mountPath, path string, data map[string]interface{}) error {
	_, err := c.api.Secrets.KvV2Write(ctx, path, schema.KvV2WriteRequest{Data: data}, vault.WithMountPath(mountPath))
	return err
}

// Read performs a generic (non-KV-v2) read against an arbitrary Vault path
func (c *Client) Read(ctx context.Context, path string) (map[string]interface{}, error) {
	resp, err := c.api.Read(ctx, path)
	if err != nil {
		return nil, err
	}

	return resp.Data, nil
}

// Write performs a generic (non-KV-v2) write against an arbitrary Vault path
func (c *Client) Write(ctx context.Context, path string, body map[string]interface{}) error {
	_, err := c.api.Write(ctx, path, body)
	return err
}

// IsNotFound reports whether err represents a 404 (not found) response from the Vault API
func IsNotFound(err error) bool {
	return vault.IsErrorStatus(err, 404)
}
