package app

import (
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGetAuthPath(t *testing.T) {
	asrt := assert.New(t)

	got := getAuthPath("/base/dir")
	asrt.Equal(filepath.Join("/base/dir", "creds", "keycloak-credentials.json"), got)
}
