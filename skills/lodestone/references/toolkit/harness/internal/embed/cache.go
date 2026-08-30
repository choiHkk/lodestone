package embed

import (
	"context"
	"crypto/sha256"
	"encoding/gob"
	"errors"
	"fmt"
	"os"
	"path/filepath"
)

const (
	cacheVersion         = 1
	privateDirectoryMode = 0o750
)

type Cache struct {
	// Discarded names why a stored cache was not loaded, for the caller to surface.
	Discarded string
	path      string
	namespace string
	entries   map[[32]byte][]float32
	dirty     bool
}

type cacheFile struct {
	Version int
	Entries map[[32]byte][]float32
}

func OpenCache(path, namespace string) (*Cache, error) {
	cache := &Cache{path: path, namespace: namespace, entries: make(map[[32]byte][]float32)}
	if path == "" {
		return cache, nil
	}
	stored, err := loadCache(path)
	if errors.Is(err, os.ErrNotExist) {
		return cache, nil
	}
	// A cache is disposable: a truncated file from a killed run or an older
	// schema must not block analysis, only cost a re-encode.
	if err != nil {
		cache.Discarded = err.Error()

		return cache, nil //nolint:nilerr // a cache is disposable; starting empty is the recovery
	}
	if stored.Version != cacheVersion {
		cache.Discarded = fmt.Sprintf("cache version %d, want %d", stored.Version, cacheVersion)

		return cache, nil
	}
	cache.entries = stored.Entries

	return cache, nil
}

func (cache *Cache) Embed(ctx context.Context, client *Client, texts []string) ([][]float32, float64, error) {
	result := make([][]float32, len(texts))
	var missing []string
	var missingIndexes []int
	for index, text := range texts {
		key := cache.key(text)
		if vector, ok := cache.entries[key]; ok {
			result[index] = vector
			continue
		}
		missing = append(missing, text)
		missingIndexes = append(missingIndexes, index)
	}
	if len(missing) == 0 {
		return result, 0, nil
	}
	vectors, elapsed, err := client.Embed(ctx, missing)
	if err != nil {
		return nil, 0, fmt.Errorf("embed cache misses: %w", err)
	}
	for index, vector := range vectors {
		resultIndex := missingIndexes[index]
		result[resultIndex] = vector
		cache.entries[cache.key(texts[resultIndex])] = vector
	}
	cache.dirty = true
	return result, elapsed, nil
}

func (cache *Cache) Save() error {
	if cache.path == "" || !cache.dirty {
		return nil
	}
	directory := filepath.Dir(cache.path)
	if err := os.MkdirAll(directory, privateDirectoryMode); err != nil {
		return fmt.Errorf("create embedding cache directory: %w", err)
	}
	temporary, err := os.CreateTemp(directory, ".embedding-cache-*")
	if err != nil {
		return fmt.Errorf("create embedding cache: %w", err)
	}
	temporaryPath := temporary.Name()
	defer func() { _ = os.Remove(temporaryPath) }()
	if err := gob.NewEncoder(temporary).Encode(cacheFile{Version: cacheVersion, Entries: cache.entries}); err != nil {
		_ = temporary.Close()
		return fmt.Errorf("encode embedding cache: %w", err)
	}
	if err := temporary.Close(); err != nil {
		return fmt.Errorf("close embedding cache: %w", err)
	}
	if err := os.Rename(temporaryPath, cache.path); err != nil {
		return fmt.Errorf("replace embedding cache: %w", err)
	}
	cache.dirty = false

	return nil
}

func loadCache(path string) (cacheFile, error) {
	directory, err := os.OpenRoot(filepath.Dir(path))
	if err != nil {
		return cacheFile{}, fmt.Errorf("open embedding cache directory: %w", err)
	}
	defer func() { _ = directory.Close() }()
	file, err := directory.Open(filepath.Base(path))
	if err != nil {
		return cacheFile{}, fmt.Errorf("open embedding cache: %w", err)
	}
	defer func() { _ = file.Close() }()
	var stored cacheFile
	if err := gob.NewDecoder(file).Decode(&stored); err != nil {
		return cacheFile{}, fmt.Errorf("decode embedding cache: %w", err)
	}

	return stored, nil
}

func (cache *Cache) key(text string) [32]byte {
	return sha256.Sum256([]byte(cache.namespace + "\x00" + text))
}
