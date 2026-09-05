package helm

import (
	"fmt"
	"strings"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/chart/loader"
)

// LoadChart resolves and loads a chart by name from repository (a traditional chart repository
// URL, an "oci://" registry reference, or empty for a local path) at the given version.
func (c *Client) LoadChart(name, repository, version string) (*chart.Chart, error) {
	cpo := action.ChartPathOptions{Version: version}

	ref := name
	switch {
	case strings.HasPrefix(repository, "oci://"):
		// OCI charts are addressed by their full registry path rather than a separate --repo
		// flag - fold the two together the way `helm install ... oci://host/repo/chart` does.
		ref = strings.TrimSuffix(repository, "/") + "/" + name
	case repository != "":
		cpo.RepoURL = repository
	}

	path, err := cpo.LocateChart(ref, c.settings)
	if err != nil {
		return nil, fmt.Errorf("could not locate chart %q: %w", ref, err)
	}

	ch, err := loader.Load(path)
	if err != nil {
		return nil, fmt.Errorf("could not load chart %q: %w", ref, err)
	}

	return ch, nil
}
