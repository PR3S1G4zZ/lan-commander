package main

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeToken(t *testing.T, content string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "agent.token")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write token file: %v", err)
	}
	return path
}

func TestAuthTokenFileIsReadAndTrimmed(t *testing.T) {
	t.Setenv(authTokenEnv, "")
	f := &agentFlags{authTokenFile: writeToken(t, "  secret-from-file \r\n")}
	if err := validateAgentFlags(f); err != nil {
		t.Fatalf("a valid token file should pass validation: %v", err)
	}
	if f.authToken != "secret-from-file" {
		t.Fatalf("token = %q, want the trimmed file content", f.authToken)
	}
}

func TestAuthTokenFileRejectsBadInput(t *testing.T) {
	t.Setenv(authTokenEnv, "")
	missing := filepath.Join(t.TempDir(), "nope")
	for name, f := range map[string]*agentFlags{
		"missing file":      {authTokenFile: missing},
		"empty file":        {authTokenFile: writeToken(t, " \n")},
		"directory":         {authTokenFile: t.TempDir()},
		"with --auth-token": {authToken: "x", authTokenFile: writeToken(t, "y")},
		"with --no-auth":    {noAuth: true, authTokenFile: writeToken(t, "y")},
	} {
		if err := validateAgentFlags(f); err == nil {
			t.Errorf("%s: expected validation to fail", name)
		}
	}
}

func TestAuthTokenFallsBackToEnvironmentOnlyWhenNothingElseIsGiven(t *testing.T) {
	t.Setenv(authTokenEnv, "from-env")

	f := &agentFlags{}
	if err := validateAgentFlags(f); err != nil || f.authToken != "from-env" {
		t.Fatalf("env token not used: token=%q err=%v", f.authToken, err)
	}

	f = &agentFlags{authToken: "explicit"}
	if err := validateAgentFlags(f); err != nil || f.authToken != "explicit" {
		t.Fatalf("explicit token must win over the environment: token=%q err=%v", f.authToken, err)
	}

	if err := validateAgentFlags(&agentFlags{noAuth: true}); err == nil {
		t.Fatal("--no-auth with a token in the environment must be rejected, not silently open the agent")
	}
}

func TestMissingTokenErrorMentionsTheFileOption(t *testing.T) {
	t.Setenv(authTokenEnv, "")
	err := validateAgentFlags(&agentFlags{})
	if err == nil || !strings.Contains(err.Error(), "--auth-token-file") {
		t.Fatalf("error should point at --auth-token-file, got %v", err)
	}
}
