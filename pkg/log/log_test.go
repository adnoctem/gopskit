package log

import (
	"testing"

	"github.com/stretchr/testify/assert"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

func TestConfig(t *testing.T) {
	asrt := assert.New(t)

	c := Config()
	asrt.Equal(zap.InfoLevel, c.Level.Level())
	asrt.True(c.Development)
	asrt.Equal("console", c.Encoding)
	asrt.Equal([]string{"stdout"}, c.OutputPaths)
	asrt.Equal([]string{"stderr"}, c.ErrorOutputPaths)
}

func TestNew(t *testing.T) {
	t.Run("builds a usable logger with default config", func(t *testing.T) {
		asrt := assert.New(t)

		l := New()
		asrt.NotNil(l.log)
		asrt.NotNil(l.SugaredLogger)
		asrt.NotNil(l.conf)
		asrt.Equal(zap.InfoLevel, l.conf.Level.Level())
	})

	t.Run("WithLevel overrides the configured log level", func(t *testing.T) {
		asrt := assert.New(t)

		l := New(WithLevel(zap.ErrorLevel))
		asrt.Equal(zap.ErrorLevel, l.conf.Level.Level())
	})

	t.Run("WithDevelopment sets Development mode", func(t *testing.T) {
		asrt := assert.New(t)

		l := New(WithDevelopment())
		asrt.True(l.conf.Development)
	})

	t.Run("WithEncoder overrides the encoder config", func(t *testing.T) {
		asrt := assert.New(t)

		enc := zapcore.EncoderConfig{MessageKey: "message"}
		l := New(WithEncoder(enc))
		asrt.Equal("message", l.conf.EncoderConfig.MessageKey)
	})

	t.Run("WithCustomConfig replaces the entire config", func(t *testing.T) {
		asrt := assert.New(t)

		custom := zap.Config{
			Level:            zap.NewAtomicLevelAt(zap.WarnLevel),
			Encoding:         "json",
			OutputPaths:      []string{"stdout"},
			ErrorOutputPaths: []string{"stderr"},
		}
		l := New(WithCustomConfig(custom))
		asrt.Equal(zap.WarnLevel, l.conf.Level.Level())
		asrt.Equal("json", l.conf.Encoding)
	})
}

func TestGlobalLogger(t *testing.T) {
	asrt := assert.New(t)

	asrt.NotNil(Global)
	asrt.NotNil(Global.SugaredLogger)
}
