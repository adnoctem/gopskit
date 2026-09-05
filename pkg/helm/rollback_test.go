package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRollback(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient(t, "ns")
	ch, err := c.LoadChart("testdata/testchart", "", "")
	asrt.NoError(err)

	_, err = c.Apply("release-a", ch, map[string]interface{}{"name": "v1"})
	asrt.NoError(err)
	_, err = c.Apply("release-a", ch, map[string]interface{}{"name": "v2"})
	asrt.NoError(err)

	asrt.NoError(c.Rollback("release-a", 1))

	manifest, err := c.CurrentManifest("release-a")
	asrt.NoError(err)
	asrt.Contains(manifest, "name: v1")
}
