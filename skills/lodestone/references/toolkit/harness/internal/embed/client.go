package embed

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

const closeGracePeriod = 5 * time.Second

var (
	errRuntimeClosed        = errors.New("embedding runtime is closed")
	errRuntimeNotExecutable = errors.New("embedding runtime is not executable")
	errRuntimeNotRegular    = errors.New("embedding runtime is not a regular file")
	errRuntimeStopped       = errors.New("embedding runtime stopped")
	errRuntimeResponse      = errors.New("embedding runtime returned an error")
	errResponseCount        = errors.New("embedding response count does not match request")
	errResponseID           = errors.New("embedding response id does not match request")
)

type Client struct {
	runtime string
	model   string
	batch   int
	tokens  int
	command *exec.Cmd
	input   io.WriteCloser
	output  *bufio.Reader
	stderr  safeBuffer
	nextID  int
	closed  bool
}

type request struct {
	ID    int      `json:"id"`
	Texts []string `json:"texts"`
}

type response struct {
	ID                  *int        `json:"id"`
	Embeddings          [][]float32 `json:"embeddings"`
	Dimension           *int        `json:"dimension"`
	ElapsedMilliseconds *float64    `json:"elapsedMilliseconds"`
	Error               *string     `json:"error"`
}

func Start(runtimePath, modelPath string, maxBatch, maxTokens int) (*Client, error) {
	runtimePath, err := filepath.Abs(runtimePath)
	if err != nil {
		return nil, fmt.Errorf("resolve embedding runtime: %w", err)
	}
	info, err := os.Stat(runtimePath)
	if err != nil {
		return nil, fmt.Errorf("inspect embedding runtime: %w", err)
	}
	if !info.Mode().IsRegular() {
		return nil, errRuntimeNotRegular
	}
	if info.Mode()&0o111 == 0 {
		return nil, errRuntimeNotExecutable
	}
	modelPath, err = filepath.Abs(modelPath)
	if err != nil {
		return nil, fmt.Errorf("resolve embedding model: %w", err)
	}
	if _, err = os.Stat(modelPath); err != nil {
		return nil, fmt.Errorf("inspect embedding model: %w", err)
	}

	return &Client{
		runtime: runtimePath, model: modelPath,
		batch: maxBatch, tokens: maxTokens,
	}, nil
}

func (client *Client) Embed(ctx context.Context, texts []string) ([][]float32, float64, error) {
	if client.closed {
		return nil, 0, errRuntimeClosed
	}
	if client.command == nil {
		if err := client.start(ctx); err != nil {
			return nil, 0, err
		}
	}
	client.nextID++
	message, err := json.Marshal(request{ID: client.nextID, Texts: texts})
	if err != nil {
		return nil, 0, fmt.Errorf("encode embedding request: %w", err)
	}
	message = append(message, '\n')
	if _, err := client.input.Write(message); err != nil {
		return nil, 0, fmt.Errorf("write embedding request: %w: %s", err, client.errorText())
	}
	line, err := client.output.ReadBytes('\n')
	if err != nil {
		if len(line) == 0 && errors.Is(err, io.EOF) {
			return nil, 0, fmt.Errorf("%w: %s", errRuntimeStopped, client.errorText())
		}

		return nil, 0, fmt.Errorf("read embedding response: %w", err)
	}
	var result response
	if err := json.Unmarshal(line, &result); err != nil {
		return nil, 0, fmt.Errorf("decode embedding response: %w", err)
	}

	return client.validate(result, len(texts))
}

func (client *Client) Close() error {
	if client.closed {
		return nil
	}
	client.closed = true
	if client.command == nil {
		return nil
	}
	inputErr := client.input.Close()
	waited := make(chan error, 1)
	go func() { waited <- client.command.Wait() }()
	var waitErr error
	select {
	case waitErr = <-waited:
	case <-time.After(closeGracePeriod):
		_ = client.command.Process.Kill()
		waitErr = <-waited
	}
	if inputErr != nil {
		return fmt.Errorf("close embedding runtime input: %w", inputErr)
	}
	if waitErr != nil {
		return fmt.Errorf("stop embedding runtime: %w: %s", waitErr, client.errorText())
	}

	return nil
}

func (client *Client) start(ctx context.Context) error {
	//nolint:gosec // the runtime path and model are operator-supplied flags
	command := exec.CommandContext(
		ctx,
		client.runtime,
		"--model", client.model,
		"--max-batch", strconv.Itoa(client.batch),
		"--max-tokens", strconv.Itoa(client.tokens),
	)
	input, err := command.StdinPipe()
	if err != nil {
		return fmt.Errorf("open runtime input: %w", err)
	}
	output, err := command.StdoutPipe()
	if err != nil {
		return fmt.Errorf("open runtime output: %w", err)
	}
	client.command = command
	client.input = input
	client.output = bufio.NewReader(output)
	command.Stderr = &client.stderr
	if err := command.Start(); err != nil {
		return fmt.Errorf("start embedding runtime: %w", err)
	}

	return nil
}

func (client *Client) validate(result response, expected int) ([][]float32, float64, error) {
	if result.Error != nil {
		return nil, 0, fmt.Errorf("%w: %s", errRuntimeResponse, *result.Error)
	}
	if result.ID == nil || *result.ID != client.nextID {
		return nil, 0, errResponseID
	}
	if len(result.Embeddings) != expected {
		return nil, 0, errResponseCount
	}
	elapsed := 0.0
	if result.ElapsedMilliseconds != nil {
		elapsed = *result.ElapsedMilliseconds
	}
	return result.Embeddings, elapsed, nil
}

func (client *Client) errorText() string {
	text := strings.TrimSpace(client.stderr.String())
	if text == "" {
		return "no error output"
	}
	return text
}

// safeBuffer guards stderr against the copier goroutine exec spawns for
// non-file writers, which may still be writing when a failed Embed reads.
type safeBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (buffer *safeBuffer) Write(data []byte) (int, error) {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()
	count, err := buffer.buf.Write(data)
	if err != nil {
		return count, fmt.Errorf("buffer runtime stderr: %w", err)
	}

	return count, nil
}

func (buffer *safeBuffer) String() string {
	buffer.mu.Lock()
	defer buffer.mu.Unlock()

	return buffer.buf.String()
}
