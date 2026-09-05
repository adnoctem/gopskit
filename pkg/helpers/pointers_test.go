package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestPtr(t *testing.T) {
	asrt := assert.New(t)

	i := 42
	p := Ptr(i)
	asrt.Equal(&i, p)
	asrt.Equal(i, *p)

	// addressing a literal directly is the main use case, since Go won't let
	// you take &42 or &"str" without a named variable first
	sp := Ptr("hello")
	asrt.Equal("hello", *sp)
}
