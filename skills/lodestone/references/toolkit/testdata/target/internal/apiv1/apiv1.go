package apiv1

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

const maximumBodyBytes = 1 << 20

func decode(writer http.ResponseWriter, request *http.Request, target any) error {
	reader := http.MaxBytesReader(writer, request.Body, maximumBodyBytes)
	decoder := json.NewDecoder(reader)
	decoder.DisallowUnknownFields()
	if err := decoder.Decode(target); err != nil {
		var tooLarge *http.MaxBytesError
		if errors.As(err, &tooLarge) {
			return fmt.Errorf("request body exceeds %d bytes", tooLarge.Limit)
		}

		return fmt.Errorf("malformed request body: %w", err)
	}
	if decoder.More() {
		return errors.New("request body holds more than one document")
	}
	_, _ = io.Copy(io.Discard, reader)

	return nil
}
