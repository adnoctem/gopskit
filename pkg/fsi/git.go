package fs

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

var (
	KnownGitDir     = ".git"
	KnownGitMarkers = []string{"config", "HEAD", "branches", "objects", "refs"}
)

// ParseGitRoot traverses the filesystem upwards from the given path and checks each
// new directory whether it contains a KnownGitDir (.git subdirectory). If the
// subdirectory is found we check if it contains the KnownGitMarkers. If so the path
// is returned with a nil error.
//
// Otherwise, the first errors that occurs is returned alongside an empty string.
func ParseGitRoot(path string) (string, error) {
	path, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}

	for {
		gitDir := filepath.Join(path, KnownGitDir)
		info, err := os.Stat(gitDir)
		switch {
		case err == nil:
			if !info.IsDir() {
				return "", fmt.Errorf("%s exists but points to a file, rather than a directory", gitDir)
			}

			ok, err := findGitMarkers(gitDir)
			if err != nil {
				return "", err
			}

			if ok {
				return filepath.Dir(gitDir), nil
			}
		case !errors.Is(err, os.ErrNotExist):
			return "", err
		}

		// check if we're in a bare repository
		ok, err := findGitMarkers(path)
		if err != nil {
			return "", err
		}

		if ok {
			return path, nil
		}

		parentDir := filepath.Dir(path)
		if parentDir == path {
			return "", fmt.Errorf("cannot find .git in or below path: %s", path)
		}

		// walk up to the parent directory before the next iteration
		path = parentDir
	}
}

// findGitMarkers checks a given path for the existence of significant directories and files
// related to Git VCS
func findGitMarkers(path string) (bool, error) {
	for _, v := range KnownGitMarkers {
		_, err := os.Stat(filepath.Join(path, v))

		if err != nil {
			if !errors.Is(err, os.ErrNotExist) {
				return false, err
			}

			return false, nil
		}
	}

	return true, nil
}
