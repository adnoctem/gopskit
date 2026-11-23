package helm

import (
	"errors"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"helm.sh/helm/v3/pkg/kube"
	kubefake "helm.sh/helm/v3/pkg/kube/fake"
)

func TestApply(t *testing.T) {
	t.Run("installs a brand-new release", func(t *testing.T) {
		asrt := assert.New(t)

		c := newTestClient(t, "ns")
		ch, err := c.LoadChart("testdata/testchart", "", "")
		asrt.NoError(err)

		rel, err := c.Apply("release-a", ch, map[string]interface{}{"name": "custom-name"})
		asrt.NoError(err)
		asrt.Equal("release-a", rel.Name)
		asrt.Equal(1, rel.Version)
	})

	t.Run("upgrades an existing release", func(t *testing.T) {
		asrt := assert.New(t)

		c := newTestClient(t, "ns")
		ch, err := c.LoadChart("testdata/testchart", "", "")
		asrt.NoError(err)

		_, err = c.Apply("release-a", ch, map[string]interface{}{"name": "v1"})
		asrt.NoError(err)

		rel, err := c.Apply("release-a", ch, map[string]interface{}{"name": "v2"})
		asrt.NoError(err)
		asrt.Equal(2, rel.Version)
		asrt.Contains(rel.Manifest, "name: v2")
	})
}

// Observe the actual Helm action's wait calls, including an atomic rollback's waits.
type recordingWaitClient struct {
	*kubefake.PrintingKubeClient
	jobs     []bool
	timeouts []time.Duration
	failNext bool
}

func (c *recordingWaitClient) record(jobs bool, timeout time.Duration) error {
	c.jobs = append(c.jobs, jobs)
	c.timeouts = append(c.timeouts, timeout)
	if c.failNext {
		c.failNext = false
		return errors.New("job wait failed")
	}
	return nil
}

func (c *recordingWaitClient) Wait(_ kube.ResourceList, timeout time.Duration) error {
	return c.record(false, timeout)
}

func (c *recordingWaitClient) WaitWithJobs(_ kube.ResourceList, timeout time.Duration) error {
	return c.record(true, timeout)
}

func recordWaits(c *Client) *recordingWaitClient {
	waiter := &recordingWaitClient{PrintingKubeClient: c.cfg.KubeClient.(*kubefake.PrintingKubeClient)}
	c.cfg.KubeClient = waiter
	return waiter
}

func TestApplyWaitOptions(t *testing.T) {
	for _, tt := range []struct {
		name    string
		opts    []ApplyOption
		jobs    bool
		timeout time.Duration
	}{
		{name: "defaults", jobs: true, timeout: 10 * time.Minute},
		{name: "disable Job waiting", opts: []ApplyOption{WithWaitForJobs(false), WithApplyTimeout(42 * time.Second)}, timeout: 42 * time.Second},
		{name: "custom Job timeout", opts: []ApplyOption{WithWaitForJobs(true), WithApplyTimeout(2 * time.Minute)}, jobs: true, timeout: 2 * time.Minute},
	} {
		t.Run(tt.name, func(t *testing.T) {
			c := newTestClient(t, "ns")
			waiter := recordWaits(c)
			ch, err := c.LoadChart("testdata/testchart", "", "")
			require.NoError(t, err)
			for range 2 { // install, then upgrade
				_, err = c.Apply("release-a", ch, nil, tt.opts...)
				require.NoError(t, err)
			}
			assert.Equal(t, []bool{tt.jobs, tt.jobs}, waiter.jobs)
			assert.Equal(t, []time.Duration{tt.timeout, tt.timeout}, waiter.timeouts)
		})
	}
}

func TestApplyRejectsNonPositiveTimeoutBeforeClusterAccess(t *testing.T) {
	for _, timeout := range []time.Duration{0, -time.Second} {
		_, err := (&Client{}).Apply("release-a", nil, nil, WithApplyTimeout(timeout))
		require.ErrorContains(t, err, "timeout must be greater than zero")
	}
}

func TestApplyJobWaitFailureIsAtomic(t *testing.T) {
	for _, upgrade := range []bool{false, true} {
		name := "install"
		if upgrade {
			name = "upgrade"
		}
		t.Run(name, func(t *testing.T) {
			c := newTestClient(t, "ns")
			waiter := recordWaits(c)
			ch, err := c.LoadChart("testdata/testchart", "", "")
			require.NoError(t, err)
			if upgrade {
				_, err = c.Apply("release-a", ch, map[string]interface{}{"name": "before"})
				require.NoError(t, err)
			}
			waiter.jobs, waiter.timeouts = nil, nil
			waiter.failNext = true
			_, err = c.Apply("release-a", ch, map[string]interface{}{"name": "after"}, WithApplyTimeout(37*time.Second))
			require.ErrorContains(t, err, "job wait failed")
			if upgrade {
				manifest, err := c.CurrentManifest("release-a")
				require.NoError(t, err)
				assert.Contains(t, manifest, "name: before")
				assert.Equal(t, []bool{true, true}, waiter.jobs)
				assert.Equal(t, []time.Duration{37 * time.Second, 37 * time.Second}, waiter.timeouts)
			} else {
				installed, err := c.Installed("release-a")
				require.NoError(t, err)
				assert.False(t, installed, "a failed atomic install must be removed")
			}
		})
	}
}
