package helpers

import (
	"context"
	b64 "encoding/base64"
	"fmt"
	"math/rand/v2"
	"strings"

	"github.com/Luzifer/go-dhparam"
	"github.com/fmjstudios/gopskit/pkg/proc"
)

const (
	PassphraseDefaultCharset = "ABCDEFGHIJKLMNOPQRSTUVWXYZabcdefghijklmnopqrstuvwxyz0123456789"
	PassphraseDefaultLength  = 48
	DHParamDefaultBits       = 4096
)

type PassphraseConfig struct {
	Length  int
	CharSet string
}

type PassphraseOpt func(conf *PassphraseConfig)

func WithLength(length int) PassphraseOpt {
	return func(cfg *PassphraseConfig) {
		cfg.Length = length
	}
}

func WithCharSet(charset string) PassphraseOpt {
	return func(cfg *PassphraseConfig) {
		cfg.CharSet = charset
	}
}

func GeneratePassphrase(opts ...PassphraseOpt) string {
	var sb strings.Builder
	cfg := &PassphraseConfig{
		Length:  PassphraseDefaultLength,
		CharSet: PassphraseDefaultCharset,
	}

	// configure
	for _, o := range opts {
		o(cfg)
	}

	for i := 0; i < cfg.Length; i++ {
		sb.WriteByte(cfg.CharSet[rand.IntN(len(cfg.CharSet))])
	}

	return sb.String()
}

type Encoding int

const (
	Base64 Encoding = iota + 1
	Raw
)

type DiffieHellmanConfig struct {
	Bits     int
	Encoding Encoding
}

type DiffieHellmanOpt func(cfg *DiffieHellmanConfig)

func WithBits(bits int) DiffieHellmanOpt {
	return func(cfg *DiffieHellmanConfig) {
		cfg.Bits = bits
	}
}

func WithEncoding(encoding Encoding) DiffieHellmanOpt {
	return func(cfg *DiffieHellmanConfig) {
		cfg.Encoding = encoding
	}
}

func DefaultDiffieHellmanConfig() *DiffieHellmanConfig {
	return &DiffieHellmanConfig{
		Bits:     DHParamDefaultBits,
		Encoding: Raw,
	}
}

func GenerateDiffieHellmanParams(opts ...DiffieHellmanOpt) (string, error) {
	// var params string
	cfg := DefaultDiffieHellmanConfig()
	ctx, cancel := context.WithCancel(context.Background())

	// (re-)configure
	for _, o := range opts {
		o(cfg)
	}

	// allow CTRL+C
	go proc.AwaitCancel(func() int {
		cancel()
		return 0
	})

	raw, err := dhparam.GenerateWithContext(ctx, cfg.Bits, dhparam.GeneratorTwo, nil)
	if err != nil {
		return "", err
	}

	params, err := raw.ToPEM()
	if err != nil {
		return "", err
	}

	switch cfg.Encoding {
	case Base64:
		return b64.StdEncoding.EncodeToString(params), nil
	case Raw:
		return string(params), nil
	default:
		return "", fmt.Errorf("invalid DiffieHellman encoding: %v", cfg.Encoding)
	}
}
