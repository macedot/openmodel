package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/macedot/openmodel/internal/api/ollama"
	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/state"
)

func TestHandleRoot(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()

	srv.handleRoot(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp map[string]string
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp["name"] != "openmodel" {
		t.Errorf("expected name 'openmodel', got %q", resp["name"])
	}
}

func TestHandleVersion(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/api/version", nil)
	rec := httptest.NewRecorder()

	srv.handleVersion(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ollama.VersionResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.Version != "0.1.0" {
		t.Errorf("expected version '0.1.0', got %q", resp.Version)
	}
}

func TestHandleTags(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string][]config.ModelBackend{
		"test-model": {
			{Provider: "ollama", Model: "test:latest"},
		},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/api/tags", nil)
	rec := httptest.NewRecorder()

	srv.handleTags(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp ollama.ListResponse
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Models) != 1 {
		t.Errorf("expected 1 model, got %d", len(resp.Models))
	}

	if resp.Models[0].Name != "test-model" {
		t.Errorf("expected model name 'test-model', got %q", resp.Models[0].Name)
	}
}

func TestHandleChatModelNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	body := `{"model":"nonexistent","messages":[{"role":"user","content":"hi"}]}`
	req := httptest.NewRequest(http.MethodPost, "/api/chat", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleChat(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleGenerateModelNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	body := `{"model":"nonexistent","prompt":"hello"}`
	req := httptest.NewRequest(http.MethodPost, "/api/generate", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleGenerate(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleV1Models(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string][]config.ModelBackend{
		"gpt-4": {{Provider: "zen", Model: "gpt-4"}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Models(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}
}
