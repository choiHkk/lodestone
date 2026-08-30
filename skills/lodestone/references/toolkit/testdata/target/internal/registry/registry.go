package registry

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

type Entry struct {
	Name string `json:"name"`
}

type Registry struct {
	list func() ([]Entry, error)
}

func readJSON(path string, target any) (bool, error) {
	raw, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		return false, nil
	}
	if err != nil {
		return false, fmt.Errorf("read %s: %w", filepath.Base(path), err)
	}
	if err := json.Unmarshal(raw, target); err != nil {
		return false, fmt.Errorf("parse %s: %w", filepath.Base(path), err)
	}

	return true, nil
}

func (r *Registry) findByName(name string) (*Entry, error) {
	entries, err := r.list()
	if err != nil {
		return nil, err
	}
	for index := range entries {
		if entries[index].Name == name {
			return &entries[index], nil
		}
	}

	return nil, nil
}
