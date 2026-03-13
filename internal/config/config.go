// Package config handles JSON configuration loading
package config

import (
	"bytes"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/santhosh-tekuri/jsonschema/v6"
)

// Environment variable to control remote schema fetching
// Set to "false" or "0" to disallow remote schemas (security hardening)
const envAllowRemoteSchemas = "OPENMODEL_ALLOW_REMOTE_SCHEMAS"

// Known schema checksums for integrity verification
// Maps schema URLs to their expected SHA256 checksums
var knownSchemaChecksums = map[string]string{
	"https://raw.githubusercontent.com/macedot/openmodel/master/openmodel.schema.json": "62ccb4faffd88dad8f58a43f9811623a08a7584d2fa45ac7fe3b4d0560f34055",
}

// jsonErrorWithContext wraps JSON parsing errors with line number and context
func jsonErrorWithContext(data []byte, err error, context string) error {
	if err == nil {
		return nil
	}

	// Extract line number from offset if it's a syntax error
	if syntaxErr, ok := err.(*json.SyntaxError); ok {
		line, col := offsetToLineCol(data, syntaxErr.Offset)
		return fmt.Errorf("%s: syntax error at line %d, column %d: %w", context, line, col, err)
	}

	// Check for unmarshal type errors
	if typeErr, ok := err.(*json.UnmarshalTypeError); ok {
		line, col := offsetToLineCol(data, typeErr.Offset)
		return fmt.Errorf("%s: cannot unmarshal %s into %s at line %d, column %d",
			context, typeErr.Value, typeErr.Type.Name(), line, col)
	}

	return fmt.Errorf("%s: %w", context, err)
}

// offsetToLineCol converts a byte offset to line and column numbers
func offsetToLineCol(data []byte, offset int64) (line, col int) {
	line = 1
	col = 1

	if offset <= 0 {
		return 1, 1
	}

	// Cap offset to data length
	if offset > int64(len(data)) {
		offset = int64(len(data))
	}

	for i := int64(0); i < offset && i < int64(len(data)); i++ {
		if data[i] == '\n' {
			line++
			col = 1
		} else {
			col++
		}
	}

	return line, col
}

// jsonUnmarshalWithLines parses JSON with better error messages
func jsonUnmarshalWithLines(data []byte, v interface{}, context string) error {
	err := json.Unmarshal(data, v)
	if err != nil {
		return jsonErrorWithContext(data, err, context)
	}
	return nil
}

// Config represents the openmodel configuration
type Config struct {
	Server     ServerConfig              `json:"server"`
	Providers  map[string]ProviderConfig `json:"providers"`
	Models     map[string]ModelConfig    `json:"models"`
	ModelOrder []string                  `json:"-"` // Preserves order of models from config file
	LogLevel   string                    `json:"log_level"`
	Thresholds ThresholdsConfig          `json:"thresholds"`
	RateLimit  *RateLimitConfig          `json:"rate_limit,omitempty"`
	HTTP       HTTPConfig                `json:"http,omitempty"`
	Limits     LimitsConfig              `json:"limits,omitempty"`
	configPath string                    `json:"-"` // Path to config file that was loaded
}

// RateLimitConfig holds rate limiting configuration
type RateLimitConfig struct {
	Enabled           bool     `json:"enabled"`
	RequestsPerSecond int      `json:"requests_per_second"`
	Burst             int      `json:"burst"`
	CleanupIntervalMs int      `json:"cleanup_interval_ms"`
	TrustedProxies    []string `json:"trusted_proxies"` // List of trusted proxy IP ranges (CIDR notation supported)
}

// HTTPConfig holds HTTP client configuration
type HTTPConfig struct {
	TimeoutSeconds               int `json:"timeout_seconds"`
	MaxIdleConns                 int `json:"max_idle_conns"`
	MaxIdleConnsPerHost          int `json:"max_idle_conns_per_host"`
	IdleConnTimeoutSeconds       int `json:"idle_conn_timeout_seconds"`
	DialTimeoutSeconds           int `json:"dial_timeout_seconds"`
	TLSHandshakeTimeoutSeconds   int `json:"tls_handshake_timeout_seconds"`
	ResponseHeaderTimeoutSeconds int `json:"response_header_timeout_seconds"`
}

// LimitsConfig holds request/response size limits
type LimitsConfig struct {
	MaxRequestBodyBytes  int64 `json:"max_request_body_bytes"`  // Max request body size in bytes
	MaxResponseBodyBytes int64 `json:"max_response_body_bytes"` // Max response body size in bytes
	MaxStreamBufferBytes int64 `json:"max_stream_buffer_bytes"` // Max stream buffer size in bytes
}

// GetLimits returns the limits config, using defaults if not set
func (c *Config) GetLimits() LimitsConfig {
	if c.Limits.MaxRequestBodyBytes == 0 {
		// Return defaults
		return LimitsConfig{
			MaxRequestBodyBytes:  1 * 1024 * 1024, // 1MB
			MaxResponseBodyBytes: 1 * 1024 * 1024, // 1MB
			MaxStreamBufferBytes: 1 * 1024 * 1024, // 1MB
		}
	}
	return c.Limits
}

// ModelConfig holds configuration for a model alias
type ModelConfig struct {
	Strategy  string          `json:"strategy"`  // "fallback" | "round-robin" | "random", default "fallback"
	Default   bool            `json:"default"`   // If true, this model is the default when no model is specified
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
	APIKey     string            `json:"api_key"`    // API key (supports ${VAR} expansion)
	ApiMode    string            `json:"api_mode"`   // API format: "openai" or "anthropic" (required)
	Models     []string          `json:"models"`     // List of models available on this provider
	Thresholds *ThresholdsConfig `json:"thresholds"` // Provider-specific thresholds (optional, defaults to global)
}

// ModelProvider represents a provider model in the chain (legacy format)
type ModelProvider struct {
	Provider string `json:"provider"` // Provider name from providers config
	Model    string `json:"model"`    // Model name on that provider
}

// ProviderModel represents a model in "provider/model" format
type ProviderModel string

// ParseProviderModel parses a "provider/model" string into a ModelProvider
func ParseProviderModel(pm ProviderModel) (ModelProvider, error) {
	// Use / as separator: provider/model (model can contain /)
	parts := strings.Split(string(pm), "/")
	if len(parts) < 2 || parts[0] == "" || parts[1] == "" {
		return ModelProvider{}, fmt.Errorf("invalid provider model format: %q (expected 'provider/model')", pm)
	}
	return ModelProvider{Provider: parts[0], Model: strings.Join(parts[1:], "/")}, nil
}

// parseModelEntries parses model entries from an array of raw values
func parseModelEntries(cfg *Config, modelName string, entries []any, visited map[string]bool) ([]ModelProvider, error) {
	result := make([]ModelProvider, 0, len(entries))
	for _, entry := range entries {
		switch v := entry.(type) {
		case string:
			// Check if it's "provider/model" format or an "own model"
			// Check if it's "provider@model" format (or "provider/model" legacy) or an "own model"
			// Check if it's "provider/model" format (model can contain /) or an "own model"
			if strings.Contains(v, "/") {
				// It's "provider/model" format
				mp, err := ParseProviderModel(ProviderModel(v))
				if err != nil {
					return nil, fmt.Errorf("invalid model %q: %w", modelName, err)
				}
				// Validate provider exists
				provider, ok := cfg.Providers[mp.Provider]
				if !ok {
					return nil, fmt.Errorf("model %q references unknown provider %q", modelName, mp.Provider)
				}
				// Validate model exists in provider's models list (if models list is defined)
				if len(provider.Models) > 0 {
					modelExists := false
					for _, m := range provider.Models {
						if m == mp.Model {
							modelExists = true
							break
						}
					}
					if !modelExists {
						return nil, fmt.Errorf("model %q references model %q not found in provider %q's models list", modelName, mp.Model, mp.Provider)
					}
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
			providerCfg, ok := cfg.Providers[provider]
			if !ok {
				return nil, fmt.Errorf("model %q references unknown provider %q", modelName, provider)
			}
			// Validate model exists in provider's models list (if models list is defined)
			if len(providerCfg.Models) > 0 {
				modelExists := false
				for _, m := range providerCfg.Models {
					if m == model {
						modelExists = true
						break
					}
				}
				if !modelExists {
					return nil, fmt.Errorf("model %q references model %q not found in provider %q's models list", modelName, model, provider)
				}
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
	var schemaBytes []byte

	isRemote := strings.HasPrefix(schemaURL, "http://") || strings.HasPrefix(schemaURL, "https://")

	if isRemote {
		// Check if remote schemas are allowed
		if !isRemoteSchemaAllowed() {
			return nil, fmt.Errorf("remote schema fetching is disabled (set %s=true to allow)", envAllowRemoteSchemas)
		}

		// Warn about remote schema fetching
		fmt.Fprintf(os.Stderr, "Warning: Fetching remote schema from %s (potential security risk)\n", schemaURL)

		client := &http.Client{Timeout: 5 * time.Second} // Reduced timeout for better availability
		resp, err := client.Get(schemaURL)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch schema: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			return nil, fmt.Errorf("schema fetch returned status %d", resp.StatusCode)
		}

		// Read all bytes for checksum verification
		schemaBytes, err = readAllWithLimit(resp.Body, 10*1024*1024) // 10MB limit
		if err != nil {
			return nil, fmt.Errorf("failed to read schema: %w", err)
		}

		// Verify checksum if known
		if expectedChecksum, known := knownSchemaChecksums[schemaURL]; known && expectedChecksum != "" {
			actualChecksum := sha256Sum(schemaBytes)
			if actualChecksum != expectedChecksum {
				return nil, fmt.Errorf("schema checksum mismatch: expected %s, got %s", expectedChecksum, actualChecksum)
			}
		}

		if err := json.Unmarshal(schemaBytes, &schemaData); err != nil {
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

		var err error
		schemaBytes, err = os.ReadFile(schemaPath)
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

// isRemoteSchemaAllowed checks if remote schema fetching is allowed
func isRemoteSchemaAllowed() bool {
	val := os.Getenv(envAllowRemoteSchemas)
	if val == "" {
		return true // Default: allow for backward compatibility
	}
	return strings.ToLower(val) != "false" && val != "0"
}

// readAllWithLimit reads from reader with a size limit to prevent memory exhaustion
func readAllWithLimit(r io.Reader, limit int64) ([]byte, error) {
	return io.ReadAll(io.LimitReader(r, limit))
}

// sha256Sum computes the SHA256 checksum of data and returns it as hex string
func sha256Sum(data []byte) string {
	hash := sha256.Sum256(data)
	return hex.EncodeToString(hash[:])
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
		LogLevel: getLogLevel(),
		Thresholds: ThresholdsConfig{
			FailuresBeforeSwitch: 3,
			InitialTimeout:       10000,
			MaxTimeout:           300000,
		},
		HTTP: HTTPConfig{
			TimeoutSeconds:               120,
			MaxIdleConns:                 100,
			MaxIdleConnsPerHost:          100,
			IdleConnTimeoutSeconds:       90,
			DialTimeoutSeconds:           10,
			TLSHandshakeTimeoutSeconds:   10,
			ResponseHeaderTimeoutSeconds: 30,
		},
		Limits: LimitsConfig{
			MaxRequestBodyBytes:  1 * 1024 * 1024, // 1MB
			MaxResponseBodyBytes: 1 * 1024 * 1024, // 1MB
			MaxStreamBufferBytes: 1 * 1024 * 1024, // 1MB
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

// GetConfigPath returns the path to the config file
// Uses the stored path if available, otherwise calculates from environment
func (c *Config) GetConfigPath() string {
	if c.configPath != "" {
		return c.configPath
	}
	// Fallback: check for explicit config path in env
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

// Validate runs the repository-level configuration validations used at startup and reload time.
func (c *Config) Validate() error {
	if err := c.ValidateProviderReferences(); err != nil {
		return err
	}
	if err := c.ValidateDefaultModels(); err != nil {
		return err
	}
	return c.ValidateApiModes()
}

// GetConfigPath returns the path to the config file (standalone function for backward compatibility)
func GetConfigPath() string {
	return (&Config{}).GetConfigPath()
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

// Load loads configuration from the specified path or default locations.
// If path is empty, it merges current directory config with user config.
// Current directory config has higher priority when merging.
func Load(path string) (*Config, error) {
	// If explicit path provided, load from that path
	if path != "" {
		if _, err := os.Stat(path); os.IsNotExist(err) {
			return nil, fmt.Errorf("config file not found: %s", path)
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return nil, fmt.Errorf("failed to read config file: %w", err)
		}
		cfg, err := parseConfig(data, true)
		if err != nil {
			return nil, err
		}
		cfg.configPath = path
		return cfg, nil
	}

	// No explicit path, use default locations
	currentDirPath, userConfigPath := GetConfigPaths()

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
		cfg, err := parseConfig(userConfigData, true)
		if err != nil {
			return nil, err
		}
		cfg.configPath = userConfigPath
		return cfg, nil
	}

	// If only current dir config exists, use it
	if len(userConfigData) == 0 {
		cfg, err := parseConfig(currentDirData, true)
		if err != nil {
			return nil, err
		}
		cfg.configPath = currentDirPath
		return cfg, nil
	}

	// Both exist: merge them (current dir has higher priority)
	cfg, err := mergeAndParseConfig(userConfigData, currentDirData)
	if err != nil {
		return nil, err
	}
	cfg.configPath = currentDirPath
	return cfg, nil
}

// mergeAndParseConfig merges two config byte slices and parses them
// Values in higherPriorityData override lowerPriorityData
func mergeAndParseConfig(lowerPriorityData, higherPriorityData []byte) (*Config, error) {
	// Unmarshal both configs as map[string]any
	var lowerMap, higherMap map[string]any
	if err := jsonUnmarshalWithLines(lowerPriorityData, &lowerMap, "parsing user config"); err != nil {
		return nil, err
	}
	if err := jsonUnmarshalWithLines(higherPriorityData, &higherMap, "parsing current directory config"); err != nil {
		return nil, err
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
	if err := jsonUnmarshalWithLines(data, &schemaConfig, "parsing $schema field"); err != nil {
		return nil, err
	}

	// Validate schema is present if validation is enabled
	if validateSchema && schemaConfig.Schema == "" {
		return nil, fmt.Errorf("config file must contain $schema field")
	}

	// Validate schema if enabled
	if validateSchema && schemaConfig.Schema != "" {
		// Get schema compiler
		compiler, err := getSchemaCompiler(schemaConfig.Schema)
		if err != nil {
			// Log warning but continue - schema validation is not critical
			fmt.Fprintf(os.Stderr, "Warning: schema validation unavailable: %v\n", err)
		} else if compiler != nil {
			compiledSchema, err := compiler.Compile(schemaConfig.Schema)
			if err != nil {
				return nil, fmt.Errorf("failed to compile schema: %w", err)
			}

			// Validate config against schema
			var configData any
			if err := jsonUnmarshalWithLines(data, &configData, "parsing for schema validation"); err != nil {
				return nil, err
			}
			if err := compiledSchema.Validate(configData); err != nil {
				return nil, fmt.Errorf("config validation failed: %w", err)
			}
		}
	}

	// Parse config into a temporary structure to handle both model formats
	var tempConfig struct {
		Server     ServerConfig              `json:"server"`
		Providers  map[string]ProviderConfig `json:"providers"`
		Models     map[string]any            `json:"models"`
		LogLevel   string                    `json:"log_level"`
		Thresholds ThresholdsConfig          `json:"thresholds"`
	}
	if err := jsonUnmarshalWithLines(data, &tempConfig, "parsing config structure"); err != nil {
		return nil, err
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
	// Only override thresholds if explicitly set (non-zero)
	if tempConfig.Thresholds.FailuresBeforeSwitch != 0 {
		cfg.Thresholds = tempConfig.Thresholds
	}

	// Extract model names in order from raw JSON to preserve config file order
	var rawConfig struct {
		Models json.RawMessage `json:"models"`
	}
	if err := json.Unmarshal(data, &rawConfig); err == nil && len(rawConfig.Models) > 0 {
		dec := json.NewDecoder(bytes.NewReader(rawConfig.Models))
		tok, err := dec.Token()
		if err == nil && tok == json.Delim('{') {
			for dec.More() {
				key, err := dec.Token()
				if err != nil {
					break
				}
				if keyStr, ok := key.(string); ok {
					cfg.ModelOrder = append(cfg.ModelOrder, keyStr)
				}
				// Skip the value
				var rawValue json.RawMessage
				if err := dec.Decode(&rawValue); err != nil {
					break
				}
			}
		}
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
			if defaultVal, ok := v["default"].(bool); ok {
				modelConfig.Default = defaultVal
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
		cfg.Providers[name] = provider
	}

	// Allow env vars to override config file values
	if level := os.Getenv("OPENMODEL_LOG_LEVEL"); level != "" {
		cfg.LogLevel = level
	}

	return cfg, nil
}

// ValidateProviderReferences checks that all model providers are defined
// in the providers section. Returns an error with details if any references
// are invalid.
func (c *Config) ValidateProviderReferences() error {
	var errs []string

	for modelName, modelConfig := range c.Models {
		for i, providerRef := range modelConfig.Providers {
			// Check provider exists
			if _, exists := c.Providers[providerRef.Provider]; !exists {
				errs = append(errs, fmt.Sprintf(
					"  model %q providers[%d] references undefined provider %q",
					modelName, i, providerRef.Provider))
			}
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("provider validation failed:\n%s",
			strings.Join(errs, "\n"))
	}
	return nil
}

// ValidateDefaultModels checks that at most one model has default: true.
// Returns an error if multiple models are marked as default.
func (c *Config) ValidateDefaultModels() error {
	var defaultModels []string
	for modelName, modelConfig := range c.Models {
		if modelConfig.Default {
			defaultModels = append(defaultModels, modelName)
		}
	}

	if len(defaultModels) > 1 {
		return fmt.Errorf("multiple models marked as default: %s (only one model can be default)", strings.Join(defaultModels, ", "))
	}
	return nil
}

// ValidateApiModes checks that all provider api_mode values are valid.
// Returns an error if any provider has an invalid api_mode (empty is allowed for passthrough).
func (c *Config) ValidateApiModes() error {
	validApiModes := map[string]bool{"": true, "openai": true, "anthropic": true}
	var errs []string

	for providerName, providerConfig := range c.Providers {
		if !validApiModes[providerConfig.ApiMode] {
			errs = append(errs, fmt.Sprintf(
				"  provider %q has invalid api_mode: %q (must be 'openai', 'anthropic', or empty for passthrough)",
				providerName, providerConfig.ApiMode))
		}
	}

	if len(errs) > 0 {
		return fmt.Errorf("api_mode validation failed:\n%s",
			strings.Join(errs, "\n"))
	}
	return nil
}
