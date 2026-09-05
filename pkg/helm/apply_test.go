package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
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
