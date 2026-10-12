package helm

import (
	"errors"
	"sort"

	"helm.sh/helm/v3/pkg/action"
	"helm.sh/helm/v3/pkg/release"
	"helm.sh/helm/v3/pkg/storage/driver"
)

// History returns name's release revisions, newest first: at most max of them, or all when max
// is zero or less.
//
// The sorting and limiting happen here because the Helm SDK does neither: action.History ignores
// its Max field, and the storage drivers return revisions in no particular order.
func (c *Client) History(name string, max int) ([]*release.Release, error) {
	releases, err := action.NewHistory(c.cfg).Run(name)
	if err != nil {
		return nil, err
	}

	sort.Slice(releases, func(i, j int) bool { return releases[i].Version > releases[j].Version })
	if max > 0 && len(releases) > max {
		releases = releases[:max]
	}

	return releases, nil
}

// IsReleaseNotFound reports whether err means the release doesn't exist, as opposed to it being
// unreadable, e.g. for lack of permissions or an unreachable cluster.
func IsReleaseNotFound(err error) bool {
	return errors.Is(err, driver.ErrReleaseNotFound)
}

// Installed reports whether name currently has a release that isn't uninstalled. Any error other
// than the release not existing is returned, rather than taken to mean "not installed".
func (c *Client) Installed(name string) (bool, error) {
	latest, err := c.History(name, 1)
	switch {
	case IsReleaseNotFound(err):
		return false, nil
	case err != nil:
		return false, err
	}

	return len(latest) > 0 && latest[0].Info.Status != release.StatusUninstalled, nil
}
