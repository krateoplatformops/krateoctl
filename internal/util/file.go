package util

import (
	"errors"
	"os"
	"path"
)

// IsRelativeFile checks if the given string is a relative path to a file
func IsRelativeFile(root, relPath string) (string, error) {
	if !path.IsAbs(relPath) {
		foo := path.Join(path.Dir(root), relPath)
		_, err := os.Stat(foo)
		return foo, err
	}
	return "", errors.New("Is absolute file")
}
