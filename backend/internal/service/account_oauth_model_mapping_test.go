package service

import "testing"

// An anthropic group that mixes in antigravity accounts receives antigravity-only names such as
// claude-opus-5-5-high; an Anthropic OAuth account maps them back with its own model_mapping.
func TestUsesAccountModelMapping(t *testing.T) {
	cases := []struct {
		account *Account
		want    bool
	}{
		{&Account{Platform: PlatformAnthropic, Type: AccountTypeAPIKey}, true},
		{&Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}, true},
		{&Account{Platform: PlatformAnthropic, Type: AccountTypeSetupToken}, true},
		{&Account{Platform: PlatformAnthropic, Type: AccountTypeServiceAccount}, false},
		{&Account{Platform: PlatformGemini, Type: AccountTypeOAuth}, false},
		{&Account{Platform: PlatformAntigravity, Type: AccountTypeOAuth}, false},
		{nil, false},
	}
	for _, c := range cases {
		if got := c.account.UsesAccountModelMapping(); got != c.want {
			t.Errorf("%+v: got %v, want %v", c.account, got, c.want)
		}
	}
}

func TestAnthropicOAuthAccountMapsTieredNames(t *testing.T) {
	account := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth, Credentials: map[string]any{
		"model_mapping": map[string]any{"claude-opus-5-5-high": "claude-opus-5-5", "claude-sonnet-5-5": "claude-sonnet-5-5"},
	}}
	if got := account.GetMappedModel("claude-opus-5-5-high"); got != "claude-opus-5-5" {
		t.Fatalf("tiered name: got %q", got)
	}
	if got := account.GetMappedModel("claude-sonnet-5-5"); got != "claude-sonnet-5-5" {
		t.Fatalf("native name: got %q", got)
	}
	svc := &GatewayService{}
	if !svc.isModelSupportedByAccount(account, "claude-opus-5-5-high") {
		t.Fatal("tiered name should be supported through the mapping")
	}
	if svc.isModelSupportedByAccount(account, "claude-haiku-5-5") {
		t.Fatal("a mapping is a whitelist: unlisted names are not supported")
	}
	plain := &Account{Platform: PlatformAnthropic, Type: AccountTypeOAuth}
	if got := plain.GetMappedModel("claude-opus-4-5"); got != "claude-opus-4-5" {
		t.Fatalf("no mapping must leave the name for OAuth normalization: got %q", got)
	}
}
