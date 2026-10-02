package main

import (
	"encoding/json"
	"strings"
	"testing"

	"control-center/backend/client"
)

func TestRedactAgentTokensRemovesSecretsBeforeTheyReachTheRenderer(t *testing.T) {
	agents := []client.AgentInfo{
		{ID: "a1", Host: "10.0.0.4", Port: 9474, AuthToken: "super-secret-token", Name: "lab-1"},
		{ID: "a2", Host: "10.0.0.5", Port: 9474, AuthToken: "another-secret"},
	}

	redacted := redactAgentTokens(agents)

	encoded, err := json.Marshal(redacted)
	if err != nil {
		t.Fatalf("marshal redacted agents: %v", err)
	}
	for _, secret := range []string{"super-secret-token", "another-secret", "auth_token"} {
		if strings.Contains(string(encoded), secret) {
			t.Fatalf("serialized agents still contain %q: %s", secret, encoded)
		}
	}
	if redacted[0].ID != "a1" || redacted[0].Name != "lab-1" || redacted[1].Host != "10.0.0.5" {
		t.Fatalf("redaction changed non-secret fields: %#v", redacted)
	}
}

func TestRedactAgentTokensReturnsEmptySliceForNil(t *testing.T) {
	if got := redactAgentTokens(nil); got == nil || len(got) != 0 {
		t.Fatalf("redactAgentTokens(nil) = %#v, want empty non-nil slice", got)
	}
}
