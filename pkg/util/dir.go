package util

import (
	"github.com/pkg/errors"
	"os"
	"path/filepath"
)

func FindFileInDirOrAncestors(dir string, filename string) (string, error) {
	startedAt := dir

	for {
		if dir == "" || dir == filepath.Dir(dir) {
			return "", errors.Errorf("file %q not found in %q or any parent", filename, startedAt)
		}
		curr := filepath.Join(dir, filename)

		_, err := os.Stat(curr)
		if err == nil {
			return curr, nil
		}
		dir = filepath.Dir(dir)
	}
}
