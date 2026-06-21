package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/coding-bridge/internal/config"
)

func resolveProjectRoot(explicit string) (string, error) {
	if explicit != "" {
		root, err := filepath.Abs(explicit)
		if err != nil {
			return "", fmt.Errorf("resolve project path %q: %w", explicit, err)
		}
		info, err := os.Stat(root)
		if err != nil {
			return "", fmt.Errorf("inspect project path %q: %w", root, err)
		}
		if !info.IsDir() {
			return "", fmt.Errorf("project path is not a directory: %s", root)
		}
		return root, nil
	}
	return config.DetectProjectRoot()
}
