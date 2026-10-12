package helm

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRenderManifest(t *testing.T) {
	asrt := assert.New(t)

	c := newTestClient(t, "ns")
	ch, err := c.LoadChart("testdata/testchart", "", "")
	asrt.NoError(err)

	manifest, err := c.RenderManifest("release-a", ch, map[string]interface{}{"name": "custom-name"})
	asrt.NoError(err)
	asrt.Contains(manifest, "kind: ConfigMap")
	asrt.Contains(manifest, "name: custom-name")
}

func TestCurrentManifest(t *testing.T) {
	t.Run("errors when no release has been installed yet", func(t *testing.T) {
		asrt := assert.New(t)

		c := newTestClient(t, "ns")
		_, err := c.CurrentManifest("release-a")
		asrt.Error(err)
	})

	t.Run("returns the manifest of an installed release", func(t *testing.T) {
		asrt := assert.New(t)

		c := newTestClient(t, "ns")
		ch, err := c.LoadChart("testdata/testchart", "", "")
		asrt.NoError(err)

		_, err = c.Apply("release-a", ch, map[string]interface{}{"name": "custom-name"})
		asrt.NoError(err)

		manifest, err := c.CurrentManifest("release-a")
		asrt.NoError(err)
		asrt.Contains(manifest, "name: custom-name")
	})
}

func TestManifestDiff(t *testing.T) {
	asrt := assert.New(t)

	asrt.Empty(ManifestDiff("same", "same"))

	deployed := "kind: ConfigMap\nmetadata:\n  name: app\ndata:\n  replicas: \"1\"\n  log: info\n"
	rendered := "kind: ConfigMap\nmetadata:\n  name: app\ndata:\n  replicas: \"3\"\n  log: info\n"

	asrt.Equal(`--- deployed
+++ rendered
@@ -2,5 +2,5 @@
 metadata:
   name: app
 data:
-  replicas: "1"
+  replicas: "3"
   log: info
`, ManifestDiff(deployed, rendered), "a unified diff of only the changed line, with context")

	everything := ManifestDiff("", "kind: ConfigMap\n")
	asrt.Contains(everything, "+kind: ConfigMap", "against an empty manifest every line is an addition")
}
