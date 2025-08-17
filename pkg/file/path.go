package file

import (
	"path/filepath"
	"strings"
)

func ReplaceExt(path, ext string) string {
	if path == "" {
		return path
	}

	dir := filepath.Dir(path)
	filename := filepath.Base(path)

	lastDot := strings.LastIndex(filename, ".")

	if lastDot <= 0 {
		if ext != "" && !strings.HasPrefix(ext, ".") {
			ext = "." + ext
		}
		return filepath.Join(dir, filename+ext)
	}

	nameWithoutExt := filename[:lastDot]

	if ext != "" && !strings.HasPrefix(ext, ".") {
		ext = "." + ext
	}

	return filepath.Join(dir, nameWithoutExt+ext)
}
