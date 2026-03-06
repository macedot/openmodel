# Design: Thinking Field Support for Streaming Responses

## Date
2026-03-06

## Status
Approved

## Problem Statement

When using thinking-capable models (Qwen, DeepSeek R1, GPT-OSS) through Ollama with openmodel as a proxy, the `thinking` field containing the reasoning trace is being dropped from responses. This works with direct Ollama access but not when proxied through openmodel.

## Current Behavior

- Ollama sends streaming chunks containing both `content` and `thinking` fields
- Openmodel's `ChatCompletionDelta` struct only has `Content`, `Role`, and `ToolCalls` fields
- JSON unmarshaling silently drops unknown fields
- Clients receive content but not thinking

## Proposed Solution

Add `Thinking` string field (with `omitempty` JSON tag) to response types:

1. `ChatCompletionMessage` - non-streaming responses
2. `ChatCompletionDelta` - streaming chunks

## Architecture

### Data Flow

```
Ollama Provider
    ↓ (JSON with "thinking" field)
Openmodel unmarshals
    ↓ (thinking field populated in struct)
Openmodel marshals response
    ↓ (thinking field included in JSON)
Client receives complete response
```

### Type Definitions

```go
// ChatCompletionMessage - non-streaming responses
type ChatCompletionMessage struct {
    Role     string `json:"role"`
    Content  string `json:"content"`
    Thinking string `json:"thinking,omitempty"` // NEW
    Name     string `json:"name,omitempty"`
}

// ChatCompletionDelta - streaming chunks
type ChatCompletionDelta struct {
    Role      string              `json:"role,omitempty"`
    Content   string              `json:"content,omitempty"`
    Thinking  string              `json:"thinking,omitempty"` // NEW
    ToolCalls []ChatToolCallDelta `json:"tool_calls,omitempty"`
}
```

### Why This Works

- `omitempty` tag means empty string is not marshaled (backward compatible)
- Presence of field in struct allows unmarshaling to populate it
- Standard JSON marshal/unmarshal handles everything automatically
- No custom logic needed

## Components Affected

### Modified Files

1. **internal/api/openai/types.go**
   - Add `Thinking` field to `ChatCompletionMessage`
   - Add `Thinking` field to `ChatCompletionDelta`

2. **internal/api/openai/types_test.go**
   - Test marshaling/unmarshaling with thinking field present
   - Test backward compatibility without thinking field

### Unchanged Files

- `validation.go` - field is optional, no validation needed
- `provider.go` - streaming pipeline already handles arbitrary fields
- All server handlers - just forward JSON as-is

## Error Handling

None required. The `thinking` field:
- Is optional in JSON responses
- Uses `omitempty` to skip marshaling when empty
- Gracefully handles presence/absence in both directions

## Testing Strategy

### Unit Tests

1. **TestChatCompletionMessage_ThinkingField**
   - Marshal message with thinking field
   - Unmarshal JSON with thinking field
   - Verify thinking is preserved

2. **TestChatCompletionDelta_ThinkingField**
   - Marshal delta with thinking field
   - Unmarshal streaming chunk with thinking field
   - Verify thinking is preserved

3. **TestChatCompletionMessage_BackwardCompat**
   - Message without thinking field
   - Verify it works without the field

4. **TestChatCompletionDelta_BackwardCompat**
   - Delta without thinking field
   - Verify it works without the field

### Integration Tests

1. Mock provider streams response with thinking field
2. Verify thinking field is forwarded to client
3. Verify backward compatibility with responses lacking thinking field

## Compatibility

### Backward Compatibility

✅ **Fully backward compatible** - Adding a new optional field with `omitempty`:
- Existing responses without thinking field: no change
- Existing clients unaware of thinking field: ignore it silently
- New clients expecting thinking field: receive it when present

### Provider Compatibility

✅ **Works with all providers**:
- Ollama thinking models (Qwen, DeepSeek R1): sends thinking field
- Standard OpenAI models: doesn't send thinking field
- Other providers: optional field, no impact

## Performance Impact

**Negligible**:
- Single additional string field per response
- `omitempty` avoids marshaling when empty
- No computational overhead

## Security Considerations

**None**:
- Thinking field is just another string in the response
- No sensitive data exposure
- No new attack vectors

## Future Enhancements

None identified. This is a minimal, complete solution.

## References

- Ollama Streaming Documentation: https://docs.ollama.com/capabilities/streaming
- Thinking Models: Qwen, DeepSeek R1, GPT-OSS
- OpenAI Chat Completions API structure

## Implementation Notes

- Follow TDD: write tests first, then add field
- Minimal changes: only add field to structs
- No validation logic: field is optional
- No handler changes: streaming pipeline unchanged

## Success Criteria

- ✅ Thinking field appears in proxied responses from thinking-capable models
- ✅ Backward compatibility maintained (responses without thinking still work)
- ✅ All existing tests pass
- ✅ New tests cover thinking field scenarios