package tools

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestExecutableString(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal("kubectl", Kubectl.String())
	asrt.Equal("helm", Helm.String())
	asrt.Equal("helmfile", Helmfile.String())
	asrt.Equal("step-ca", StepCA.String())
	asrt.Equal("kustomize", Kustomize.String())
}

func TestExecutableIndex(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal(0, Kubectl.Index())
	asrt.Equal(1, Helm.Index())
}

func TestHelmPluginString(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal("diff", diff.String())
	asrt.Equal("secrets", secrets.String())
}

func TestHelmPluginIndex(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal(0, diff.Index())
	asrt.Equal(1, secrets.Index())
}

func TestParseGitHubURL(t *testing.T) {
	asrt := assert.New(t)

	owner, repo, err := parseGitHubURL("https://github.com/databus23/helm-diff")
	asrt.NoError(err)
	asrt.Equal("databus23", owner)
	asrt.Equal("helm-diff", repo)

	_, _, err = parseGitHubURL("https://gitlab.com/foo/bar")
	asrt.Error(err, "non-GitHub URL")

	_, _, err = parseGitHubURL("https://github.com/onlyowner")
	asrt.Error(err, "missing repo segment must not panic")

	_, _, err = parseGitHubURL("https://github.com/")
	asrt.Error(err, "empty owner/repo must not panic")
}

func TestVersionFromTag(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal("1.2.3", versionFromTag("v1.2.3"))
	asrt.Equal("1.2.3", versionFromTag("1.2.3"))
}

func TestTagFromVersion(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal("v1.2.3", tagFromVersion("1.2.3"))
	asrt.Equal("v1.2.3", tagFromVersion("v1.2.3"))
}
