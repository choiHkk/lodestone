package embed

import (
	"context"
	"errors"
	"fmt"
	"io"
	"math"
	"os"
	"path/filepath"
	"time"

	"lodestone/internal/analyze"
)

const progressInterval = 64

var errDimensionMismatch = errors.New("embedding dimensions changed between batches")

type Settings struct {
	Runtime     string
	Model       string
	Profile     string
	Instruction string
	Cache       string
	Batch       int
	MaxTokens   int
	ChunkTokens int
	SplitAfter  int
	PauseMS     int
}

type Vectors struct {
	Embeddings [][]float32
	Time       time.Duration
}

type unit struct {
	owner int
	text  string
}

type state struct {
	vectors [][]float32
	counts  []int
	elapsed time.Duration
}

func Functions(
	ctx context.Context,
	functions []analyze.Function,
	settings Settings,
	progress io.Writer,
) (Vectors, error) {
	client, err := Start(settings.Runtime, settings.Model, settings.Batch, settings.MaxTokens)
	if err != nil {
		return Vectors{}, fmt.Errorf("prepare embedding runtime: %w", err)
	}
	defer func() { _ = client.Close() }()
	cache, err := OpenCache(settings.Cache, Namespace(settings))
	if err != nil {
		return Vectors{}, fmt.Errorf("open embedding cache: %w", err)
	}
	if cache.Discarded != "" {
		_, _ = fmt.Fprintf(progress, "embedding cache discarded (%s); re-encoding\n", cache.Discarded)
	}
	embeddings, elapsed, err := encode(ctx, client, cache, functions, settings, progress)
	if err != nil {
		// Keep whatever was encoded before the failure so a retry resumes
		// from the cache instead of redoing the inference.
		_ = cache.Save()

		return Vectors{}, err
	}
	if err := cache.Save(); err != nil {
		return Vectors{}, fmt.Errorf("save embeddings: %w", err)
	}
	if err := client.Close(); err != nil {
		return Vectors{}, fmt.Errorf("close embedding runtime: %w", err)
	}

	return Vectors{
		Embeddings: embeddings,
		Time:       elapsed,
	}, nil
}

func Namespace(settings Settings) string {
	info, err := os.Stat(filepath.Join(settings.Model, "model.safetensors"))
	if err != nil {
		// Sharded models carry no single weight file; the conversion
		// manifest still changes with every re-conversion, so the
		// namespace keeps a content fingerprint either way.
		info, err = os.Stat(filepath.Join(settings.Model, "conversion.json"))
	}
	if err != nil {
		return settings.Profile + "|" + settings.Model + fmt.Sprintf("|%d", settings.MaxTokens)
	}

	return fmt.Sprintf(
		"%s|%s|%d|%d|%d",
		settings.Profile, settings.Model, settings.MaxTokens, info.Size(), info.ModTime().UnixNano(),
	)
}

func Representation(settings Settings) string {
	if settings.ChunkTokens == 0 {
		return "whole-function"
	}
	threshold := settings.SplitAfter
	if threshold == 0 {
		threshold = settings.ChunkTokens
	}

	return fmt.Sprintf("%d-token-windows-after-%d", settings.ChunkTokens, threshold)
}

func encode(
	ctx context.Context,
	client *Client,
	cache *Cache,
	functions []analyze.Function,
	settings Settings,
	progress io.Writer,
) ([][]float32, time.Duration, error) {
	units := makeUnits(functions, settings)
	current := state{
		vectors: make([][]float32, len(functions)),
		counts:  make([]int, len(functions)),
	}
	for start := 0; start < len(units); start += settings.Batch {
		end := min(start+settings.Batch, len(units))
		milliseconds, err := encodeBatch(ctx, client, cache, units[start:end], &current)
		if err != nil {
			return nil, 0, fmt.Errorf("embed functions: %w", err)
		}
		if end == len(units) || end%progressInterval == 0 {
			if _, err := fmt.Fprintf(progress, "embedded functions %d/%d\r", end, len(units)); err != nil {
				return nil, 0, fmt.Errorf("write embedding progress: %w", err)
			}
		}
		if err := pause(ctx, milliseconds, end < len(units), settings.PauseMS); err != nil {
			return nil, 0, err
		}
	}
	if _, err := fmt.Fprintln(progress); err != nil {
		return nil, 0, fmt.Errorf("finish embedding progress: %w", err)
	}
	for index := range current.vectors {
		normalize(current.vectors[index], current.counts[index])
	}

	return current.vectors, current.elapsed, nil
}

func makeUnits(functions []analyze.Function, settings Settings) []unit {
	var units []unit
	for owner, function := range functions {
		for _, text := range analyze.TokenChunks(function.Source, settings.ChunkTokens, settings.SplitAfter) {
			units = append(units, unit{owner: owner, text: settings.Instruction + text})
		}
	}

	return units
}

func encodeBatch(
	ctx context.Context,
	client *Client,
	cache *Cache,
	units []unit,
	current *state,
) (float64, error) {
	texts := make([]string, 0, len(units))
	for _, item := range units {
		texts = append(texts, item.text)
	}
	embeddings, milliseconds, err := cache.Embed(ctx, client, texts)
	if err != nil {
		return 0, fmt.Errorf("read embedding batch: %w", err)
	}
	for index, vector := range embeddings {
		owner := units[index].owner
		if current.vectors[owner] == nil {
			current.vectors[owner] = make([]float32, len(vector))
		}
		if len(vector) != len(current.vectors[owner]) {
			return 0, fmt.Errorf("%w: %d then %d", errDimensionMismatch, len(current.vectors[owner]), len(vector))
		}
		for dimension, value := range vector {
			current.vectors[owner][dimension] += value
		}
		current.counts[owner]++
	}
	current.elapsed += time.Duration(milliseconds * float64(time.Millisecond))

	return milliseconds, nil
}

func pause(ctx context.Context, milliseconds float64, more bool, pauseMS int) error {
	if milliseconds <= 0 || !more || pauseMS <= 0 {
		return nil
	}
	timer := time.NewTimer(time.Duration(pauseMS) * time.Millisecond)
	defer timer.Stop()
	select {
	case <-ctx.Done():
		return fmt.Errorf("pause embedding batches: %w", ctx.Err())
	case <-timer.C:
		return nil
	}
}

func normalize(vector []float32, count int) {
	if count == 0 {
		return
	}
	var sum float64
	for _, value := range vector {
		scaled := float64(value) / float64(count)
		sum += scaled * scaled
	}
	norm := math.Sqrt(sum)
	if norm == 0 {
		return
	}
	for index := range vector {
		vector[index] = float32(float64(vector[index]) / float64(count) / norm)
	}
}
