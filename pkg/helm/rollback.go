package helm

import (
	"time"

	"helm.sh/helm/v3/pkg/action"
)

// Rollback reverts name to the given revision, recreating pods to ensure a clean state
func (c *Client) Rollback(name string, version int) error {
	client := action.NewRollback(c.cfg)
	client.Version = version
	client.Timeout = 5 * time.Minute
	client.Wait = true
	client.CleanupOnFail = true
	client.Recreate = true

	return client.Run(name)
}
