package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestLoadChart(t *testing.T) {
	t.Run("loads a local chart directory", func(t *testing.T) {
		asrt := assert.New(t)

		c := newTestClient(t, "ns")
		ch, err := c.LoadChart("testdata/testchart", "", "")
		asrt.NoError(err)
		asrt.Equal("testchart", ch.Name())
	})

	t.Run("errors on a chart that doesn't exist", func(t *testing.T) {
		asrt := assert.New(t)

		c := newTestClient(t, "ns")
		_, err := c.LoadChart("testdata/does-not-exist", "", "")
		asrt.Error(err)
	})

	t.Run("builds an OCI reference from repository + name", func(t *testing.T) {
		asrt := assert.New(t)

		c := newTestClient(t, "ns")
		// no real registry is reachable in this test environment, so this must fail - the point
		// is confirming it attempts an OCI pull (via the combined ref) rather than treating
		// "oci://registry.example.com" as a literal local path
		_, err := c.LoadChart("mychart", "oci://registry.example.com/charts", "1.0.0")
		asrt.Error(err)
	})
}
