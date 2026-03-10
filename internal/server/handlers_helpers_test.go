package server

import (
	"net/http/httptest"
	"testing"

	"github.com/macedot/openmodel/internal/config"
	"github.com/macedot/openmodel/internal/provider"
	"github.com/macedot/openmodel/internal/state"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFindProviderWithFailover(t *testing.T) {
	tests := []struct {
		name            string
		model           string
		modelsConfig    map[string]config.ModelConfig
		providers       map[string]provider.Provider
		failures        map[string]int
		wantErr         bool
		errContains     string
		wantProviderKey string
		wantModel       string
	}{
		{
			name:  "model exists and provider available",
			model: "gpt-4",
			modelsConfig: map[string]config.ModelConfig{
				"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
			},
			providers: map[string]provider.Provider{
				"openai": &mockProvider{nameVal: "openai"},
			},
			failures:        map[string]int{},
			wantErr:         false,
			wantProviderKey: "openai/gpt-4",
			wantModel:       "gpt-4",
		},
		{
			name:  "model exists but no providers available due to failures",
			model: "gpt-4",
			modelsConfig: map[string]config.ModelConfig{
				"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
			},
			providers: map[string]provider.Provider{
				"openai": &mockProvider{nameVal: "openai"},
			},
			failures:    map[string]int{"openai/gpt-4": 10},
			wantErr:     true,
			errContains: "no available providers",
		},
		{
			name:         "model not found",
			model:        "nonexistent",
			modelsConfig: map[string]config.ModelConfig{},
			providers: map[string]provider.Provider{
				"openai": &mockProvider{nameVal: "openai"},
			},
			failures:    map[string]int{},
			wantErr:     true,
			errContains: "not found",
		},
		{
			name:  "multiple providers first unavailable",
			model: "gpt-4",
			modelsConfig: map[string]config.ModelConfig{
				"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{
					{Provider: "openai", Model: "gpt-4"},
					{Provider: "ollama", Model: "llama-2"},
				}},
			},
			providers: map[string]provider.Provider{
				"openai": &mockProvider{nameVal: "openai"},
				"ollama": &mockProvider{nameVal: "ollama"},
			},
			failures:        map[string]int{"openai/gpt-4": 10},
			wantErr:         false,
			wantProviderKey: "ollama/llama-2",
			wantModel:       "llama-2",
		},
		{
			name:  "provider not in providers map",
			model: "gpt-4",
			modelsConfig: map[string]config.ModelConfig{
				"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
			},
			providers:   map[string]provider.Provider{},
			failures:    map[string]int{},
			wantErr:     true,
			errContains: "no available providers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Models = tt.modelsConfig

			stateMgr := state.New(cfg.Thresholds.InitialTimeout)
			// Set failure counts
			for k, v := range tt.failures {
				for i := 0; i < v; i++ {
					stateMgr.RecordFailure(k, cfg.Thresholds.FailuresBeforeSwitch)
				}
			}

			srv := New(cfg, tt.providers, stateMgr)

			_, providerKey, providerModel, err := srv.findProviderWithFailover(tt.model, "")

			if tt.wantErr {
				require.Error(t, err)
				assert.Contains(t, err.Error(), tt.errContains)
			} else {
				require.NoError(t, err)
				assert.Equal(t, tt.wantProviderKey, providerKey)
				assert.Equal(t, tt.wantModel, providerModel)
			}
		})
	}
}

func TestFindAllAvailableProviders(t *testing.T) {
	tests := []struct {
		name         string
		model        string
		modelsConfig map[string]config.ModelConfig
		providers    map[string]provider.Provider
		failures     map[string]int
		wantCount    int
	}{
		{
			name:  "all providers available",
			model: "gpt-4",
			modelsConfig: map[string]config.ModelConfig{
				"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{
					{Provider: "openai", Model: "gpt-4"},
					{Provider: "ollama", Model: "llama-2"},
				}},
			},
			providers: map[string]provider.Provider{
				"openai": &mockProvider{nameVal: "openai"},
				"ollama": &mockProvider{nameVal: "ollama"},
			},
			failures:  map[string]int{},
			wantCount: 2,
		},
		{
			name:  "some providers marked unavailable",
			model: "gpt-4",
			modelsConfig: map[string]config.ModelConfig{
				"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{
					{Provider: "openai", Model: "gpt-4"},
					{Provider: "ollama", Model: "llama-2"},
				}},
			},
			providers: map[string]provider.Provider{
				"openai": &mockProvider{nameVal: "openai"},
				"ollama": &mockProvider{nameVal: "ollama"},
			},
			failures:  map[string]int{"openai/gpt-4": 10},
			wantCount: 1,
		},
		{
			name:  "no providers available",
			model: "gpt-4",
			modelsConfig: map[string]config.ModelConfig{
				"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{
					{Provider: "openai", Model: "gpt-4"},
					{Provider: "ollama", Model: "llama-2"},
				}},
			},
			providers: map[string]provider.Provider{
				"openai": &mockProvider{nameVal: "openai"},
				"ollama": &mockProvider{nameVal: "ollama"},
			},
			failures:  map[string]int{"openai/gpt-4": 10, "ollama/llama-2": 10},
			wantCount: 0,
		},
		{
			name:  "mixed availability states",
			model: "gpt-4",
			modelsConfig: map[string]config.ModelConfig{
				"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{
					{Provider: "openai", Model: "gpt-4"},
					{Provider: "ollama", Model: "llama-2"},
					{Provider: "anthropic", Model: "claude-3"},
				}},
			},
			providers: map[string]provider.Provider{
				"openai":    &mockProvider{nameVal: "openai"},
				"ollama":    &mockProvider{nameVal: "ollama"},
				"anthropic": &mockProvider{nameVal: "anthropic"},
			},
			failures:  map[string]int{"openai/gpt-4": 5, "anthropic/claude-3": 10},
			wantCount: 1,
		},
		{
			name:  "model not found returns nil",
			model: "nonexistent",
			modelsConfig: map[string]config.ModelConfig{
				"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
			},
			providers: map[string]provider.Provider{
				"openai": &mockProvider{nameVal: "openai"},
			},
			failures:  map[string]int{},
			wantCount: 0,
		},
		{
			name:  "provider not in providers map",
			model: "gpt-4",
			modelsConfig: map[string]config.ModelConfig{
				"gpt-4": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
			},
			providers: map[string]provider.Provider{},
			failures:  map[string]int{},
			wantCount: 0,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			cfg := config.DefaultConfig()
			cfg.Models = tt.modelsConfig

			stateMgr := state.New(cfg.Thresholds.InitialTimeout)
			// Set failure counts
			for k, v := range tt.failures {
				for i := 0; i < v; i++ {
					stateMgr.RecordFailure(k, cfg.Thresholds.FailuresBeforeSwitch)
				}
			}

			srv := New(cfg, tt.providers, stateMgr)

			results := srv.findAllAvailableProviders(tt.model)

			assert.Len(t, results, tt.wantCount)
		})
	}
}

func TestValidateModel(t *testing.T) {
	cfg := config.DefaultConfig()
	cfg.Models = map[string]config.ModelConfig{
		"gpt-4":         {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-4"}}},
		"gpt-3.5-turbo": {Strategy: "fallback", Providers: []config.ModelProvider{{Provider: "openai", Model: "gpt-3.5-turbo"}}},
	}

	s := &Server{config: cfg}

	t.Run("valid model", func(t *testing.T) {
		err := s.validateModel("gpt-4")
		assert.NoError(t, err)
	})

	t.Run("another valid model", func(t *testing.T) {
		err := s.validateModel("gpt-3.5-turbo")
		assert.NoError(t, err)
	})

	t.Run("invalid model", func(t *testing.T) {
		err := s.validateModel("nonexistent-model")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
		assert.Contains(t, err.Error(), "nonexistent-model")
	})

	t.Run("empty model name", func(t *testing.T) {
		err := s.validateModel("")
		assert.Error(t, err)
		assert.Contains(t, err.Error(), "not found")
	})
}

func TestModelNotFoundError(t *testing.T) {
	err := modelNotFoundError("gpt-5")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "gpt-5")
	assert.Contains(t, err.Error(), "not found")
}

func TestIsModelNotFoundError(t *testing.T) {
	t.Run("model not found error", func(t *testing.T) {
		err := modelNotFoundError("test-model")
		assert.True(t, isModelNotFoundError(err))
	})

	t.Run("other error", func(t *testing.T) {
		err := assert.AnError
		assert.False(t, isModelNotFoundError(err))
	})

	t.Run("nil error", func(t *testing.T) {
		assert.False(t, isModelNotFoundError(nil))
	})
}

func TestEncodeJSON(t *testing.T) {
	t.Run("encodes valid data", func(t *testing.T) {
		w := httptest.NewRecorder()
		data := map[string]string{"status": "ok"}

		err := encodeJSON(w, data)

		assert.NoError(t, err)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), `"status"`)
	})

	t.Run("encodes nil returns null", func(t *testing.T) {
		w := httptest.NewRecorder()

		err := encodeJSON(w, nil)

		assert.NoError(t, err)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
	})

	t.Run("encodes struct", func(t *testing.T) {
		w := httptest.NewRecorder()
		data := struct {
			Name  string `json:"name"`
			Value int    `json:"value"`
		}{
			Name:  "test",
			Value: 42,
		}

		err := encodeJSON(w, data)

		assert.NoError(t, err)
		assert.Equal(t, "application/json", w.Header().Get("Content-Type"))
		assert.Contains(t, w.Body.String(), `"name"`)
		assert.Contains(t, w.Body.String(), `"value"`)
	})
}
