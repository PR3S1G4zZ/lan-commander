package wol

import (
	"bytes"
	"net"
	"strings"
	"testing"
	"time"
)

func TestParseMACAcceptsCommonRepresentations(t *testing.T) {
	want := net.HardwareAddr{0x00, 0x11, 0x22, 0x33, 0x44, 0xaa}
	for _, input := range []string{
		"00:11:22:33:44:aa",
		"00-11-22-33-44-AA",
		"0011223344aa",
		"0011.2233.44aa",
	} {
		got, err := ParseMAC(input)
		if err != nil {
			t.Errorf("ParseMAC(%q) error = %v", input, err)
			continue
		}
		if !bytes.Equal(got, want) {
			t.Errorf("ParseMAC(%q) = %v, want %v", input, got, want)
		}
		if !ValidateMAC(input) {
			t.Errorf("ValidateMAC(%q) = false, want true", input)
		}
	}
}

func TestParseMACRejectsInvalidValues(t *testing.T) {
	for _, input := range []string{
		"",
		"00:11:22:33:44",
		"00:11:22:33:44:55:66",
		"00112233445g",
		"00:11:22:33:44:5z",
		" 00:11:22:33:44:55",
	} {
		if _, err := ParseMAC(input); err == nil {
			t.Errorf("ParseMAC(%q) succeeded, want error", input)
		}
		if ValidateMAC(input) {
			t.Errorf("ValidateMAC(%q) = true, want false", input)
		}
	}
}

func TestSendWritesCanonicalMagicPacketToLoopback(t *testing.T) {
	listener, err := net.ListenUDP("udp4", &net.UDPAddr{IP: net.ParseIP("127.0.0.1"), Port: 9})
	if err != nil {
		t.Skipf("loopback UDP port 9 is unavailable: %v", err)
	}
	t.Cleanup(func() { _ = listener.Close() })

	if err := NewSenderWithLocalAddr("127.0.0.1").Send("00:11:22:33:44:55", "127.0.0.1"); err != nil {
		t.Fatalf("Send() error = %v", err)
	}
	if err := listener.SetReadDeadline(time.Now().Add(2 * time.Second)); err != nil {
		t.Fatalf("set UDP read deadline: %v", err)
	}
	packet := make([]byte, 2048)
	n, source, err := listener.ReadFromUDP(packet)
	if err != nil {
		t.Fatalf("read magic packet: %v", err)
	}
	packet = packet[:n]
	if source.IP.String() != "127.0.0.1" {
		t.Fatalf("packet source IP = %s, want loopback", source.IP)
	}
	if len(packet) != 102 {
		t.Fatalf("magic packet length = %d, want 102", len(packet))
	}
	for i, value := range packet[:6] {
		if value != 0xff {
			t.Fatalf("magic packet prefix byte %d = %#x, want 0xff", i, value)
		}
	}
	mac := []byte{0x00, 0x11, 0x22, 0x33, 0x44, 0x55}
	for repetition := 0; repetition < 16; repetition++ {
		start := 6 + repetition*len(mac)
		if !bytes.Equal(packet[start:start+len(mac)], mac) {
			t.Fatalf("magic packet repetition %d = %x, want %x", repetition, packet[start:start+len(mac)], mac)
		}
	}
}

func TestSendRejectsInvalidMACBeforeNetworkAccess(t *testing.T) {
	err := NewSenderWithLocalAddr("not-an-ip").Send("invalid", "not-an-ip")
	if err == nil {
		t.Fatal("Send() with invalid MAC succeeded")
	}
	if !strings.Contains(err.Error(), `invalid MAC address "invalid"`) {
		t.Fatalf("invalid MAC error = %q, want validation context", err)
	}
}

func TestSendRejectsInvalidBroadcastIP(t *testing.T) {
	err := NewSenderWithLocalAddr("127.0.0.1").Send("00:11:22:33:44:55", "not-an-ip")
	if err == nil {
		t.Fatal("Send() succeeded with an invalid broadcast IP")
	}
}

func TestSendRejectsInvalidLocalIP(t *testing.T) {
	err := NewSenderWithLocalAddr("not-an-ip").Send("00:11:22:33:44:55", "127.0.0.1")
	if err == nil {
		t.Fatal("Send() succeeded with an invalid local IP")
	}
}
