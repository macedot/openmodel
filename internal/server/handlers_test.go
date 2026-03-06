package server

import (
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestWriteSSEChunkWithNilFlusher(t *testing.T) {
	// httptest.ResponseRecorder doesn't implement http.Flusher
	// so the type assertion returns nil
	w := httptest.NewRecorder()
	var nilFlusher http.Flusher = nil // explicit nil flusher

	data := []byte(`{"test": "data"}`)

	// This should not panic when flusher is nil
	err := writeSSEChunk(w, nilFlusher, data)
	if err != nil {
		t.Errorf("writeSSEChunk failed: %v", err)
	}

	// Verify the response was written correctly
	if w.Body.String() != "data: {\"test\": \"data\"}\n\n" {
		t.Errorf("unexpected body: %q", w.Body.String())
	}
}

func TestWriteSSEDoneWithNilFlusher(t *testing.T) {
	w := httptest.NewRecorder()
	var nilFlusher http.Flusher = nil

	err := writeSSEDone(w, nilFlusher)
	if err != nil {
		t.Errorf("writeSSEDone failed: %v", err)
	}

	if w.Body.String() != "data: [DONE]\n\n" {
		t.Errorf("unexpected body: %q", w.Body.String())
	}
}
