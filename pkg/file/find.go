package file

import (
	"os"
	"path/filepath"
	"time"
)

func FindRecentAfter(dir string, startTime time.Time) ([]string, error) {
	var recentFiles []string

	err := filepath.Walk(dir, func(path string, info os.FileInfo,
		err error) error {
		if err != nil {
			return err
		}

		if !info.IsDir() && info.ModTime().After(startTime) {
			recentFiles = append(recentFiles, path)
		}
		return nil
	})

	return recentFiles, err
}
