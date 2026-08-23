package main

import (
	"testing"

	"control-center/backend/client"
)

func TestSessionForConnectionOptionsPersistsSecureTransport(t *testing.T) {
	s := sessionForConnectionOptions("10.0.0.4", 9474, "lab-agent", client.ConnectOptions{
		AuthToken:  "secret-token",
		TLS:        true,
		CAFile:     `C:\certs\lan-ca.pem`,
		ServerName: "agent.lab",
	})

	if s.AuthToken != "secret-token" || !s.TLS || s.CAFile != `C:\certs\lan-ca.pem` || s.ServerName != "agent.lab" {
		t.Fatalf("secure connection options were not persisted in session: %#v", s)
	}
}
