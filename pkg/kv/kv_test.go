package kv

import (
	"testing"

	"github.com/dgraph-io/badger/v4"
	"github.com/stretchr/testify/assert"
)

func newTestDatabase(t *testing.T) *Database {
	t.Helper()

	dir := t.TempDir()
	db, err := New(dir, WithBadgerOptions(badger.DefaultOptions(dir).WithLoggingLevel(badger.WARNING)))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = db.Close() })

	return db
}

func TestNew(t *testing.T) {
	asrt := assert.New(t)

	db := newTestDatabase(t)
	asrt.Equal(DefaultNamespaces, db.Namespaces())
	asrt.Equal([]string{DefaultNamespace}, db.Namespaces())
}

func TestNewAppliesOptions(t *testing.T) {
	asrt := assert.New(t)

	db, err := New(t.TempDir(), WithNamespace("custom"), WithDiscardRatio(0.5))
	asrt.NoError(err)
	t.Cleanup(func() { _ = db.Close() })

	asrt.Equal("custom", db.currentNamespace)
	asrt.Equal(0.5, db.discardRatio)
}

func TestGetSetHasDelete(t *testing.T) {
	asrt := assert.New(t)
	db := newTestDatabase(t)

	has, err := db.Has("missing")
	asrt.NoError(err)
	asrt.False(has)

	asrt.NoError(db.Set("greeting", []byte("hello")))

	got, err := db.Get("greeting")
	asrt.NoError(err)
	asrt.Equal("hello", string(got))

	has, err = db.Has("greeting")
	asrt.NoError(err)
	asrt.True(has)

	asrt.NoError(db.Delete("greeting"))

	has, err = db.Has("greeting")
	asrt.NoError(err)
	asrt.False(has)
}

func TestSetOverwritesExistingValue(t *testing.T) {
	asrt := assert.New(t)
	db := newTestDatabase(t)

	asrt.NoError(db.Set("key", []byte("first")))
	asrt.NoError(db.Set("key", []byte("second")))

	got, err := db.Get("key")
	asrt.NoError(err)
	asrt.Equal("second", string(got))
}

func TestRejectsNamespacedKeys(t *testing.T) {
	asrt := assert.New(t)
	db := newTestDatabase(t)

	_, err := db.Get("ns/key")
	asrt.Error(err)

	err = db.Set("ns/key", []byte("value"))
	asrt.Error(err)

	_, err = db.Has("ns/key")
	asrt.Error(err)
}

func TestNamespaceIsolation(t *testing.T) {
	asrt := assert.New(t)
	db := newTestDatabase(t)

	asrt.NoError(db.Set("key", []byte("in-default")))

	db.SetNamespace("other")
	has, err := db.Has("key")
	asrt.NoError(err)
	asrt.False(has, "a key set in one namespace must not be visible in another")

	asrt.NoError(db.Set("key", []byte("in-other")))
	got, err := db.Get("key")
	asrt.NoError(err)
	asrt.Equal("in-other", string(got))

	db.SetNamespace(DefaultNamespace)
	got, err = db.Get("key")
	asrt.NoError(err)
	asrt.Equal("in-default", string(got), "switching back to the original namespace must restore its own value")
}

func TestPathAndConfig(t *testing.T) {
	asrt := assert.New(t)

	dir := t.TempDir()
	db, err := New(dir)
	asrt.NoError(err)
	t.Cleanup(func() { _ = db.Close() })

	asrt.Equal(dir, db.Path())
	asrt.Equal(dir, db.Config().Dir)
}
