package openai

import (
	"encoding/json"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChatCompletionRequest_ExtraFields(t *testing.T) {
	// Test that extra fields like enable_thinking are preserved through marshal/unmarshal
	req := ChatCompletionRequest{
		Model:    "qwen3",
		Messages: []ChatCompletionMessage{{Role: "user", Content: "Hello"}},
		Extra: map[string]any{
			"enable_thinking": true,
			"think":           "high",
		},
	}

	// Marshal to JSON
	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Verify extra fields are included in JSON
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))
	assert.Equal(t, true, raw["enable_thinking"], "enable_thinking should be in JSON")
	assert.Equal(t, "high", raw["think"], "think should be in JSON")

	// Unmarshal back and verify extra fields are preserved
	var unmarshaled ChatCompletionRequest
	require.NoError(t, json.Unmarshal(data, &unmarshaled))

	assert.Equal(t, "qwen3", unmarshaled.Model)
	assert.Len(t, unmarshaled.Messages, 1)
	assert.Equal(t, true, unmarshaled.Extra["enable_thinking"], "enable_thinking should be preserved")
	assert.Equal(t, "high", unmarshaled.Extra["think"], "think should be preserved")
}

func TestChatCompletionRequest_ExtraFields_BackwardCompat(t *testing.T) {
	// Test that requests without extra fields work (backward compatibility)
	jsonData := `{"model":"gpt-4","messages":[{"role":"user","content":"test"}]}`

	var req ChatCompletionRequest
	require.NoError(t, json.Unmarshal([]byte(jsonData), &req))

	assert.Equal(t, "gpt-4", req.Model)
	assert.Len(t, req.Messages, 1)
	assert.Equal(t, "user", req.Messages[0].Role)
	assert.Equal(t, "test", req.Messages[0].Content)

	// Extra should be nil or empty when not provided
	if req.Extra != nil {
		assert.Empty(t, req.Extra, "Extra should be empty when not provided in JSON")
	}
}

func TestChatCompletionRequest_ExtraFields_RoundTrip(t *testing.T) {
	tests := []struct {
		name     string
		input    string
		expected map[string]any
	}{
		{
			name:  "enable_thinking boolean",
			input: `{"model":"qwen3","messages":[],"enable_thinking":true}`,
			expected: map[string]any{
				"model":           "qwen3",
				"messages":        []any{},
				"enable_thinking": true,
			},
		},
		{
			name:  "think string",
			input: `{"model":"gpt-4","messages":[],"think":"high"}`,
			expected: map[string]any{
				"model":    "gpt-4",
				"messages": []any{},
				"think":    "high",
			},
		},
		{
			name:  "nested object",
			input: `{"model":"test","messages":[],"options":{"custom_param":42}}`,
			expected: map[string]any{
				"model":    "test",
				"messages": []any{},
				"options": map[string]any{
					"custom_param": 42.0, // JSON numbers are float64
				},
			},
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			// Unmarshal from JSON
			var req ChatCompletionRequest
			require.NoError(t, json.Unmarshal([]byte(tt.input), &req))

			// Marshal back to JSON
			data, err := json.Marshal(req)
			require.NoError(t, err)

			// Verify structure
			var result map[string]any
			require.NoError(t, json.Unmarshal(data, &result))

			for k, v := range tt.expected {
				assert.Equal(t, v, result[k], "field %s should match", k)
			}
		})
	}
}

func TestChatCompletionRequest_ExtraFields_MarshalEmpty(t *testing.T) {
	// Test marshaling request without extra fields
	req := ChatCompletionRequest{
		Model:    "gpt-4",
		Messages: []ChatCompletionMessage{{Role: "user", Content: "test"}},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	// Should not have extra fields
	var raw map[string]any
	require.NoError(t, json.Unmarshal(data, &raw))

	// Check it only has standard fields
	assert.Equal(t, "gpt-4", raw["model"])
	assert.Contains(t, raw, "messages")
	assert.Len(t, raw, 2, "should only have model and messages")
}

func TestModel_JSONMarshaling(t *testing.T) {
	// Test marshaling
	model := Model{
		ID:      "gpt-4",
		Object:  "model",
		Created: 1700000000,
		OwnedBy: "openai",
	}

	data, err := json.Marshal(model)
	require.NoError(t, err)

	expected := `{"id":"gpt-4","object":"model","created":1700000000,"owned_by":"openai"}`
	assert.JSONEq(t, expected, string(data))
}

func TestModel_JSONUnmarshaling(t *testing.T) {
	jsonData := `{"id":"gpt-4","object":"model","created":1700000000,"owned_by":"openai"}`

	var model Model
	err := json.Unmarshal([]byte(jsonData), &model)
	require.NoError(t, err)

	assert.Equal(t, "gpt-4", model.ID)
	assert.Equal(t, "model", model.Object)
	assert.Equal(t, int64(1700000000), model.Created)
	assert.Equal(t, "openai", model.OwnedBy)
}

func TestModel_NewModel(t *testing.T) {
	before := time.Now().Unix()
	model := NewModel("test-model", "test-owner")
	after := time.Now().Unix()

	assert.Equal(t, "test-model", model.ID)
	assert.Equal(t, "model", model.Object)
	assert.Equal(t, "test-owner", model.OwnedBy)
	assert.GreaterOrEqual(t, model.Created, before)
	assert.LessOrEqual(t, model.Created, after)
}

func TestModelList_JSONRoundTrip(t *testing.T) {
	models := ModelList{
		Object: "list",
		Data: []Model{
			{ID: "model1", Object: "model", Created: 1700000000, OwnedBy: "owner1"},
			{ID: "model2", Object: "model", Created: 1700000001, OwnedBy: "owner2"},
		},
	}

	data, err := json.Marshal(models)
	require.NoError(t, err)

	var result ModelList
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, models.Object, result.Object)
	assert.Len(t, result.Data, 2)
	assert.Equal(t, "model1", result.Data[0].ID)
	assert.Equal(t, "model2", result.Data[1].ID)
}

func TestChatCompletionMessage_JSONRoundTrip(t *testing.T) {
	msg := ChatCompletionMessage{
		Role:    "user",
		Content: "Hello, world!",
		Name:    "test-user",
	}

	data, err := json.Marshal(msg)
	require.NoError(t, err)

	var result ChatCompletionMessage
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, msg.Role, result.Role)
	assert.Equal(t, msg.Content, result.Content)
	assert.Equal(t, msg.Name, result.Name)
}

func TestChatCompletionMessage_OptionalFields(t *testing.T) {
	// Test without optional Name field
	jsonData := `{"role":"assistant","content":"Hello!"}`

	var msg ChatCompletionMessage
	err := json.Unmarshal([]byte(jsonData), &msg)
	require.NoError(t, err)

	assert.Equal(t, "assistant", msg.Role)
	assert.Equal(t, "Hello!", msg.Content)
	assert.Empty(t, msg.Name)
}

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

func TestChatCompletionRequest_JSONMarshaling(t *testing.T) {
	temp := 0.7
	topP := 0.9
	maxTokens := 100
	n := 1

	req := ChatCompletionRequest{
		Model: "gpt-4",
		Messages: []ChatCompletionMessage{
			{Role: "user", Content: "Hello"},
		},
		Temperature:      &temp,
		TopP:             &topP,
		N:                &n,
		MaxTokens:        &maxTokens,
		Stream:           false,
		Stop:             []string{"END"},
		PresencePenalty:  &temp,
		FrequencyPenalty: &temp,
		User:             "test-user",
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var result ChatCompletionRequest
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "gpt-4", result.Model)
	assert.Len(t, result.Messages, 1)
	assert.Equal(t, 0.7, *result.Temperature)
	assert.Equal(t, 0.9, *result.TopP)
	assert.Equal(t, 100, *result.MaxTokens)
	assert.Equal(t, 1, *result.N)
	assert.False(t, result.Stream)
	assert.Equal(t, []string{"END"}, result.Stop)
	assert.Equal(t, "test-user", result.User)
}

func TestChatCompletionRequest_OptionalFields(t *testing.T) {
	// Minimal request - only required fields
	jsonData := `{"model":"gpt-4","messages":[{"role":"user","content":"hi"}]}`

	var req ChatCompletionRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	require.NoError(t, err)

	assert.Equal(t, "gpt-4", req.Model)
	assert.Len(t, req.Messages, 1)
	assert.Nil(t, req.Temperature)
	assert.Nil(t, req.TopP)
	assert.Nil(t, req.MaxTokens)
	assert.False(t, req.Stream)
}

func TestChatCompletionRequest_WithTools(t *testing.T) {
	jsonData := `{
		"model":"gpt-4",
		"messages":[{"role":"user","content":"weather"}],
		"tools":[{"type":"function","function":{"name":"get_weather","description":"Get weather","parameters":{"type":"object","properties":{"city":{"type":"string"}}}}}]
	}`

	var req ChatCompletionRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	require.NoError(t, err)

	require.Len(t, req.Tools, 1)
	assert.Equal(t, "function", req.Tools[0].Type)
	assert.Equal(t, "get_weather", req.Tools[0].Function.Name)
}

func TestChatCompletionResponse_JSONUnmarshaling(t *testing.T) {
	jsonData := `{
		"id":"chatcmpl-123",
		"object":"chat.completion",
		"created":1700000000,
		"model":"gpt-4",
		"choices":[{
			"index":0,
			"message":{"role":"assistant","content":"Hello!"},
			"finish_reason":"stop"
		}],
		"usage":{"prompt_tokens":10,"completion_tokens":5,"total_tokens":15}
	}`

	var resp ChatCompletionResponse
	err := json.Unmarshal([]byte(jsonData), &resp)
	require.NoError(t, err)

	assert.Equal(t, "chatcmpl-123", resp.ID)
	assert.Equal(t, "chat.completion", resp.Object)
	assert.Equal(t, int64(1700000000), resp.Created)
	assert.Equal(t, "gpt-4", resp.Model)

	require.Len(t, resp.Choices, 1)
	assert.Equal(t, 0, resp.Choices[0].Index)
	assert.Equal(t, "assistant", resp.Choices[0].Message.Role)
	assert.Equal(t, "Hello!", resp.Choices[0].Message.Content)
	assert.Equal(t, "stop", resp.Choices[0].FinishReason)

	require.NotNil(t, resp.Usage)
	assert.Equal(t, 10, resp.Usage.PromptTokens)
	assert.Equal(t, 5, resp.Usage.CompletionTokens)
	assert.Equal(t, 15, resp.Usage.TotalTokens)
}

func TestChatCompletionResponse_Streaming(t *testing.T) {
	jsonData := `{
		"id":"chatcmpl-123",
		"object":"chat.completion",
		"created":1700000000,
		"model":"gpt-4",
		"choices":[{
			"index":0,
			"delta":{"role":"assistant","content":"Hello"},
			"finish_reason":null
		}]
	}`

	var resp ChatCompletionResponse
	err := json.Unmarshal([]byte(jsonData), &resp)
	require.NoError(t, err)

	require.Len(t, resp.Choices, 1)
	assert.NotNil(t, resp.Choices[0].Delta)
	assert.Equal(t, "assistant", resp.Choices[0].Delta.Role)
	assert.Equal(t, "Hello", resp.Choices[0].Delta.Content)
}

func TestUsage_JSONRoundTrip(t *testing.T) {
	usage := Usage{
		PromptTokens:     100,
		CompletionTokens: 50,
		TotalTokens:      150,
	}

	data, err := json.Marshal(usage)
	require.NoError(t, err)

	var result Usage
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, usage.PromptTokens, result.PromptTokens)
	assert.Equal(t, usage.CompletionTokens, result.CompletionTokens)
	assert.Equal(t, usage.TotalTokens, result.TotalTokens)
}

func TestErrorResponse_JSONUnmarshaling(t *testing.T) {
	jsonData := `{
		"error":{
			"message":"Invalid API key",
			"type":"authentication_error",
			"param":"api_key",
			"code":"invalid_api_key"
		}
	}`

	var resp ErrorResponse
	err := json.Unmarshal([]byte(jsonData), &resp)
	require.NoError(t, err)

	require.NotNil(t, resp.Err)
	assert.Equal(t, "Invalid API key", resp.Err.Message)
	assert.Equal(t, "authentication_error", resp.Err.Type)
	assert.Equal(t, "api_key", resp.Err.Param)
	assert.Equal(t, "invalid_api_key", resp.Err.Code)
}

func TestErrorResponse_Error(t *testing.T) {
	resp := ErrorResponse{
		Err: &ErrorDetail{
			Message: "test error",
			Type:    "invalid_request_error",
		},
	}

	assert.Equal(t, "invalid_request_error: test error", resp.Error())
}

func TestErrorResponse_Error_Nil(t *testing.T) {
	resp := ErrorResponse{}
	assert.Equal(t, "unknown error", resp.Error())
}

func TestErrorResponse_Error_NilDetail(t *testing.T) {
	resp := ErrorResponse{Err: &ErrorDetail{}}
	// When Err exists but fields are empty, Error() returns ": " (empty strings)
	assert.Equal(t, ": ", resp.Error())
}

func TestParseErrorResponse_Valid(t *testing.T) {
	body := []byte(`{"error":{"message":"rate limited","type":"rate_limit_error"}}`)

	resp := ParseErrorResponse(body)
	require.NotNil(t, resp)
	require.NotNil(t, resp.Err)
	assert.Equal(t, "rate limited", resp.Err.Message)
	assert.Equal(t, "rate_limit_error", resp.Err.Type)
}

func TestParseErrorResponse_InvalidJSON(t *testing.T) {
	body := []byte(`not valid json`)

	resp := ParseErrorResponse(body)
	assert.Nil(t, resp)
}

func TestParseErrorResponse_NoErrorField(t *testing.T) {
	body := []byte(`{"success":true}`)

	resp := ParseErrorResponse(body)
	assert.Nil(t, resp)
}

func TestParseErrorResponse_NullError(t *testing.T) {
	body := []byte(`{"error":null}`)

	resp := ParseErrorResponse(body)
	assert.Nil(t, resp)
}

func TestCompletionRequest_JSONRoundTrip(t *testing.T) {
	temp := 0.5
	maxTokens := 200

	req := CompletionRequest{
		Model:       "text-davinci-003",
		Prompt:      "Write a story",
		MaxTokens:   &maxTokens,
		Temperature: &temp,
		Stream:      false,
		Stop:        []string{"END"},
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var result CompletionRequest
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "text-davinci-003", result.Model)
	assert.Equal(t, "Write a story", result.Prompt)
	assert.Equal(t, 200, *result.MaxTokens)
	assert.Equal(t, 0.5, *result.Temperature)
	assert.False(t, result.Stream)
}

func TestCompletionRequest_PromptArray(t *testing.T) {
	jsonData := `{"model":"text-davinci-003","prompt":["line1","line2","line3"]}`

	var req CompletionRequest
	err := json.Unmarshal([]byte(jsonData), &req)
	require.NoError(t, err)

	prompt, ok := req.Prompt.([]interface{})
	require.True(t, ok)
	assert.Len(t, prompt, 3)
}

func TestCompletionResponse_JSONRoundTrip(t *testing.T) {
	resp := CompletionResponse{
		ID:      "cmpl-123",
		Object:  "text_completion",
		Created: 1700000000,
		Model:   "text-davinci-003",
		Choices: []CompletionChoice{
			{
				Text:         "Hello world",
				Index:        0,
				FinishReason: "stop",
			},
		},
		Usage: &Usage{
			PromptTokens:     10,
			CompletionTokens: 5,
			TotalTokens:      15,
		},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var result CompletionResponse
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "cmpl-123", result.ID)
	assert.Equal(t, "text_completion", result.Object)
	require.Len(t, result.Choices, 1)
	assert.Equal(t, "Hello world", result.Choices[0].Text)
	assert.Equal(t, "stop", result.Choices[0].FinishReason)
}

func TestCompletionChoice_WithLogprobs(t *testing.T) {
	jsonData := `{
		"text":"Hello",
		"index":0,
		"logprobs":{"tokens":["Hello"],"token_logprobs":[0.5],"top_logprobs":[{}],"text_offset":[0]},
		"finish_reason":"stop"
	}`

	var choice CompletionChoice
	err := json.Unmarshal([]byte(jsonData), &choice)
	require.NoError(t, err)

	assert.Equal(t, "Hello", choice.Text)
	require.NotNil(t, choice.Logprobs)
	assert.Len(t, choice.Logprobs.Tokens, 1)
}

func TestEmbeddingRequest_JSONRoundTrip(t *testing.T) {
	req := EmbeddingRequest{
		Model:          "text-embedding-ada-002",
		Input:          "The quick brown fox",
		EncodingFormat: "float",
		Dimensions:     1536,
		User:           "test-user",
	}

	data, err := json.Marshal(req)
	require.NoError(t, err)

	var result EmbeddingRequest
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "text-embedding-ada-002", result.Model)
	assert.Equal(t, "The quick brown fox", result.Input)
	assert.Equal(t, "float", result.EncodingFormat)
	assert.Equal(t, 1536, result.Dimensions)
	assert.Equal(t, "test-user", result.User)
}

func TestEmbeddingRequest_InputVariants(t *testing.T) {
	// Test string input
	req1 := EmbeddingRequest{Model: "test", Input: "single string"}
	data1, _ := json.Marshal(req1)
	assert.Contains(t, string(data1), `"input":"single string"`)

	// Test array of strings
	req2 := EmbeddingRequest{Model: "test", Input: []string{"str1", "str2"}}
	data2, _ := json.Marshal(req2)
	assert.Contains(t, string(data2), `"input":["str1","str2"]`)
}

func TestEmbeddingResponse_JSONRoundTrip(t *testing.T) {
	resp := EmbeddingResponse{
		Object: "list",
		Data: []EmbeddingData{
			{
				Object:    "embedding",
				Index:     0,
				Embedding: []float64{0.1, 0.2, 0.3},
			},
		},
		Model: "text-embedding-ada-002",
		Usage: &Usage{
			PromptTokens:     10,
			CompletionTokens: 0,
			TotalTokens:      10,
		},
	}

	data, err := json.Marshal(resp)
	require.NoError(t, err)

	var result EmbeddingResponse
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "list", result.Object)
	require.Len(t, result.Data, 1)
	assert.Equal(t, 0, result.Data[0].Index)
	assert.Equal(t, []float64{0.1, 0.2, 0.3}, result.Data[0].Embedding)
	assert.Equal(t, "text-embedding-ada-002", result.Model)
}

func TestEmbeddingData_JSONRoundTrip(t *testing.T) {
	data := EmbeddingData{
		Object:    "embedding",
		Index:     5,
		Embedding: []float64{0.5, -0.3, 1.2},
	}

	jsonData, err := json.Marshal(data)
	require.NoError(t, err)

	var result EmbeddingData
	err = json.Unmarshal(jsonData, &result)
	require.NoError(t, err)

	assert.Equal(t, data.Object, result.Object)
	assert.Equal(t, data.Index, result.Index)
	assert.Equal(t, data.Embedding, result.Embedding)
}

func TestResponseFormat_JSONRoundTrip(t *testing.T) {
	format := ResponseFormat{
		Type: "json_object",
	}

	data, err := json.Marshal(format)
	require.NoError(t, err)

	var result ResponseFormat
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "json_object", result.Type)
}

func TestTool_JSONRoundTrip(t *testing.T) {
	tool := Tool{
		Type: "function",
		Function: ToolFunction{
			Name:        "get_weather",
			Description: "Get weather for a location",
			Parameters:  map[string]interface{}{"type": "object"},
		},
	}

	data, err := json.Marshal(tool)
	require.NoError(t, err)

	var result Tool
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, "function", result.Type)
	assert.Equal(t, "get_weather", result.Function.Name)
}

func TestChatCompletionChunk_StreamResponseToChunk(t *testing.T) {
	data := []byte(`{
		"id":"chatcmpl-123",
		"object":"chat.completion.chunk",
		"created":1700000000,
		"model":"gpt-4",
		"choices":[{
			"index":0,
			"delta":{"content":"Hello"},
			"finish_reason":null
		}]
	}`)

	chunk, err := StreamResponseToChunk(data)
	require.NoError(t, err)

	assert.Equal(t, "chatcmpl-123", chunk.ID)
	require.Len(t, chunk.Choices, 1)
	assert.Equal(t, "Hello", chunk.Choices[0].Delta.Content)
}

func TestChatCompletionChunk_StreamResponseToChunk_Invalid(t *testing.T) {
	data := []byte(`not valid json`)

	chunk, err := StreamResponseToChunk(data)
	assert.Nil(t, chunk)
	assert.Error(t, err)
}

func TestIsStreamDone(t *testing.T) {
	assert.True(t, IsStreamDone("[DONE]"))
	assert.True(t, IsStreamDone("data: [DONE]"))
	assert.False(t, IsStreamDone("data: {...}"))
	assert.False(t, IsStreamDone(""))
}

func TestLogprobs_JSONRoundTrip(t *testing.T) {
	logprobs := Logprobs{
		Tokens:        []string{"Hello", "world"},
		TokenLogprobs: []float64{0.1, 0.2},
		TopLogprobs:   []map[string]float64{{"world": 0.8}},
		TextOffset:    []int{0, 5},
	}

	data, err := json.Marshal(logprobs)
	require.NoError(t, err)

	var result Logprobs
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, logprobs.Tokens, result.Tokens)
	assert.Equal(t, logprobs.TokenLogprobs, result.TokenLogprobs)
	assert.Equal(t, logprobs.TopLogprobs, result.TopLogprobs)
	assert.Equal(t, logprobs.TextOffset, result.TextOffset)
}

func TestChatCompletionDelta_Streaming(t *testing.T) {
	delta := ChatCompletionDelta{
		Role:      "assistant",
		Content:   "Hello there!",
		ToolCalls: nil,
	}

	data, err := json.Marshal(delta)
	require.NoError(t, err)

	var result ChatCompletionDelta
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, delta.Role, result.Role)
	assert.Equal(t, delta.Content, result.Content)
}

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

func TestChatToolCallDelta_JSONRoundTrip(t *testing.T) {
	toolCall := ChatToolCallDelta{
		Index: 0,
		ID:    "call_123",
		Type:  "function",
		Function: &ToolCallFunctionDelta{
			Name:      "test_func",
			Arguments: `{"arg":"value"}`,
		},
	}

	data, err := json.Marshal(toolCall)
	require.NoError(t, err)

	var result ChatToolCallDelta
	err = json.Unmarshal(data, &result)
	require.NoError(t, err)

	assert.Equal(t, 0, result.Index)
	assert.Equal(t, "call_123", result.ID)
	assert.Equal(t, "function", result.Type)
	require.NotNil(t, result.Function)
	assert.Equal(t, "test_func", result.Function.Name)
}

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
