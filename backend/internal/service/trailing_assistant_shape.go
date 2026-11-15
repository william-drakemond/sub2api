package service

import (
	"encoding/json"
	"strings"

	"github.com/Wei-Shaw/sub2api/internal/pkg/apicompat"
)

type trailingAssistantDiagnostic struct {
	InputItems  int
	InputTail   []string
	MessageTail []string
}

// trailingAssistantShape summarizes the tail of a Responses input and its
// converted Anthropic messages when the conversion ends on an assistant turn
// (which adaptive Claude 5.5 / 5.2 models reject as assistant prefill).
func trailingAssistantShape(inputRaw json.RawMessage, req *apicompat.AnthropicRequest) (trailingAssistantDiagnostic, bool) {
	if req == nil || len(req.Messages) == 0 {
		return trailingAssistantDiagnostic{}, false
	}
	if req.Messages[len(req.Messages)-1].Role != "assistant" {
		return trailingAssistantDiagnostic{}, false
	}

	var diag trailingAssistantDiagnostic
	var items []apicompat.ResponsesInputItem
	if len(inputRaw) > 0 && json.Unmarshal(inputRaw, &items) == nil {
		diag.InputItems = len(items)
		start := len(items) - 3
		if start < 0 {
			start = 0
		}
		for _, item := range items[start:] {
			label := item.Type
			if label == "" {
				label = "message"
			}
			if item.Role != "" && (label == "message" || item.Type == "") {
				label = label + ":" + item.Role
			} else if item.Role != "" {
				label = label + ":" + item.Role
			}
			diag.InputTail = append(diag.InputTail, label)
		}
	}

	msgStart := len(req.Messages) - 3
	if msgStart < 0 {
		msgStart = 0
	}
	for _, m := range req.Messages[msgStart:] {
		diag.MessageTail = append(diag.MessageTail, summarizeAnthropicMessageShape(m))
	}
	return diag, true
}

func summarizeAnthropicMessageShape(m apicompat.AnthropicMessage) string {
	var blocks []struct {
		Type string `json:"type"`
	}
	if json.Unmarshal(m.Content, &blocks) == nil && len(blocks) > 0 {
		types := make([]string, 0, len(blocks))
		for _, b := range blocks {
			t := b.Type
			if t == "" {
				t = "?"
			}
			types = append(types, t)
		}
		return m.Role + "[" + strings.Join(types, ",") + "]"
	}
	var s string
	if json.Unmarshal(m.Content, &s) == nil {
		return m.Role + "[string]"
	}
	return m.Role + "[?]"
}
