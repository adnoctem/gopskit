package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestOnlyLetters(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(OnlyLetters("abcXYZ"))
	asrt.False(OnlyLetters("abc123"))
	asrt.False(OnlyLetters("abc def"))
	asrt.True(OnlyLetters(""))
}

func TestNotEmptyString(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(NotEmptyString("a"))
	asrt.False(NotEmptyString(""))
}

func TestNotEmptyStrings(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(NotEmptyStrings("a", "b"))
	asrt.False(NotEmptyStrings("a", ""))
	asrt.False(NotEmptyStrings("", "b"))
	asrt.False(NotEmptyStrings("", ""))
}

func TestEmptyString(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(EmptyString(""))
	asrt.False(EmptyString("a"))
}

func TestEmptyStrings(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(EmptyStrings("", ""))
	asrt.False(EmptyStrings("a", ""))
	asrt.False(EmptyStrings("", "b"))
	asrt.False(EmptyStrings("a", "b"))
}
