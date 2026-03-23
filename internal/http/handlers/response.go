package handlers

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
)

type errorResponse struct {
	Code    string `json:"code"`
	Message string `json:"message"`
}

var errRequestTooLarge = errors.New("request body too large")

func decodeJSON(r *http.Request, dst any) error {
	defer r.Body.Close()

	decoder := json.NewDecoder(r.Body)
	decoder.DisallowUnknownFields()

	if err := decoder.Decode(dst); err != nil {
		if errors.Is(err, io.EOF) {
			return fmt.Errorf("request body must not be empty")
		}
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return errRequestTooLarge
		}
		return err
	}

	if err := decoder.Decode(&struct{}{}); !errors.Is(err, io.EOF) {
		var maxBytesErr *http.MaxBytesError
		if errors.As(err, &maxBytesErr) {
			return errRequestTooLarge
		}
		return fmt.Errorf("request body must contain only a single JSON object")
	}

	return nil
}

func writeDecodeError(w http.ResponseWriter, err error) {
	if errors.Is(err, errRequestTooLarge) {
		writeError(w, http.StatusRequestEntityTooLarge, "request_too_large", "request body exceeds configured limit")
		return
	}
	writeError(w, http.StatusBadRequest, "bad_request", fmt.Sprintf("invalid request body: %v", err))
}

func writeJSON(w http.ResponseWriter, statusCode int, value any) {
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(statusCode)
	_ = json.NewEncoder(w).Encode(value)
}

func writeError(w http.ResponseWriter, statusCode int, code, message string) {
	writeJSON(w, statusCode, errorResponse{
		Code:    code,
		Message: message,
	})
}
