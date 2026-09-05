package helpers

import (
	"encoding/base64"
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGeneratePassphrase(t *testing.T) {
	t.Run("uses the default length and charset", func(t *testing.T) {
		asrt := assert.New(t)

		p := GeneratePassphrase()
		asrt.Len(p, PassphraseDefaultLength)
		for _, r := range p {
			asrt.Contains(PassphraseDefaultCharset, string(r))
		}
	})

	t.Run("honors WithLength and WithCharSet", func(t *testing.T) {
		asrt := assert.New(t)

		p := GeneratePassphrase(WithLength(8), WithCharSet("x"))
		asrt.Equal("xxxxxxxx", p)
	})

	t.Run("produces different output across calls", func(t *testing.T) {
		asrt := assert.New(t)

		a := GeneratePassphrase(WithLength(64))
		b := GeneratePassphrase(WithLength(64))
		asrt.NotEqual(a, b)
	})
}

func TestGenerateDiffieHellmanParams(t *testing.T) {
	t.Run("defaults to raw PEM encoding", func(t *testing.T) {
		asrt := assert.New(t)

		params, err := GenerateDiffieHellmanParams(WithBits(64))
		asrt.NoError(err)
		asrt.True(strings.Contains(params, "BEGIN DH PARAMETERS"))
	})

	t.Run("honors WithEncoding(Base64)", func(t *testing.T) {
		asrt := assert.New(t)

		params, err := GenerateDiffieHellmanParams(WithBits(64), WithEncoding(Base64))
		asrt.NoError(err)

		decoded, err := base64.StdEncoding.DecodeString(params)
		asrt.NoError(err)
		asrt.Contains(string(decoded), "BEGIN DH PARAMETERS")
	})

	t.Run("rejects an unknown encoding", func(t *testing.T) {
		asrt := assert.New(t)

		_, err := GenerateDiffieHellmanParams(WithBits(64), WithEncoding(Encoding(99)))
		asrt.Error(err)
	})
}
