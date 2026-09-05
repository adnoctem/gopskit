package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

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
