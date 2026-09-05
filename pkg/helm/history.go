package helm

import (
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
)

// History returns up to max revisions of name's release history
func (c *Client) History(name string, max int) ([]*release.Release, error) {
	client := action.NewHistory(c.cfg)
	client.Max = max

	return client.Run(name)
}
