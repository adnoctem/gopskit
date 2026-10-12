package helm

import (
	"fmt"
	"strings"

	"github.com/pmezard/go-difflib/difflib"
	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
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

// ManifestDiff renders a unified diff from current to proposed - the deployed and the rendered
// manifest - with three lines of context, or an empty string if they're identical. Callers wanting
// to ignore metadata noise should normalize both manifests beforehand.
func ManifestDiff(current, proposed string) string {
	if current == proposed {
		return ""
	}

	diff, err := difflib.GetUnifiedDiffString(difflib.UnifiedDiff{
		A:        splitLines(current),
		B:        splitLines(proposed),
		FromFile: "deployed",
		ToFile:   "rendered",
		Context:  3,
	})
	if err != nil {
		// GetUnifiedDiffString only fails when writing to its in-memory buffer fails
		return fmt.Sprintf("could not compute the diff: %v", err)
	}

	return diff
}

// splitLines splits s into lines that keep their newline, as the diff expects. difflib's own
// SplitLines appends an empty line that shows up in the diff as a phantom line of context.
func splitLines(s string) []string {
	if s == "" {
		return nil
	}

	lines := strings.SplitAfter(s, "\n")
	if lines[len(lines)-1] == "" {
		lines = lines[:len(lines)-1]
	} else {
		// a last line without a newline still has to end one for the diff's layout
		lines[len(lines)-1] += "\n"
	}

	return lines
}
