package models

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

func parseBody(request *http.Request, target any) error {
	raw, err := io.ReadAll(request.Body)
	if err != nil {
		var oversized *http.MaxBytesError
		if errors.As(err, &oversized) {
			return fmt.Errorf("models: the request body exceeds %d bytes", oversized.Limit)
		}

		return fmt.Errorf("models: read the request body: %w", err)
	}

	decoder := json.NewDecoder(bytes.NewReader(raw))
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		return fmt.Errorf("models: the request body does not parse: %w", err)
	}

	return nil
}
