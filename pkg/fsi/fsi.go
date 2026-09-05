// Package fsi implements a filesystem interface as well as a Paths object which holds references, to
// important directories like the platform-specific logging for configuration directories. These
// paths are either built for the generic 'gopskit' application name (to ensure our files don't
// crowd global OS directories) or for custom names, which is configurable with Opt arguments for NewPaths.
package fs

import (
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// Remove removes all files and directories at the specified path. Before running delete operations the
// function checks if the path actually exists, if it does not, it exits silently. If an error occurs
// during the delete operation, that isn't a fs.PathError, we panic.
func Remove(path string) error {
	// don't delete non-existing paths
	exists := CheckIfExists(path)
	if !exists {
		return nil
	}

	err := os.RemoveAll(path)
	if err != nil {
		var pathErr *fs.PathError
		if !errors.As(err, &pathErr) {
			panic(fmt.Errorf("os.RemoveAll returned unknown error: %v", err))
		}

		return err
	}

	return nil
}

// CheckIfExists checks if the specified path exists and returns the boolean result
func CheckIfExists(path string) bool {
	_, err := os.Stat(path)
	return !errors.Is(err, os.ErrNotExist)
}
