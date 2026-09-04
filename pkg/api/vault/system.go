package vault

import (
	"context"

	"github.com/hashicorp/vault-client-go/schema"
)

// InitializeResponse is the decoded response of a successful Initialize call
type InitializeResponse struct {
	Keys            []string `json:"keys,omitempty"`
	KeysB64         []string `json:"keys_base64,omitempty"`
	RootToken       string   `json:"root_token,omitempty"`
	RecoveryKeys    []string `json:"recovery_keys,omitempty"`
	RecoveryKeysB64 []string `json:"recovery_keys_base64,omitempty"`
}

// SealStatus returns the Vault cluster's current seal/initialization status
func (c *Client) SealStatus(ctx context.Context) (*schema.SealStatusResponse, error) {
	resp, err := c.api.System.SealStatus(ctx)
	if err != nil {
		return nil, err
	}

	return &resp.Data, nil
}

// Initialize initializes a new Vault cluster, generating unseal (or recovery) keys and the
// initial root token
func (c *Client) Initialize(ctx context.Context, req schema.InitializeRequest) (*InitializeResponse, error) {
	resp, err := c.api.System.Initialize(ctx, req)
	if err != nil {
		return nil, err
	}

	var out InitializeResponse
	if err := decodeInto(resp.Data, &out); err != nil {
		return nil, err
	}

	return &out, nil
}

// Unseal submits a single unseal key share towards unsealing the Vault cluster
func (c *Client) Unseal(ctx context.Context, key string) (*schema.UnsealResponse, error) {
	resp, err := c.api.System.Unseal(ctx, schema.UnsealRequest{Key: key})
	if err != nil {
		return nil, err
	}

	return &resp.Data, nil
}

// AuthEnableMethod enables the given auth method at path
func (c *Client) AuthEnableMethod(ctx context.Context, path string, req schema.AuthEnableMethodRequest) error {
	_, err := c.api.System.AuthEnableMethod(ctx, path, req)
	return err
}

// AuthListEnabledMethods lists every currently-enabled auth method
func (c *Client) AuthListEnabledMethods(ctx context.Context) (map[string]interface{}, error) {
	resp, err := c.api.System.AuthListEnabledMethods(ctx)
	if err != nil {
		return nil, err
	}

	return resp.Data, nil
}

// MountsEnableSecretsEngine mounts a new secrets engine at path
func (c *Client) MountsEnableSecretsEngine(ctx context.Context, path string, req schema.MountsEnableSecretsEngineRequest) error {
	_, err := c.api.System.MountsEnableSecretsEngine(ctx, path, req)
	return err
}

// MountsListSecretsEngines lists every currently-mounted secrets engine
func (c *Client) MountsListSecretsEngines(ctx context.Context) (map[string]interface{}, error) {
	resp, err := c.api.System.MountsListSecretsEngines(ctx)
	if err != nil {
		return nil, err
	}

	return resp.Data, nil
}

// PoliciesWriteAclPolicy writes (or overwrites) the named ACL policy
func (c *Client) PoliciesWriteAclPolicy(ctx context.Context, name, policy string) error {
	_, err := c.api.System.PoliciesWriteAclPolicy(ctx, name, schema.PoliciesWriteAclPolicyRequest{
		Policy: policy,
	})
	return err
}

// PoliciesListAclPolicies lists every ACL policy currently configured
func (c *Client) PoliciesListAclPolicies(ctx context.Context) ([]string, error) {
	resp, err := c.api.System.PoliciesListAclPolicies(ctx)
	if err != nil {
		return nil, err
	}

	return resp.Data.Policies, nil
}

// PoliciesWritePasswordPolicy writes (or overwrites) the named password policy
func (c *Client) PoliciesWritePasswordPolicy(ctx context.Context, name, policy string) error {
	_, err := c.api.System.PoliciesWritePasswordPolicy(ctx, name, schema.PoliciesWritePasswordPolicyRequest{
		Policy: policy,
	})
	return err
}

// PoliciesListPasswordPolicies lists every password policy currently configured
func (c *Client) PoliciesListPasswordPolicies(ctx context.Context) ([]string, error) {
	resp, err := c.api.System.PoliciesListPasswordPolicies(ctx)
	if err != nil {
		return nil, err
	}

	return resp.Data.Keys, nil
}

// PoliciesGeneratePasswordFromPasswordPolicy generates a new password using the named password policy
func (c *Client) PoliciesGeneratePasswordFromPasswordPolicy(ctx context.Context, name string) (string, error) {
	resp, err := c.api.System.PoliciesGeneratePasswordFromPasswordPolicy(ctx, name)
	if err != nil {
		return "", err
	}

	return resp.Data.Password, nil
}
