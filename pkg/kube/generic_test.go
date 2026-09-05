package kube

import (
	"errors"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestGet(t *testing.T) {
	asrt := assert.New(t)

	obj := "value"
	got, err := get(&obj, nil)
	asrt.NoError(err)
	asrt.Equal(&obj, got)

	boom := errors.New("boom")
	got, err = get(&obj, boom)
	asrt.ErrorIs(err, boom)
	asrt.Nil(got)
}

func TestCreated(t *testing.T) {
	asrt := assert.New(t)

	obj := "value"
	asrt.NoError(created(&obj, nil))

	boom := errors.New("boom")
	asrt.ErrorIs(created(&obj, boom), boom)
}

func TestItems(t *testing.T) {
	asrt := assert.New(t)

	type list struct{ Items []string }

	l := &list{Items: []string{"a", "b"}}
	got, err := items(l, nil, func(l *list) []string { return l.Items })
	asrt.NoError(err)
	asrt.Equal([]string{"a", "b"}, got)

	boom := errors.New("boom")
	got, err = items(l, boom, func(l *list) []string { return l.Items })
	asrt.ErrorIs(err, boom)
	asrt.Nil(got)
}
