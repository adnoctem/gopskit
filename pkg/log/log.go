package log

import (
	"fmt"
	"os"
	"sync"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

var (
	Global *Logger

	// DefaultConfig is the default configuration used for the zap-based Logger
	DefaultConfig = zap.Config{
		Level:             zap.NewAtomicLevelAt(zap.InfoLevel),
		Development:       true,
		DisableCaller:     false,
		DisableStacktrace: false,
		Sampling:          nil,
		Encoding:          "console",
		EncoderConfig: zapcore.EncoderConfig{
			MessageKey:  "msg",
			LevelKey:    "lvl",
			EncodeLevel: zapcore.CapitalColorLevelEncoder,
			EncodeTime:  zapcore.RFC3339TimeEncoder,
			TimeKey:     "time",
		},
		OutputPaths:      []string{"stdout"},
		ErrorOutputPaths: []string{"stderr"},
		// InitialFields: map[string]interface{}{
		// 	"date": time.Now().Format(time.RFC3339),
		// },
	}
)

// create a Global logger instance for use within packages
func init() {
	Global = New()
}

// Option is a utility function which is called during Logger initialization to alter or even override the DefaultConfig
type Option func(logger *Logger)

// Logger is a type alias to Uber's zap logger, saving us from importing zap everywhere
// and assuming the SugaredLogger's API results low effort logs later
type Logger struct {
	// log is the underlying zap Logger which is later embedded into
	// our Logger
	log *zap.Logger

	// conf is private zap.Config for the underlying instance within log
	conf *zap.Config

	// lock is a Mutex which ensures that only one goroutine may modify the configuration
	lock sync.Mutex

	// Logger embeds a pointer zap.SugaredLogger to assume its' API
	*zap.SugaredLogger
}

// Config builds a new, validated zap.Config using the DefaultConfig as its base
func Config() *zap.Config {
	c := DefaultConfig

	// assert that it builds
	if _, err := c.Build(); err != nil {
		fmt.Printf("could not create zap.Config for Logger: %v", err)
		os.Exit(1)
	}

	return &c
}

// New returns a newly built Logger including all or no Options for configuration
func New(opts ...Option) *Logger {
	l := &Logger{
		conf: Config(),
	}

	l.lock.Lock()
	for _, opt := range opts {
		opt(l)
	}
	l.lock.Unlock()

	lgr, err := l.conf.Build()
	if err != nil {
		fmt.Printf("could not build Config for Logger: %v", err)
		os.Exit(1)
	}

	return &Logger{
		log:           lgr,
		SugaredLogger: lgr.Sugar(),
	}
}

// WithCustomConfig overrides the entire DefaultConfig configuration, replacing the reference with a new zap.Config
// object which will be used to configure the Logger
func WithCustomConfig(cfg zap.Config) Option {
	return func(logger *Logger) {
		logger.conf = &cfg
	}
}

// WithLevel configures the Logger with a custom logging level
func WithLevel(level zapcore.Level) Option {
	return func(logger *Logger) {
		logger.conf.Level = zap.NewAtomicLevelAt(level)
	}
}

// WithEncoder configures a custom Encoder for the new Logger
func WithEncoder(encoder zapcore.EncoderConfig) Option {
	return func(logger *Logger) {
		logger.conf.EncoderConfig = encoder
	}
}

// WithDevelopment configures the new Logger for use within development contexts
func WithDevelopment() Option {
	return func(logger *Logger) {
		logger.conf.Development = true
	}
}
