package schema

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

func loadDocument(path string, target any) error {
	if _, err := os.Stat(path); err != nil {
		if os.IsNotExist(err) {
			return fmt.Errorf("schema: %s is absent", filepath.Base(path))
		}
		return fmt.Errorf("schema: stat %s: %w", filepath.Base(path), err)
	}

	handle, err := os.Open(path)
	if err != nil {
		return fmt.Errorf("schema: open %s: %w", filepath.Base(path), err)
	}
	defer handle.Close()

	if err := json.NewDecoder(handle).Decode(target); err != nil {
		return fmt.Errorf("schema: parse %s: %w", filepath.Base(path), err)
	}

	return nil
}
