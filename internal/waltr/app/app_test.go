package app

import (
	"testing"
	"time"

	apivault "github.com/adnoctem/gopskit/pkg/api/vault"
	"github.com/stretchr/testify/assert"
)

func TestWithVaultOpts(t *testing.T) {
	asrt := assert.New(t)

	a := &State{Vault: apivault.New("https://old-host:8200")}

	opt := WithVaultOpts("https://new-host:8200", apivault.WithTimeout(30*time.Second))
	opt(a)

	asrt.NotNil(a.Vault)
}
