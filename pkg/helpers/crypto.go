package helpers

import (
	"context"
	"crypto/rand"
	b64 "encoding/base64"
	"fmt"
	"math/big"
	"os"
	"os/signal"
	"strings"
	"syscall"

	"github.com/Luzifer/go-dhparam"
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

	// characters, not bytes, so a charset with non-ASCII characters yields whole characters
	chars := []rune(cfg.CharSet)
	if len(chars) == 0 {
		return ""
	}

	// crypto/rand: these passphrases encrypt volumes and CA keys. rand.Int draws uniformly, so
	// no character is more likely than another.
	limit := big.NewInt(int64(len(chars)))
	for range cfg.Length {
		n, err := rand.Int(rand.Reader, limit)
		if err != nil {
			// crypto/rand only fails when the operating system can't provide randomness at all
			panic(fmt.Sprintf("helpers: no randomness available: %v", err))
		}
		sb.WriteRune(chars[n.Int64()])
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
	cfg := DefaultDiffieHellmanConfig()

	// (re-)configure
	for _, o := range opts {
		o(cfg)
	}

	// generation can take minutes, so CTRL+C cancels it - and only it: the handler ends with the
	// generation, and an interruption is returned as an error rather than exiting successfully
	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	raw, err := dhparam.GenerateWithContext(ctx, cfg.Bits, dhparam.GeneratorTwo, nil)
	if err != nil {
		if ctx.Err() != nil {
			return "", fmt.Errorf("generating Diffie-Hellman parameters was interrupted: %w", ctx.Err())
		}
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
