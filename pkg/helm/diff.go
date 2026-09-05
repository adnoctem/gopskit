package helm

import (
	"fmt"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"k8s.io/utils/diff"
)

// CurrentManifest fetches the manifest of name's latest deployed release from the cluster
func (c *Client) CurrentManifest(name string) (string, error) {
	client := action.NewGet(c.cfg)

	rel, err := client.Run(name)
	if err != nil {
		return "", err
	}

	return rel.Manifest, nil
}

// RenderManifest performs a client-only dry-run install (Helm's `helm template` equivalent) to
// produce the manifest that would be applied for name/ch/vals, without touching the cluster.
func (c *Client) RenderManifest(name string, ch *chart.Chart, vals map[string]interface{}) (string, error) {
	client := action.NewInstall(c.cfg)
	client.ReleaseName = name
	client.Namespace = c.namespace
	client.DryRun = true
	client.ClientOnly = true
	client.Replace = true

	rel, err := client.Run(ch, vals)
	if err != nil {
		return "", fmt.Errorf("could not render chart manifest: %w", err)
	}

	return rel.Manifest, nil
}

// ManifestDiff renders a human-readable delta between two manifests. Callers wanting to ignore
// metadata noise should normalize both manifests beforehand.
func ManifestDiff(current, proposed string) string {
	return diff.StringDiff(current, proposed)
}
