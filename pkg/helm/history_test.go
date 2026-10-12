package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/chart"
	"helm.sh/helm/v3/pkg/release"
)

// storeRevisions stores name's revisions in the given order, each with the given status.
func storeRevisions(t *testing.T, c *Client, name string, revisions map[int]release.Status, order ...int) {
	t.Helper()

	for _, v := range order {
		require.NoError(t, c.cfg.Releases.Create(&release.Release{
			Name:      name,
			Namespace: "default",
			Version:   v,
			Info:      &release.Info{Status: revisions[v]},
			Chart:     &chart.Chart{Metadata: &chart.Metadata{Name: name, Version: "1.0.0"}},
		}))
	}
}

func TestHistoryIsNewestFirstAndLimited(t *testing.T) {
	asrt := assert.New(t)
	c := newTestClient(t, "default")

	statuses := map[int]release.Status{1: release.StatusSuperseded, 2: release.StatusSuperseded, 3: release.StatusDeployed}
	storeRevisions(t, c, "app", statuses, 2, 3, 1)

	all, err := c.History("app", 0)
	require.NoError(t, err)
	require.Len(t, all, 3)
	asrt.Equal([]int{3, 2, 1}, []int{all[0].Version, all[1].Version, all[2].Version})

	two, err := c.History("app", 2)
	require.NoError(t, err)
	asrt.Equal([]int{3, 2}, []int{two[0].Version, two[1].Version}, "--max must keep the newest revisions")
}

func TestMissingReleaseIsNotFound(t *testing.T) {
	c := newTestClient(t, "default")

	_, err := c.History("missing", 1)
	assert.True(t, IsReleaseNotFound(err), "got %v", err)

	_, err = c.CurrentManifest("missing")
	assert.True(t, IsReleaseNotFound(err), "got %v", err)
}

func TestInstalled(t *testing.T) {
	asrt := assert.New(t)
	c := newTestClient(t, "default")

	installed, err := c.Installed("missing")
	require.NoError(t, err)
	asrt.False(installed)

	storeRevisions(t, c, "gone", map[int]release.Status{1: release.StatusDeployed, 2: release.StatusUninstalled}, 2, 1)
	installed, err = c.Installed("gone")
	require.NoError(t, err)
	asrt.False(installed, "a release whose newest revision is uninstalled isn't installed")

	storeRevisions(t, c, "live", map[int]release.Status{1: release.StatusSuperseded, 2: release.StatusDeployed}, 2, 1)
	installed, err = c.Installed("live")
	require.NoError(t, err)
	asrt.True(installed)
}

func TestHistory(t *testing.T) {
	t.Run("errors when the release doesn't exist", func(t *testing.T) {
		asrt := assert.New(t)

		c := newTestClient(t, "ns")
		_, err := c.History("release-a", 10)
		asrt.Error(err)
	})

	t.Run("returns revisions for an installed and upgraded release", func(t *testing.T) {
		asrt := assert.New(t)

		c := newTestClient(t, "ns")
		ch, err := c.LoadChart("testdata/testchart", "", "")
		asrt.NoError(err)

		_, err = c.Apply("release-a", ch, map[string]interface{}{"name": "v1"})
		asrt.NoError(err)
		_, err = c.Apply("release-a", ch, map[string]interface{}{"name": "v2"})
		asrt.NoError(err)

		revs, err := c.History("release-a", 10)
		asrt.NoError(err)
		asrt.Len(revs, 2)
	})
}
