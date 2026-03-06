// Package config handles JSON configuration loading
package config

import (
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Config represents the openmodel configuration
type Config struct {
	Server     ServerConfig              `json:"server"`
	Providers  map[string]ProviderConfig `json:"providers"`
	Models     map[string]ModelConfig    `json:"models"`
	LogLevel   string                    `json:"log_level"`
	LogFormat  string                    `json:"log_format"`
	Thresholds ThresholdsConfig          `json:"thresholds"`
}

// ModelConfig holds configuration for a model alias
type ModelConfig struct {
	Strategy  string          `json:"strategy"`  // "fallback" | "round-robin" | "random", default "fallback"
	Providers []ModelProvider `json:"providers"` // Resolved model providers
}

// Strategy constants
const (
	StrategyFallback   = "fallback"
	StrategyRoundRobin = "round-robin"
	StrategyRandom     = "random"
)

// GetThresholds returns the thresholds for a provider (provider-specific or global)
func (c *Config) GetThresholds(providerName string) ThresholdsConfig {
	if provider, ok := c.Providers[providerName]; ok && provider.Thresholds != nil {
		return *provider.Thresholds
	}
	return c.Thresholds
}

// ResolveOwnModel resolves an "own model" (without provider prefix) to a ModelProvider
// by searching through all providers for one that has this model in its models list
// visited is used to detect circular references
func (c *Config) ResolveOwnModel(modelName string, visited map[string]bool) (ModelProvider, bool) {
	// Check for circular reference
	if visited[modelName] {
		return ModelProvider{}, false
	}
	// Mark as visited
	visited[modelName] = true
	defer func() { visited[modelName] = false }()

	for providerName, provider := range c.Providers {
		for _, m := range provider.Models {
			if m == modelName {
				return ModelProvider{Provider: providerName, Model: m}, true
			}
		}
	}
	return ModelProvider{}, false
}

// ServerConfig holds server settings
type ServerConfig struct {
	Port int    `json:"port"`
	Host string `json:"host"`
}

// ProviderConfig holds provider connection settings
type ProviderConfig struct {
	URL        string            `json:"url"`        // Base URL for the provider (e.g., https://api.openai.com/v1)
	APIKey     string            `json:"apiKey"`     // API key (supports ${VAR} expansion)
	Models     []string          `json:"models"`     // List of models available on this provider
	Thresholds *ThresholdsConfig `json:"thresholds"` // Provider-specific thresholds (optional, defaults to global)
}

// ModelProvider represents a provider model in the chain (legacy format)
type ModelProvider struct {
	Provider string `json:"provider"` // Provider name from providers config
	Model    string `json:"model"`    // Model name on that provider
}

// convertModelsField is a helper to set the models field (avoids type conflict)
func convertModelsField(cfg *Config, modelName string, models []ModelProvider) error {
	// We need to use reflection or a different approach since
	// Models is map[string][]ProviderModel but we want []ModelProvider
	// Actually, let's just change the approach - use a temporary map
	return nil // Placeholder - will be handled differently
}

// ProviderModel represents a model in "provider/model" format
type ProviderModel string

// ParseProviderModel parses a "provider/model" string into a ModelProvider
func ParseProviderModel(pm ProviderModel) (ModelProvider, error) {
	parts := strings.Split(string(pm), "/")
	if len(parts) != 2 || parts[0] == "" || parts[1] == "" {
		return ModelProvider{}, fmt.Errorf("invalid provider model format: %q (expected 'provider/model')", pm)
	}
	return ModelProvider{Provider: parts[0], Model: parts[1]}, nil
}

// parseModelEntries parses model entries from an array of raw values
func parseModelEntries(cfg *Config, modelName string, entries []any, visited map[string]bool) ([]ModelProvider, error) {
	result := make([]ModelProvider, 0, len(entries))
	for _, entry := range entries {
		switch v := entry.(type) {
		case string:
			// Check if it's "provider/model" format or an "own model"
			if strings.Contains(v, "/") {
				// It's "provider/model" format
				mp, err := ParseProviderModel(ProviderModel(v))
				if err != nil {
					return nil, fmt.Errorf("invalid model %q: %w", modelName, err)
				}
				// Validate provider exists
				if _, ok := cfg.Providers[mp.Provider]; !ok {
					return nil, fmt.Errorf("model %q references unknown provider %q", modelName, mp.Provider)
				}
				result = append(result, mp)
			} else {
				// It's an "own model" - resolve to first provider that has it
				// Note: visited map tracks resolution chain to detect true circular references
				mp, found := cfg.ResolveOwnModel(v, visited)
				if !found {
					return nil, fmt.Errorf("own model %q not found in any provider's models list", v)
				}
				result = append(result, mp)
			}

		case map[string]any:
			// Object format {provider, model}
			provider, _ := v["provider"].(string)
			model, _ := v["model"].(string)
			if provider == "" || model == "" {
				return nil, fmt.Errorf("invalid model entry in %q: missing provider or model", modelName)
			}
			if _, ok := cfg.Providers[provider]; !ok {
				return nil, fmt.Errorf("model %q references unknown provider %q", modelName, provider)
			}
			result = append(result, ModelProvider{Provider: provider, Model: model})

		default:
			return nil, fmt.Errorf("invalid model entry type in %q", modelName)
		}
	}
	return result, nil
}

// ToProviderModel converts a ModelProvider to ProviderModel format
func (mp ModelProvider) ToProviderModel() ProviderModel {
	return ProviderModel(mp.Provider + "/" + mp.Model)
}

// ThresholdsConfig holds failure threshold settings
type ThresholdsConfig struct {
	FailuresBeforeSwitch int `json:"failures_before_switch"`
	InitialTimeout       int `json:"initial_timeout_ms"`
	MaxTimeout           int `json:"max_timeout_ms"`
}

// configWithSchema is used to extract the $schema field before full parsing
type configWithSchema struct {
	Schema string `json:"$schema"`
}

// SchemaCache caches compiled JSON schemas
type SchemaCache struct {
	mu        sync.RWMutex
	compilers map[string]*jsonschema.Compiler
}

var (
	// Global schema cache instance
	schemaCache = &SchemaCache{
		compilers: make(map[string]*jsonschema.Compiler),
	}
)

func getSchemaCompiler(schemaURL string) (*jsonschema.Compiler, error) {
	// Check cache first (read lock)
	schemaCache.mu.RLock()
	if compiler, exists := schemaCache.compilers[schemaURL]; exists {
		schemaCache.mu.RUnlock()
		return compiler, nil
	}
	schemaCache.mu.RUnlock()

	// Acquire write lock and double-check
	schemaCache.mu.Lock()
	defer schemaCache.mu.Unlock()

	// Double-check after acquiring write lock
	if compiler, exists := schemaCache.compilers[schemaURL]; exists {
		return compiler, nil
	}

	// Compile new schema
	compiler := jsonschema.NewCompiler()

	var schemaData any

	if strings.HasPrefix(schemaURL, "http://") || strings.HasPrefix(schemaURL, "https://") {
		client := &http.Client{Timeout: 10 * time.Second}
		resp, err := client.Get(schemaURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch schema: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("schema fetch returned status %d", resp.StatusCode)
		}

		if err := json.NewDecoder(resp.Body).Decode(&schemaData); err != nil {
			return nil, fmt.Errorf("failed to parse schema: %w", err)
		}
	} else {
		schemaPath := schemaURL
		if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
			schemaPath = filepath.Join(os.Getenv("HOME"), ".config", "openmodel", schemaURL)
		}
		if _, err := os.Stat(schemaPath); os.IsNotExist(err) {
			schemaPath = filepath.Join(filepath.Dir(os.Args[0]), schemaURL)
		}

		schemaBytes, err := os.ReadFile(schemaPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read schema file: %w", err)
		}

		if err := json.Unmarshal(schemaBytes, &schemaData); err != nil {
			return nil, fmt.Errorf("failed to parse schema: %w", err)
		}
	}

	if err := compiler.AddResource(schemaURL, schemaData); err != nil {
		return nil, fmt.Errorf("failed to add schema: %w", err)
	}

	// Store in cache
	schemaCache.compilers[schemaURL] = compiler

	return compiler, nil
}

// DefaultConfig returns the default configuration
func DefaultConfig() *Config {
	return &Config{
		Server: ServerConfig{
			Port: 12345,
			Host: "localhost",
		},
		Providers: map[string]ProviderConfig{
			"local": {
				URL:    "http://localhost:11434/v1",
				APIKey: "",
			},
		},
		Models: map[string]ModelConfig{
			"test": {
				Strategy:  StrategyFallback,
				Providers: []ModelProvider{{Provider: "local", Model: "test"}},
			},
		},
		LogLevel:  getLogLevel(),
		LogFormat: getLogFormat(),
		Thresholds: ThresholdsConfig{
			FailuresBeforeSwitch: 3,
			InitialTimeout:       10000,
			MaxTimeout:           300000,
		},
	}
}

// expandEnvVars expands environment variables in ${VAR} format
func expandEnvVars(s string) string {
	for {
		start := strings.Index(s, "${")
		if start == -1 {
			break
		}
		end := strings.Index(s[start:], "}")
		if end == -1 {
			break
		}
		end += start
		varName := s[start+2 : end]
		envValue := os.Getenv(varName)
		s = s[:start] + envValue + s[end+1:]
	}
	return s
}

// expandProviderEnvVars expands environment variables in provider config
func expandProviderEnvVars(pc *ProviderConfig) {
	pc.APIKey = expandEnvVars(pc.APIKey)
	pc.URL = expandEnvVars(pc.URL)
}

// GetConfigPath returns the path to the config file (for backward compatibility)
func GetConfigPath() string {
	// Check for explicit config path in env
	if path := os.Getenv("OPENMODEL_CONFIG"); path != "" {
		return path
	}
	// Default to ~/.config/openmodel/openmodel.json
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return ""
	}
	return filepath.Join(homeDir, ".config", "openmodel", "openmodel.json")
}

// GetConfigPaths returns both config paths: current directory and user config
// Current directory has higher priority
func GetConfigPaths() (currentDirPath, userConfigPath string) {
	// Check for explicit config path in env
	if path := os.Getenv("OPENMODEL_CONFIG"); path != "" {
		return path, ""
	}

	// Current directory: ./openmodel.json
	currentDirPath = "openmodel.json"

	// User config: ~/.config/openmodel/openmodel.json
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return currentDirPath, ""
	}
	userConfigPath = filepath.Join(homeDir, ".config", "openmodel", "openmodel.json")

	return currentDirPath, userConfigPath
}

// getLogLevel returns the log level from environment or default
func getLogLevel() string {
	if level := os.Getenv("OPENMODEL_LOG_LEVEL"); level != "" {
		return level
	}
	return "info"
}

// getLogFormat returns the log format from environment or default
func getLogFormat() string {
	if format := os.Getenv("OPENMODEL_LOG_FORMAT"); format != "" {
		return format
	}
	return "text"
}

// Load loads configuration from file, merging current directory config with user config
// Current directory config has higher priority
func Load() (*Config, error) {
	currentDirPath, userConfigPath := GetConfigPaths()

	// If explicit path set via env, use only that
	if currentDirPath != "" && userConfigPath == "" {
		if _, err := os.Stat(currentDirPath); os.IsNotExist(err) {
			return DefaultConfig(), nil
		}
		data, err := os.ReadFile(currentDirPath)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		return parseConfig(data, true)
	}

	// Try current directory first
	currentDirData, currentDirErr := os.ReadFile(currentDirPath)

	// Try user config
	var userConfigData []byte
	if userConfigPath != "" {
		userConfigData, _ = os.ReadFile(userConfigPath)
	}

	// If neither exists, return defaults
	if currentDirErr != nil && len(userConfigData) == 0 {
		return DefaultConfig(), nil
	}

	// If only user config exists, use it
	if currentDirErr != nil && len(userConfigData) > 0 {
		return parseConfig(userConfigData, true)
	}

	// If only current dir config exists, use it
	if len(userConfigData) == 0 {
		return parseConfig(currentDirData, true)
	}

	// Both exist: merge them (current dir has higher priority)
	return mergeAndParseConfig(userConfigData, currentDirData)
}

// mergeAndParseConfig merges two config byte slices and parses them
// Values in higherPriorityData override lowerPriorityData
func mergeAndParseConfig(lowerPriorityData, higherPriorityData []byte) (*Config, error) {
	// Unmarshal both configs as map[string]any
	var lowerMap, higherMap map[string]any
	if err := json.Unmarshal(lowerPriorityData, &lowerMap); err != nil {
		return nil, fmt.Errorf("failed to parse user config: %w", err)
	}
	if err := json.Unmarshal(higherPriorityData, &higherMap); err != nil {
		return nil, fmt.Errorf("failed to parse current dir config: %w", err)
	}

	// Merge: higherPriority overwrites lowerPriority
	merged := mergeMaps(lowerMap, higherMap)

	// Re-marshal and parse
	mergedData, err := json.Marshal(merged)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal merged config: %w", err)
	}

	return parseConfig(mergedData, true)
}

// mergeMaps recursively merges two maps, with b taking priority over a
func mergeMaps(a, b map[string]any) map[string]any {
	result := make(map[string]any)

	// Copy all from a
	for k, v := range a {
		result[k] = v
	}

	// Override with b
	for k, v := range b {
		if vMap, ok := v.(map[string]any); ok {
			// If both are maps, recurse
			if aMap, ok := result[k].(map[string]any); ok {
				result[k] = mergeMaps(aMap, vMap)
			} else {
				result[k] = vMap
			}
		} else if vSlice, ok := v.([]any); ok {
			// For arrays, higher priority completely replaces lower
			result[k] = vSlice
		} else {
			// Otherwise, just override
			result[k] = v
		}
	}

	return result
}

// LoadFromPath loads configuration from a specific path
func LoadFromPath(path string) (*Config, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("failed to read config file: %w", err)
	}

	// Skip schema validation for custom paths
	return parseConfig(data, false)
}

// parseConfig parses configuration data with optional schema validation
func parseConfig(data []byte, validateSchema bool) (*Config, error) {
	// Extract $schema field
	var schemaConfig configWithSchema
	if err := json.Unmarshal(data, &schemaConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config: %w", err)
	}

	// Validate schema is present if validation is enabled
	if validateSchema && schemaConfig.Schema == "" {
		return nil, fmt.Errorf("config file must contain $schema field")
	}

	// Validate schema if enabled
	if validateSchema {
		// Get schema compiler
		compiler, err := getSchemaCompiler(schemaConfig.Schema)
		if err != nil {
			return nil, fmt.Errorf("failed to load schema: %w", err)
		}

		compiledSchema, err := compiler.Compile(schemaConfig.Schema)
		if err != nil {
			return nil, fmt.Errorf("failed to compile schema: %w", err)
		}

		// Validate config against schema
		var configData any
		if err := json.Unmarshal(data, &configData); err != nil {
			return nil, fmt.Errorf("failed to parse config data: %w", err)
		}
		if err := compiledSchema.Validate(configData); err != nil {
			return nil, fmt.Errorf("config validation failed: %w", err)
		}
	}

	// Parse config into a temporary structure to handle both model formats
	var tempConfig struct {
		Server     ServerConfig              `json:"server"`
		Providers  map[string]ProviderConfig `json:"providers"`
		Models     map[string]any            `json:"models"`
		LogLevel   string                    `json:"log_level"`
		LogFormat  string                    `json:"log_format"`
		Thresholds ThresholdsConfig          `json:"thresholds"`
	}
	if err := json.Unmarshal(data, &tempConfig); err != nil {
		return nil, fmt.Errorf("failed to parse config file: %w", err)
	}

	cfg := DefaultConfig()
	// Only override non-zero values from tempConfig
	if tempConfig.Server.Port != 0 {
		cfg.Server.Port = tempConfig.Server.Port
	}
	if tempConfig.Server.Host != "" {
		cfg.Server.Host = tempConfig.Server.Host
	}
	if len(tempConfig.Providers) > 0 {
		cfg.Providers = tempConfig.Providers
	}
	cfg.Models = make(map[string]ModelConfig)
	if tempConfig.LogLevel != "" {
		cfg.LogLevel = tempConfig.LogLevel
	}
	if tempConfig.LogFormat != "" {
		cfg.LogFormat = tempConfig.LogFormat
	}
	// Only override thresholds if explicitly set (non-zero)
	if tempConfig.Thresholds.FailuresBeforeSwitch != 0 {
		cfg.Thresholds = tempConfig.Thresholds
	}

	// Convert Models - parse each entry as either:
	// 1. Array of strings/objects (legacy format with default fallback strategy)
	// 2. Object with "strategy" and "providers" fields
	visited := make(map[string]bool)
	for modelName, modelValue := range tempConfig.Models {
		modelConfig := ModelConfig{Strategy: StrategyFallback}

		switch v := modelValue.(type) {
		case []any:
			// Legacy format: array of model entries
			providers, err := parseModelEntries(cfg, modelName, v, visited)
			if err != nil {
				return nil, err
			}
			modelConfig.Providers = providers

		case map[string]any:
			// New format: object with strategy and providers
			if strategy, ok := v["strategy"].(string); ok && strategy != "" {
				modelConfig.Strategy = strategy
			}
			if providersRaw, ok := v["providers"].([]any); ok {
				providers, err := parseModelEntries(cfg, modelName, providersRaw, visited)
				if err != nil {
					return nil, err
				}
				modelConfig.Providers = providers
			} else {
				return nil, fmt.Errorf("model %q missing providers array", modelName)
			}

		default:
			return nil, fmt.Errorf("model %q has invalid format", modelName)
		}

		cfg.Models[modelName] = modelConfig
	}

	// Expand environment variables in all provider configs
	for name, provider := range cfg.Providers {
		expandProviderEnvVars(&provider)
		// Set provider-specific thresholds from provider config
		if provider.Thresholds != nil {
			provider.Thresholds.FailuresBeforeSwitch = provider.Thresholds.FailuresBeforeSwitch
			provider.Thresholds.InitialTimeout = provider.Thresholds.InitialTimeout
			provider.Thresholds.MaxTimeout = provider.Thresholds.MaxTimeout
		}
		cfg.Providers[name] = provider
	}

	// Allow env vars to override config file values
	if level := os.Getenv("OPENMODEL_LOG_LEVEL"); level != "" {
		cfg.LogLevel = level
	}
	if format := os.Getenv("OPENMODEL_LOG_FORMAT"); format != "" {
		cfg.LogFormat = format
	}

	return cfg, nil
}
