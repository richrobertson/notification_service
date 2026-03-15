package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func newTestServer() *Server {
	service := NewService()
	service.now = func() time.Time {
		return time.Date(2026, 3, 14, 15, 0, 0, 0, time.UTC)
	}
	counter := 0
	service.idgen = func() string {
		counter++
		return fmt.Sprintf("id-%d", counter)
	}
	return NewServer(service)
}

func doJSONRequest(t testing.TB, handler http.Handler, method, path string, body any) *httptest.ResponseRecorder {
	t.Helper()

	var reader *bytes.Reader
	if body == nil {
		reader = bytes.NewReader(nil)
	} else {
		payload, err := json.Marshal(body)
		if err != nil {
			t.Fatalf("The test setup should be able to encode the JSON request body: %v", err)
		}
		reader = bytes.NewReader(payload)
	}

	req := httptest.NewRequest(method, path, reader)
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	recorder := httptest.NewRecorder()
	handler.ServeHTTP(recorder, req)
	return recorder
}

func decodeBody[T any](t testing.TB, recorder *httptest.ResponseRecorder) T {
	t.Helper()

	var value T
	if err := json.Unmarshal(recorder.Body.Bytes(), &value); err != nil {
		t.Fatalf("The API response should be valid JSON that matches the expected shape: %v", err)
	}
	return value
}

func expectStatus(t testing.TB, recorder *httptest.ResponseRecorder, want int, because string) {
	t.Helper()
	if recorder.Code != want {
		t.Fatalf("%s: expected HTTP %d but received HTTP %d with body %s", because, want, recorder.Code, recorder.Body.String())
	}
}

func expectEqual[T comparable](t testing.TB, got, want T, because string) {
	t.Helper()
	if got != want {
		t.Fatalf("%s: expected %v but received %v", because, want, got)
	}
}

func expectTrue(t testing.TB, condition bool, because string) {
	t.Helper()
	if !condition {
		t.Fatalf("%s", because)
	}
}
