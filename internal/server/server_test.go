package server

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/macedot/openmodel/internal/api/openai"
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

func TestHandleV1Models(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string][]config.ModelProvider{
		"gpt-4": {{Provider: "openai", Model: "gpt-4"}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Models(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp openai.ModelList
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if len(resp.Data) != 1 {
		t.Errorf("expected 1 model, got %d", len(resp.Data))
	}

	if resp.Data[0].ID != "gpt-4" {
		t.Errorf("expected model id 'gpt-4', got %q", resp.Data[0].ID)
	}
}

func TestHandleV1ModelNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models/nonexistent", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Model(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}

func TestHandleV1ModelFound(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string][]config.ModelProvider{
		"gpt-4": {{Provider: "openai", Model: "gpt-4"}},
	}
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	req := httptest.NewRequest(http.MethodGet, "/v1/models/gpt-4", nil)
	rec := httptest.NewRecorder()

	srv.handleV1Model(rec, req)

	if rec.Code != http.StatusOK {
		t.Errorf("expected status 200, got %d", rec.Code)
	}

	var resp openai.Model
	if err := json.Unmarshal(rec.Body.Bytes(), &resp); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if resp.ID != "gpt-4" {
		t.Errorf("expected model id 'gpt-4', got %q", resp.ID)
	}
}

func TestHandleV1ChatCompletionsModelNotFound(t *testing.T) {
	cfg := config.DefaultConfig()
	stateMgr := state.New(cfg.Thresholds.InitialTimeout)
	srv := New(cfg, nil, stateMgr)

	body := strings.NewReader(`{"model":"nonexistent","messages":[{"role":"user","content":"hi"}]}`)
	req := httptest.NewRequest(http.MethodPost, "/v1/chat/completions", body)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()

	srv.handleV1ChatCompletions(rec, req)

	if rec.Code != http.StatusNotFound {
		t.Errorf("expected status 404, got %d", rec.Code)
	}
}
