package core

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestEnvironmentString(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal("dev", Development.String())
	asrt.Equal("stage", Staging.String())
	asrt.Equal("prod", Production.String())
	asrt.Equal("unknown", Environment(0).String())
	asrt.Equal("unknown", Environment(99).String())
}

func TestEnvironmentIndex(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal(1, Development.Index())
	asrt.Equal(2, Staging.Index())
	asrt.Equal(3, Production.Index())
}

func TestEnvironmentPredicates(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(Development.IsDevelopment())
	asrt.False(Development.IsStaging())
	asrt.False(Development.IsProduction())

	asrt.True(Staging.IsStaging())
	asrt.False(Staging.IsDevelopment())
	asrt.False(Staging.IsProduction())

	asrt.True(Production.IsProduction())
	asrt.False(Production.IsDevelopment())
	asrt.False(Production.IsStaging())
}

func TestEnvFromString(t *testing.T) {
	asrt := assert.New(t)

	tests := []struct {
		in      string
		want    Environment
		wantErr bool
	}{
		{"dev", Development, false},
		{"DEV", Development, false},
		{"stage", Staging, false},
		{"prod", Production, false},
		{"bogus", Development, true},
		{"", Development, true},
	}

	for _, tt := range tests {
		got, err := EnvFromString(tt.in)
		asrt.Equal(tt.want, got, "input %q", tt.in)
		if tt.wantErr {
			asrt.Error(err, "input %q", tt.in)
		} else {
			asrt.NoError(err, "input %q", tt.in)
		}
	}
}
