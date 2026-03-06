# Thinking Field Support Implementation Plan

> **For Claude:** REQUIRED SUB-SKILL: Use superpowers:executing-plans to implement this plan task-by-task.

**Goal:** Add thinking field support to response types for thinking-capable models (Qwen, DeepSeek R1, GPT-OSS).

**Architecture:** Add `Thinking` string field with `omitempty` to `ChatCompletionMessage` and `ChatCompletionDelta`. Standard JSON marshal/unmarshal handles the rest automatically.

**Tech Stack:** Go, testify assertions, standard JSON encoding

---

## Task 1: Add Thinking Field to ChatCompletionMessage

**Files:**
- Modify: `internal/api/openai/types.go:25-29`
- Test: `internal/api/openai/types_test.go` (new test)

**Step 1: Write the failing test**

Add test to `internal/api/openai/types_test.go` after the existing message tests:

```go
func TestChatCompletionMessage_ThinkingField(t *testing.T) {
	// Test that thinking field is preserved through marshal/unmarshal
	msg := ChatCompletionMessage{
		Role:     "assistant",
		Content:  "The answer is 42",
		Thinking: "Let me think about this step by step...",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "assistant", raw["role"])
	assert.Equal(t, "The answer is 42", raw["content"])
	assert.Equal(t, "Let me think about this step by step...", raw["thinking"])

	// Unmarshal back
	var unmarshaled ChatCompletionMessage
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, "assistant", unmarshaled.Role)
	assert.Equal(t, "The answer is 42", unmarshaled.Content)
	assert.Equal(t, "Let me think about this step by step...", unmarshaled.Thinking)
}

func TestChatCompletionMessage_ThinkingField_Empty(t *testing.T) {
	// Test backward compatibility - empty thinking is omitted
	msg := ChatCompletionMessage{
		Role:    "assistant",
		Content: "Hello",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "assistant", raw["role"])
	assert.Equal(t, "Hello", raw["content"])
	assert.NotContains(t, raw, "thinking", "empty thinking should be omitted")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -race -v -run TestChatCompletionMessage_ThinkingField ./internal/api/openai/`

Expected: FAIL with "unknown field Thinking in struct literal"

**Step 3: Add Thinking field to ChatCompletionMessage**

Modify `internal/api/openai/types.go` line 25-29:

```go
// ChatCompletionMessage represents a message in a chat completion
type ChatCompletionMessage struct {
	Role     string `json:"role"`
	Content  string `json:"content"`
	Thinking string `json:"thinking,omitempty"`
	Name     string `json:"name,omitempty"`
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -v -run TestChatCompletionMessage_ThinkingField ./internal/api/openai/`

Expected: PASS all tests

**Step 5: Run all tests to verify no regressions**

Run: `go test -race ./internal/api/openai/`

Expected: PASS all tests

**Step 6: Commit**

```bash
git add internal/api/openai/types.go internal/api/openai/types_test.go
git commit -m "feat: add Thinking field to ChatCompletionMessage

- Add Thinking string field with omitempty tag
- Test marshaling/unmarshaling with thinking field
- Test backward compatibility without thinking field"
```

---

## Task 2: Add Thinking Field to ChatCompletionDelta

**Files:**
- Modify: `internal/api/openai/types.go:80-85`
- Test: `internal/api/openai/types_test.go` (new test)

**Step 1: Write the failing test**

Add test to `internal/api/openai/types_test.go` after Task 1 tests:

```go
func TestChatCompletionDelta_ThinkingField(t *testing.T) {
	// Test that thinking field is preserved in streaming delta
	delta := ChatCompletionDelta{
		Role:     "assistant",
		Content:  "The answer",
		Thinking: "Reasoning trace",
	}

	data, err := json.Marshal(delta)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, "assistant", raw["role"])
	assert.Equal(t, "The answer", raw["content"])
	assert.Equal(t, "Reasoning trace", raw["thinking"])

	// Unmarshal back
	var unmarshaled ChatCompletionDelta
	require.NoError(t, json.Unmarshal(data, &unmarshaled))
	assert.Equal(t, "assistant", unmarshaled.Role)
	assert.Equal(t, "The answer", unmarshaled.Content)
	assert.Equal(t, "Reasoning trace", unmarshaled.Thinking)
}

func TestChatCompletionDelta_ThinkingField_Empty(t *testing.T) {
	// Test backward compatibility - empty thinking is omitted
	delta := ChatCompletionDelta{
		Role:    "assistant",
		Content: "Hello",
	}

	data, err := json.Marshal(delta)
	require.NoError(t, err)

	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.NotContains(t, raw, "thinking", "empty thinking should be omitted")
}
```

**Step 2: Run tests to verify they fail**

Run: `go test -race -v -run TestChatCompletionDelta_ThinkingField ./internal/api/openai/`

Expected: FAIL with "unknown field Thinking in struct literal"

**Step 3: Add Thinking field to ChatCompletionDelta**

Modify `internal/api/openai/types.go` line 80-85:

```go
// ChatCompletionDelta is used for streaming responses
type ChatCompletionDelta struct {
	Role      string              `json:"role,omitempty"`
	Content   string              `json:"content,omitempty"`
	Thinking  string              `json:"thinking,omitempty"`
	ToolCalls []ChatToolCallDelta `json:"tool_calls,omitempty"`
}
```

**Step 4: Run tests to verify they pass**

Run: `go test -race -v -run TestChatCompletionDelta_ThinkingField ./internal/api/openai/`

Expected: PASS all tests

**Step 5: Run all tests to verify no regressions**

Run: `go test -race ./internal/api/openai/`

Expected: PASS all tests

**Step 6: Commit**

```bash
git add internal/api/openai/types.go internal/api/openai/types_test.go
git commit -m "feat: add Thinking field to ChatCompletionDelta

- Add Thinking string field with omitempty tag for streaming
- Test marshaling/unmarshaling with thinking field
- Test backward compatibility without thinking field"
```

---

## Task 3: Test Integration with Streaming Chunks

**Files:**
- Test: `internal/api/openai/types_test.go` (new test)

**Step 1: Write integration test for streaming chunk with thinking**

Add test to `internal/api/openai/types_test.go`:

```go
func TestChatCompletionChunk_WithThinking(t *testing.T) {
	// Test that thinking field flows through chunk structure
	chunkJSON := `{
		"id": "chatcmpl-123",
		"object": "chat.completion.chunk",
		"created": 1234567890,
		"model": "qwen3",
		"choices": [
			{
				"index": 0,
				"delta": {
					"role": "assistant",
					"thinking": "Let me calculate 17 × 23..."
				},
				"finish_reason": null
			}
		]
	}`

	chunk, err := StreamResponseToChunk([]byte(chunkJSON))
	require.NoError(t, err)
	assert.Equal(t, "chatcmpl-123", chunk.ID)
	assert.Len(t, chunk.Choices, 1)
	assert.Equal(t, 0, chunk.Choices[0].Index)
	assert.Equal(t, "assistant", chunk.Choices[0].Delta.Role)
	assert.Equal(t, "Let me calculate 17 × 23...", chunk.Choices[0].Delta.Thinking)
	assert.Equal(t, "", chunk.Choices[0].Delta.Content)
}

func TestChatCompletionChunk_WithBothThinkingAndContent(t *testing.T) {
	// Test chunk that has both thinking and content (later in stream)
	chunkJSON := `{
		"id": "chatcmpl-123",
		"object": "chat.completion.chunk",
		"created": 1234567890,
		"model": "qwen3",
		"choices": [
			{
				"index": 0,
				"delta": {
					"thinking": "So the answer is",
					"content": "391"
				},
				"finish_reason": null
			}
		]
	}`

	chunk, err := StreamResponseToChunk([]byte(chunkJSON))
	require.NoError(t, err)
	assert.Equal(t, "So the answer is", chunk.Choices[0].Delta.Thinking)
	assert.Equal(t, "391", chunk.Choices[0].Delta.Content)
}

func TestChatCompletionChunk_WithoutThinking(t *testing.T) {
	// Test backward compatibility - chunk without thinking field
	chunkJSON := `{
		"id": "chatcmpl-123",
		"object": "chat.completion.chunk",
		"created": 1234567890,
		"model": "gpt-4",
		"choices": [
			{
				"index": 0,
				"delta": {
					"role": "assistant",
					"content": "Hello"
				},
				"finish_reason": null
			}
		]
	}`

	chunk, err := StreamResponseToChunk([]byte(chunkJSON))
	require.NoError(t, err)
	assert.Equal(t, "Hello", chunk.Choices[0].Delta.Content)
	assert.Equal(t, "", chunk.Choices[0].Delta.Thinking)
}
```

**Step 2: Run tests to verify they pass**

Run: `go test -race -v -run TestChatCompletionChunk ./internal/api/openai/`

Expected: PASS all tests (fields already added in previous tasks)

**Step 3: Run all tests**

Run: `go test -race ./internal/api/openai/`

Expected: PASS all tests

**Step 4: Commit**

```bash
git add internal/api/openai/types_test.go
git commit -m "test: add integration tests for thinking field in streaming chunks

- Test chunk with thinking only
- Test chunk with both thinking and content
- Test backward compatibility without thinking"
```

---

## Task 4: Test Provider Integration

**Files:**
- Test: `internal/provider/provider_test.go` (new test)

**Step 1: Write provider integration test**

Add test to `internal/provider/provider_test.go` after existing StreamChat tests:

```go
func TestStreamChat_WithThinkingField(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req openai.ChatCompletionRequest
		body, _ := io.ReadAll(r.Body)
		_ = json.Unmarshal(body, &req)

		if !req.Stream {
			t.Error("expected streaming request")
		}

		w.Header().Set("Content-Type", "text/event-stream")
		w.Header().Set("Transfer-Encoding", "chunked")

		streamData := []string{
			`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"qwen3","choices":[{"index":0,"delta":{"role":"assistant","thinking":"Let me think..."},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"qwen3","choices":[{"index":0,"delta":{"thinking":" calculating..."},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"qwen3","choices":[{"index":0,"delta":{"content":"391"},"finish_reason":null}]}`,
			`data: {"id":"chatcmpl-123","object":"chat.completion.chunk","created":1234567890,"model":"qwen3","choices":[{"index":0,"delta":{},"finish_reason":"stop"}]}`,
			`data: [DONE]`,
		}

		flusher, ok := w.(http.Flusher)
		if !ok {
			t.Fatal("expected http.Flusher")
		}

		for _, data := range streamData {
			w.Write([]byte(data + "\n"))
			flusher.Flush()
		}
	}))
	defer server.Close()

	provider := newTestProvider(server.URL)
	ctx := context.Background()

	messages := []openai.ChatCompletionMessage{
		{Role: "user", Content: "What is 17 × 23?"},
	}

	ch, err := provider.StreamChat(ctx, "qwen3", messages, nil)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	var chunks []openai.ChatCompletionResponse
	for resp := range ch {
		chunks = append(chunks, resp)
	}

	if len(chunks) != 4 {
		t.Errorf("expected 4 chunks, got %d", len(chunks))
	}

	// Verify thinking in first chunk
	if chunks[0].Choices[0].Delta.Thinking != "Let me think..." {
		t.Errorf("expected thinking in first chunk, got %q", chunks[0].Choices[0].Delta.Thinking)
	}

	// Verify thinking in second chunk
	if chunks[1].Choices[0].Delta.Thinking != " calculating..." {
		t.Errorf("expected thinking in second chunk, got %q", chunks[1].Choices[0].Delta.Thinking)
	}

	// Verify content in third chunk
	if chunks[2].Choices[0].Delta.Content != "391" {
		t.Errorf("expected content in third chunk, got %q", chunks[2].Choices[0].Delta.Content)
	}
}
```

**Step 2: Run test to verify it passes**

Run: `go test -race -v -run TestStreamChat_WithThinkingField ./internal/provider/`

Expected: PASS (fields already added)

**Step 3: Run all provider tests**

Run: `go test -race ./internal/provider/`

Expected: PASS all tests

**Step 4: Commit**

```bash
git add internal/provider/provider_test.go
git commit -m "test: add provider integration test for thinking field streaming

- Test streaming chunks with thinking field
- Verify thinking is preserved through stream pipeline"
```

---

## Task 5: Run Complete Test Suite

**Files:**
- All packages

**Step 1: Run all tests with race detection**

Run: `go test -race ./...`

Expected: PASS all tests

**Step 2: Run tests with verbose output**

Run: `go test -race -v ./internal/api/openai/ ./internal/provider/ ./internal/server/`

Expected: PASS all tests, see detailed output

**Step 3: Verify backward compatibility**

Run: `go test -race -v -run 'TestChatCompletionMessage|TestChatCompletionDelta|TestChatCompletionChunk' ./internal/api/openai/`

Expected: All tests pass, including backward compatibility tests

**Step 4: Commit (if any fixes needed)**

If tests failed and needed fixes:
```bash
git add -A
git commit -m "fix: resolve test failures for thinking field"
```

---

## Task 6: Create Example Test Script

**Files:**
- Create: `test_thinking.sh`

**Step 1: Create test script**

Create `test_thinking.sh`:

```bash
#!/bin/bash
# Test script for thinking-capable models

PORT=${PORT:-12345}
MODEL=${MODEL:-qwen3}

time curl -v -s http://127.0.0.1:${PORT}/v1/chat/completions \
  -H "Content-Type: application/json" \
  -d '{
    "model": "'"${MODEL}"'",
    "messages": [{"role": "user", "content": "What is 17 × 23?"}],
    "stream": true
  }'
```

**Step 2: Make executable**

Run: `chmod +x test_thinking.sh`

**Step 3: Git add and commit**

```bash
git add test_thinking.sh
git commit -m "feat: add test script for thinking-capable models

- Test streaming with thinking field
- Configurable PORT and MODEL environment variables"
```

---

## Task 7: Final Verification

**Step 1: Run complete test suite one more time**

Run: `go test -race ./...`

Expected: PASS all tests

**Step 2: Verify all changes**

Run: `git log --oneline -10`

Expected: See all commits from tasks 1-6

**Step 3: Push changes**

Run: `git push origin master`

Expected: Push succeeds

---

## Success Criteria

- ✅ Thinking field added to `ChatCompletionMessage`
- ✅ Thinking field added to `ChatCompletionDelta`
- ✅ Streaming chunks preserve thinking field
- ✅ Backward compatibility maintained (empty thinking omitted)
- ✅ All tests pass with race detection
- ✅ Provider integration test confirms thinking flows through pipeline
- ✅ Example test script for manual verification

## Verification Commands

```bash
# Run all tests
go test -race ./...

# Run thinking-specific tests
go test -race -v -run 'Thinking' ./internal/api/openai/ ./internal/provider/

# Check git history
git log --oneline -7

# Verify commits
git show --stat HEAD~6
```

## Notes

- Follow TDD: write tests before implementation
- Each task commits after verification
- Tests include backward compatibility checks
- Integration test verifies end-to-end flow
- Performance: negligible overhead (single string field per response)