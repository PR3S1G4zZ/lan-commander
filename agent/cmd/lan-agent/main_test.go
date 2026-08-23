package main

import (
	"flag"
	"strings"
	"testing"
)

func TestParseAgentFlagsSupportsExplicitNoAuth(t *testing.T) {
	fs := flag.NewFlagSet("lan-agent", flag.ContinueOnError)
	flags := parseAgentFlags(fs, []string{"--no-auth"})

	if !flags.noAuth {
		t.Fatal("--no-auth should explicitly enable laboratory mode")
	}
}

func TestValidateAgentFlagsRequiresAuthenticationByDefault(t *testing.T) {
	if err := validateAgentFlags(&agentFlags{}); err == nil {
		t.Fatal("expected missing authentication token to fail startup validation")
	} else if !strings.Contains(err.Error(), "--auth-token") || !strings.Contains(err.Error(), "--no-auth") {
		t.Fatalf("startup error is not actionable: %v", err)
	}

	if err := validateAgentFlags(&agentFlags{authToken: "test-token"}); err != nil {
		t.Fatalf("a configured token should pass validation: %v", err)
	}

	if err := validateAgentFlags(&agentFlags{noAuth: true}); err != nil {
		t.Fatalf("explicit laboratory mode should pass validation: %v", err)
	}

	if err := validateAgentFlags(&agentFlags{authToken: "test-token", noAuth: true}); err == nil {
		t.Fatal("conflicting authentication flags should fail validation")
	}
}
