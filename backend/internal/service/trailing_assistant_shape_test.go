package service

import (
	"encoding/json"
	"testing"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
	"github.com/stretchr/testify/require"
)

func TestTrailingAssistantShape(t *testing.T) {
	input := json.RawMessage(`[
		{"type":"message","role":"user","content":"hi"},
		{"type":"reasoning","encrypted_content":"x"},
		{"type":"function_call","call_id":"c1","name":"sh","arguments":"{}"},
		{"type":"function_call_output","call_id":"c1","output":"ok"},
		{"type":"message","role":"assistant","content":[{"type":"output_text","text":"half an answer"}]}
	]`)
	req := &apicompat.AnthropicRequest{Messages: []apicompat.AnthropicMessage{
		{Role: "user", Content: json.RawMessage(`"hi"`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"thinking"},{"type":"tool_use"}]`)},
		{Role: "user", Content: json.RawMessage(`[{"type":"tool_result"}]`)},
		{Role: "assistant", Content: json.RawMessage(`[{"type":"text","text":"half an answer"}]`)},
	}}

	shape, trailing := trailingAssistantShape(input, req)
	require.True(t, trailing)
	require.Equal(t, 5, shape.InputItems)
	require.Equal(t, []string{"function_call", "function_call_output", "message:assistant"}, shape.InputTail)
	require.Equal(t, []string{"assistant[thinking,tool_use]", "user[tool_result]", "assistant[text]"}, shape.MessageTail)
}

func TestTrailingAssistantShapeEndsWithUser(t *testing.T) {
	req := &apicompat.AnthropicRequest{Messages: []apicompat.AnthropicMessage{
		{Role: "assistant", Content: json.RawMessage(`"x"`)},
		{Role: "user", Content: json.RawMessage(`"continue"`)},
	}}
	_, trailing := trailingAssistantShape(json.RawMessage(`"continue"`), req)
	require.False(t, trailing)

	_, trailing = trailingAssistantShape(nil, &apicompat.AnthropicRequest{})
	require.False(t, trailing)
}
