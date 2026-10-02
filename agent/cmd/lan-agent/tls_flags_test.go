package main

import (
	"strings"
	"testing"
)

func TestValidateAgentFlagsRequiresTLSCertificateAndKeyTogether(t *testing.T) {
	for name, f := range map[string]*agentFlags{
		"certificate without key": {authToken: "t", tlsCert: "cert.pem"},
		"key without certificate": {authToken: "t", tlsKey: "key.pem"},
		"no-auth with only cert":  {noAuth: true, tlsCert: "cert.pem"},
	} {
		err := validateAgentFlags(f)
		if err == nil {
			t.Errorf("%s: expected startup validation to fail instead of serving plaintext", name)
			continue
		}
		if !strings.Contains(err.Error(), "--tls-cert") || !strings.Contains(err.Error(), "--tls-key") {
			t.Errorf("%s: error is not actionable: %v", name, err)
		}
	}

	if err := validateAgentFlags(&agentFlags{authToken: "t", tlsCert: "cert.pem", tlsKey: "key.pem"}); err != nil {
		t.Fatalf("a complete TLS configuration should pass validation: %v", err)
	}
}
