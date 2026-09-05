package fs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/stretchr/testify/assert"
)

func TestWrite(t *testing.T) {
	t.Run("writes content, creating parent directories", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		path := filepath.Join(dir, "nested", "sub", "file.txt")

		asrt.NoError(Write(path, []byte("hello")))

		got, err := os.ReadFile(path)
		asrt.NoError(err)
		asrt.Equal("hello", string(got))
	})

	t.Run("overwrites an existing file", func(t *testing.T) {
		asrt := assert.New(t)

		dir := t.TempDir()
		path := filepath.Join(dir, "file.txt")

		asrt.NoError(Write(path, []byte("first")))
		asrt.NoError(Write(path, []byte("second")))

		got, err := os.ReadFile(path)
		asrt.NoError(err)
		asrt.Equal("second", string(got))
	})
}

func TestRead(t *testing.T) {
	asrt := assert.New(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")
	asrt.NoError(os.WriteFile(path, []byte("content"), 0600))

	got, err := Read(path)
	asrt.NoError(err)
	asrt.Equal("content", string(got))

	_, err = Read(filepath.Join(dir, "missing.txt"))
	asrt.Error(err)
}

func TestCreateFile(t *testing.T) {
	asrt := assert.New(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "nested", "file.txt")

	f, err := CreateFile(path)
	asrt.NoError(err)
	asrt.NotNil(f)
	_ = f.Close()

	asrt.True(CheckIfExists(path))
}

func TestWriteFile(t *testing.T) {
	asrt := assert.New(t)

	dir := t.TempDir()
	path := filepath.Join(dir, "file.txt")

	f, err := os.Create(path)
	asrt.NoError(err)

	asrt.NoError(WriteFile(f, []byte("payload")))

	got, err := os.ReadFile(path)
	asrt.NoError(err)
	asrt.Equal("payload", string(got))
}
