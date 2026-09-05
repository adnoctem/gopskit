package helpers

import (
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestRemoveDuplicates(t *testing.T) {
	asrt := assert.New(t)

	asrt.Equal([]int{1, 2, 3}, RemoveDuplicates([]int{1, 2, 2, 3, 1, 3}))
	asrt.Equal([]string{"a", "b"}, RemoveDuplicates([]string{"a", "a", "b"}))
	asrt.Nil(RemoveDuplicates([]int{}))
}

func TestSliceContains(t *testing.T) {
	asrt := assert.New(t)

	asrt.True(SliceContains([]int{1, 2, 3}, 2))
	asrt.False(SliceContains([]int{1, 2, 3}, 4))
	asrt.False(SliceContains([]int{}, 1))
	asrt.True(SliceContains([]string{"a", "b"}, "b"))
}
