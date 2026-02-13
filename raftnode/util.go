package raftnode

import (
	"os"
	"path/filepath"
)

func createFile(path string) (*os.File, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return nil, err
	}
	return os.Create(path)
}
