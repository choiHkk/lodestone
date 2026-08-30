package loader

import (
	"path/filepath"
	"sort"
)

func configPaths(dir string, extensions []string) ([]string, error) {
	var paths []string
	for _, extension := range extensions {
		matches, err := filepath.Glob(filepath.Join(dir, "*"+extension))
		if err != nil {
			return nil, err
		}
		paths = append(paths, matches...)
	}
	sort.Strings(paths)

	return paths, nil
}
