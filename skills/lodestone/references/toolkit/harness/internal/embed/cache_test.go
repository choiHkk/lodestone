package embed

import (
	"os"
	"path/filepath"
	"testing"
)

func TestCacheSaveAndLoad(t *testing.T) {
	t.Parallel()

	path := filepath.Join(t.TempDir(), "embeddings.gob")
	cache, err := OpenCache(path, "model")
	if err != nil {
		t.Fatal(err)
	}
	cache.entries[cache.key("source")] = []float32{1, 2}
	cache.dirty = true
	if err := cache.Save(); err != nil {
		t.Fatal(err)
	}
	loaded, err := OpenCache(path, "model")
	if err != nil {
		t.Fatal(err)
	}
	vector := loaded.entries[loaded.key("source")]
	if len(vector) != 2 || vector[0] != 1 || vector[1] != 2 {
		t.Fatalf("vector = %v", vector)
	}
	if _, ok := loaded.entries[loaded.key("other")]; ok {
		t.Fatal("unexpected cache hit")
	}
}

func TestClientCloseBeforeModelLoad(t *testing.T) {
	t.Parallel()

	runtime := "/usr/bin/true"
	model := filepath.Join(t.TempDir(), "model")
	if err := os.Mkdir(model, 0o700); err != nil {
		t.Fatal(err)
	}
	client, err := Start(runtime, model, 8, 128)
	if err != nil {
		t.Fatal(err)
	}
	if err := client.Close(); err != nil {
		t.Fatal(err)
	}
}
