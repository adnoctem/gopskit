package fs

import (
	"os"
	"path/filepath"
)

func CreateFile(path string) (*os.File, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	// mkdir -p
	dir := filepath.Dir(abs)
	exists := CheckIfExists(dir)
	if !exists {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return nil, err
		}
	}

	f, err := os.Create(path)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func Write(path string, content []byte) error {
	abs, err := filepath.Abs(path)
	if err != nil {
		return err
	}

	dir := filepath.Dir(abs)

	exists := CheckIfExists(dir)
	if !exists {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	if err := os.WriteFile(abs, content, 0600); err != nil {
		return err
	}

	return nil
}

func Read(path string) ([]byte, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}

	f, err := os.ReadFile(abs)
	if err != nil {
		return nil, err
	}

	return f, nil
}

func WriteFile(file *os.File, content []byte) error {
	defer func() { _ = file.Close() }()

	abs, err := filepath.Abs(file.Name())
	if err != nil {
		return err
	}

	dir := filepath.Dir(abs)

	exists := CheckIfExists(dir)
	if !exists {
		if err := os.MkdirAll(dir, 0755); err != nil {
			return err
		}
	}

	if _, err := file.Write(content); err != nil {
		return err
	}

	return nil
}
