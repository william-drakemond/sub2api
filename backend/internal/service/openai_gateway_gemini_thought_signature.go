package service

import (
	"net/url"
	"strconv"
	"strings"

	"github.com/tidwall/gjson"
	"github.com/tidwall/sjson"
)

const (
	geminiAIStudioHost             = "generativelanguage.googleapis.com"
	liteLLMThoughtSignatureIDInfix = "__thought__"
)

// targetsGeminiAIStudio reports whether the account's Chat Completions
// upstream is Google AI Studio's OpenAI-compatible endpoint.
func targetsGeminiAIStudio(account *Account) bool {
	if account == nil {
		return false
	}
	u, err := url.Parse(strings.TrimSpace(account.GetOpenAIBaseURL()))
	if err != nil {
		return false
	}
	return strings.EqualFold(u.Hostname(), geminiAIStudioHost)
}

// ensureGeminiChatThoughtSignatures gives every replayed assistant tool call a
// Gemini thought signature before the request goes to AI Studio.
//
// Gemini returns the signature in tool_calls[].extra_content.google, but the
// Responses items Codex keeps have no place for it, so history comes back
// without one and AI Studio answers 400 "Function call is missing a
// thought_signature in functionCall parts". A call whose id carries LiteLLM's
// "__thought__<signature>" suffix (Unity's LiteLLM) gets that real signature;
// any other gets the documented skip_thought_signature_validator value, the
// same one the native Gemini path uses. Existing signatures are kept, and the
// body is returned unchanged for every other upstream.
func ensureGeminiChatThoughtSignatures(account *Account, body []byte) []byte {
	if !targetsGeminiAIStudio(account) {
		return body
	}
	messages := gjson.GetBytes(body, "messages")
	if !messages.IsArray() {
		return body
	}
	updated := body
	for i, msg := range messages.Array() {
		if msg.Get("role").String() != "assistant" {
			continue
		}
		calls := msg.Get("tool_calls")
		if !calls.IsArray() {
			continue
		}
		for j, call := range calls.Array() {
			if call.Get("extra_content.google.thought_signature").String() != "" {
				continue
			}
			signature := geminiDummyThoughtSignature
			if _, suffix, ok := strings.Cut(call.Get("id").String(), liteLLMThoughtSignatureIDInfix); ok && suffix != "" {
				signature = suffix
			}
			path := "messages." + strconv.Itoa(i) + ".tool_calls." + strconv.Itoa(j) + ".extra_content.google.thought_signature"
			next, err := sjson.SetBytes(updated, path, signature)
			if err != nil {
				return body
			}
			updated = next
		}
	}
	return updated
}
