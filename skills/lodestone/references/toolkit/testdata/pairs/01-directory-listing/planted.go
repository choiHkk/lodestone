package tools

import (
	"os"
	"path/filepath"
	"sort"
	"strings"
)

var specExts = []string{".yaml", ".yml", ".json"}

func specFiles(root string) ([]string, error) {
	entries, err := os.ReadDir(root)
	if err != nil {
		return nil, err
	}

	var out []string
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		for _, suffix := range specExts {
			if strings.HasSuffix(entry.Name(), suffix) {
				out = append(out, filepath.Join(root, entry.Name()))
				break
			}
		}
	}
	sort.Strings(out)
	return out, nil
}
