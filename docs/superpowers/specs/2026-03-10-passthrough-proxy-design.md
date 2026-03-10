# Design: Pure Passthrough Proxy

**Date:** 2026-03-10
**Status:** Approved

## Summary

Convert OpenModel from a format-converting proxy to a pure transparent passthrough proxy. Remove all request/response conversion logic and forward requests directly to providers.

## Problem

The current implementation converts between OpenAI and Claude API formats:
- Claude endpoint (`/v1/messages`) converts Claude requests to OpenAI format before forwarding
- OpenAI endpoint (`/v1/chat/completions`) is OpenAI-native
- This adds unnecessary complexity and overhead when providers support both formats

## Solution

**Pure transparent proxy:** Forward requests directly to the same endpoint path on the provider, without any conversion.

### Request Flow

```
Client calls: POST /v1/messages
Proxy validates: model exists in config
Proxy resolves: model alias → provider/model
Proxy forwards: POST {provider_url}/v1/messages (same path, body unchanged)
Proxy returns: response as-is

Client calls: POST /v1/chat/completions
Proxy validates: model exists in config
Proxy resolves: model alias → provider/model
Proxy forwards: POST {provider_url}/v1/chat/completions (same path, body unchanged)
Proxy returns: response as-is
```

### What Changes

**Removed:**
- `internal/api/claude/converter.go` - Claude ↔ OpenAI conversion
- `internal/api/claude/validation.go` - Claude request validation
- Conversion calls in handlers

**Simplified:**
- `internal/provider/provider.go` - Simplify to raw forwarding
- `internal/server/claude_handlers.go` - Remove conversion, just forward
- `internal/server/handlers.go` - Keep model validation, remove response conversion

**No config changes needed:**
- No `api_mode` field required
- Config remains exactly the same
- Provider URL already includes the base path

### Code Changes

#### Provider Interface (Simplified)

```go
type Provider interface {
    Name() string
    ListModels(ctx context.Context) (*openai.ModelList, error)

    // Forward request to endpoint, return raw response
    // endpoint is the path: /v1/chat/completions, /v1/messages, etc.
    DoRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) ([]byte, error)

    // Forward streaming request to endpoint
    DoStreamRequest(ctx context.Context, endpoint string, body []byte, headers map[string]string) (<-chan []byte, error)
}
```

#### Handler Logic (Simplified)

```go
func (s *Server) handleV1Messages(w http.ResponseWriter, r *http.Request) {
    // 1. Parse request to get model name
    body, err := io.ReadAll(r.Body)
    var req struct { Model string `json:"model"` }
    json.Unmarshal(body, &req)

    // 2. Validate model exists
    if err := s.validateModel(req.Model); err != nil {
        handleError(w, err.Error(), http.StatusNotFound)
        return
    }

    // 3. Resolve model alias to provider
    prov, providerKey, providerModel, err := s.findProviderWithFailover(req.Model, "")
    if err != nil {
        s.handleAllProvidersFailed(w, err)
        return
    }

    // 4. Replace model name in body
    body = replaceModelInBody(body, providerModel)

    // 5. Forward request as-is to provider
    respBody, err := prov.DoRequest(r.Context(), "/v1/messages", body, headers)

    // 6. Return response as-is
    w.Write(respBody)
}
```

### Benefits

1. **Simplicity:** Less code, less complexity, easier to maintain
2. **Transparency:** What you send is what the provider receives
3. **Flexibility:** Supports any provider format without code changes
4. **Performance:** No conversion overhead

### Trade-offs

1. **Client compatibility:** Clients must call the correct endpoint for their provider
2. **No cross-format support:** Cannot call Claude endpoint with OpenAI format (or vice versa)

## Implementation Plan

1. Simplify `Provider` interface to raw forwarding methods
2. Update `OpenAIProvider` to implement simplified interface
3. Remove Claude conversion code
4. Update handlers to forward requests without conversion
5. Update tests to reflect new behavior
6. Update documentation

## Files Changed

- `internal/provider/provider.go` - Simplify interface
- `internal/server/handlers.go` - Remove conversion
- `internal/server/claude_handlers.go` - Simplify to forward-only
- `internal/api/claude/converter.go` - **DELETE**
- `internal/api/claude/validation.go` - **DELETE** (keep types.go)
- Tests updated accordingly