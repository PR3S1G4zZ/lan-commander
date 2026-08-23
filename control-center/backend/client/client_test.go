package client

import (
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"reflect"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"control-center/backend/protocol"

	"github.com/gorilla/websocket"
)

func tlsGreetingServer(t *testing.T, closeFirst bool) (*httptest.Server, string, *atomic.Int32) {
	t.Helper()
	var connections atomic.Int32
	upgrader := websocket.Upgrader{}
	server := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		conn, err := upgrader.Upgrade(w, r, nil)
		if err != nil {
			return
		}
		count := connections.Add(1)
		if err := conn.WriteJSON(protocol.Message{Type: protocol.MsgAgentInfo}); err != nil {
			_ = conn.Close()
			return
		}
		if closeFirst && count == 1 {
			_ = conn.Close()
		}
	}))
	t.Cleanup(server.Close)

	certificate := server.Certificate()
	serverName := certificate.Subject.CommonName
	if len(certificate.DNSNames) > 0 {
		serverName = certificate.DNSNames[0]
	}
	if serverName == "" {
		t.Fatal("test certificate has no usable hostname")
	}

	return server, serverName, &connections
}

func writeTestCA(t *testing.T, certificate *x509.Certificate) string {
	t.Helper()
	path := t.TempDir() + "\\ca.pem"
	pemBytes := pem.EncodeToMemory(&pem.Block{Type: "CERTIFICATE", Bytes: certificate.Raw})
	if err := os.WriteFile(path, pemBytes, 0o600); err != nil {
		t.Fatalf("write test CA: %v", err)
	}
	return path
}

func serverEndpoint(t *testing.T, server *httptest.Server) (string, int) {
	t.Helper()
	u, err := url.Parse(server.URL)
	if err != nil {
		t.Fatalf("parse test server URL: %v", err)
	}
	host, portText, ok := strings.Cut(u.Host, ":")
	if !ok {
		t.Fatalf("test server address has no port: %q", u.Host)
	}
	var port int
	if _, err := fmt.Sscanf(portText, "%d", &port); err != nil {
		t.Fatalf("parse test server port: %v", err)
	}
	return host, port
}

func connectWithOptionsReflection(t *testing.T, manager *Manager, host string, port int, fields map[string]any) (string, error) {
	t.Helper()
	method := reflect.ValueOf(manager).MethodByName("ConnectWithOptions")
	if !method.IsValid() {
		t.Fatal("Manager.ConnectWithOptions is not implemented")
	}
	if method.Type().NumIn() != 3 {
		t.Fatalf("ConnectWithOptions argument count = %d, want 3", method.Type().NumIn())
	}
	options := reflect.New(method.Type().In(2)).Elem()
	setCount := 0
	for name, value := range fields {
		field := options.FieldByName(name)
		if !field.IsValid() {
			continue
		}
		incoming := reflect.ValueOf(value)
		if !incoming.Type().AssignableTo(field.Type()) {
			t.Fatalf("ConnectOptions.%s type = %s, cannot assign %s", name, field.Type(), incoming.Type())
		}
		field.Set(incoming)
		setCount++
	}
	if setCount != len(fields) {
		t.Fatalf("ConnectOptions is missing one or more expected fields: %#v", fields)
	}

	results := method.Call([]reflect.Value{reflect.ValueOf(host), reflect.ValueOf(port), options})
	if len(results) != 2 {
		t.Fatalf("ConnectWithOptions returned %d values, want 2", len(results))
	}
	var err error
	if !results[1].IsNil() {
		err = results[1].Interface().(error)
	}
	return results[0].String(), err
}

func TestConnectWithOptionsUsesTLSCAAndVerifiesServerName(t *testing.T) {
	server, serverName, _ := tlsGreetingServer(t, false)
	host, port := serverEndpoint(t, server)
	manager := NewManager(nil)
	t.Cleanup(manager.CloseAll)

	_, err := connectWithOptionsReflection(t, manager, host, port, map[string]any{
		"TLS":        true,
		"CAFile":     writeTestCA(t, server.Certificate()),
		"ServerName": serverName,
		"AuthToken":  "",
	})
	if err != nil {
		t.Fatalf("TLS connection with trusted CA failed: %v", err)
	}

	wrongManager := NewManager(nil)
	t.Cleanup(wrongManager.CloseAll)
	if _, err := connectWithOptionsReflection(t, wrongManager, host, port, map[string]any{
		"TLS":        true,
		"CAFile":     writeTestCA(t, server.Certificate()),
		"ServerName": "wrong.example.invalid",
		"AuthToken":  "",
	}); err == nil {
		t.Fatal("TLS connection with an invalid server name was accepted")
	}
}

func TestReconnectReusesTheExactTLSOptions(t *testing.T) {
	server, serverName, connections := tlsGreetingServer(t, true)
	host, port := serverEndpoint(t, server)
	manager := NewManager(nil)
	manager.reconnectDelay = 5 * time.Millisecond
	t.Cleanup(manager.CloseAll)

	if _, err := connectWithOptionsReflection(t, manager, host, port, map[string]any{
		"TLS":        true,
		"CAFile":     writeTestCA(t, server.Certificate()),
		"ServerName": serverName,
		"AuthToken":  "",
	}); err != nil {
		t.Fatalf("initial TLS connection failed: %v", err)
	}

	deadline := time.Now().Add(2 * time.Second)
	for connections.Load() < 2 && time.Now().Before(deadline) {
		time.Sleep(5 * time.Millisecond)
	}
	if got := connections.Load(); got < 2 {
		t.Fatalf("reconnect did not reuse TLS options; server saw %d connections", got)
	}
}
