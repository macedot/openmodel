// Package anthropic provides validation utilities for Anthropic API requests
package anthropic

import (
	"encoding/json"
	"fmt"
)

// ValidateMessagesRequest validates a messages request
func ValidateMessagesRequest(data []byte) error {
	var req map[string]interface{}
	if err := json.Unmarshal(data, &req); err != nil {
		return fmt.Errorf("invalid JSON: %w", err)
	}

	// Required: model
	if model, ok := req["model"]; !ok || model == "" {
		return fmt.Errorf("model is required")
	}

	// Required: messages
	messages, ok := req["messages"].([]interface{})
	if !ok || len(messages) == 0 {
		return fmt.Errorf("messages is required and cannot be empty")
	}

	// Validate each message
	for i, msg := range messages {
		if err := validateMessage(msg, i); err != nil {
			return err
		}
	}

	// Optional: max_tokens must be positive if set
	if maxTokens, ok := req["max_tokens"].(float64); ok && maxTokens <= 0 {
		return fmt.Errorf("max_tokens must be positive")
	}

	return nil
}

func validateMessage(m interface{}, index int) error {
	msg, ok := m.(map[string]interface{})
	if !ok {
		return fmt.Errorf("messages[%d] must be an object", index)
	}

	// Required: role
	role, ok := msg["role"].(string)
	if !ok {
		return fmt.Errorf("messages[%d].role is required", index)
	}

	validRoles := map[string]bool{"user": true, "assistant": true}
	if !validRoles[role] {
		return fmt.Errorf("messages[%d].role must be 'user' or 'assistant'", index)
	}

	// Required: content
	if content := msg["content"]; content == nil {
		return fmt.Errorf("messages[%d].content is required", index)
	}

	return nil
}
