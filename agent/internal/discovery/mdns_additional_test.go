package discovery

import (
	"net"
	"testing"
)

func assertLocalIPv4OrNil(t *testing.T, ip net.IP) {
	t.Helper()
	if ip == nil {
		return
	}
	if ip.To4() == nil || ip.IsLoopback() {
		t.Fatalf("local IP = %v, want a non-loopback IPv4 address", ip)
	}
}

func TestGetLocalIPReturnsNonLoopbackIPv4OrNil(t *testing.T) {
	assertLocalIPv4OrNil(t, getLocalIP())
}

func TestNewMDNSServiceRejectsMissingPort(t *testing.T) {
	service, err := NewMDNSService(0, false)
	if err == nil {
		t.Fatal("mDNS service with port 0 was accepted")
	}
	if service != nil {
		t.Fatalf("service = %#v on constructor error, want nil", service)
	}
}

func TestNewMDNSServiceStartsAndClosesLocalAdvertisement(t *testing.T) {
	service, err := NewMDNSService(9474, true)
	if err != nil {
		t.Skipf("mDNS advertisement is unavailable in this environment: %v", err)
	}
	t.Cleanup(func() { _ = service.Close() })

	if service == nil || service.server == nil {
		t.Fatal("NewMDNSService returned no running server")
	}
	if service.port != 9474 {
		t.Fatalf("advertised port = %d, want 9474", service.port)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("close mDNS service: %v", err)
	}
	if err := service.Close(); err != nil {
		t.Fatalf("close mDNS service a second time: %v", err)
	}
}

func TestCloseOnUnstartedMDNSServiceIsSafe(t *testing.T) {
	if err := (&MDNSService{}).Close(); err != nil {
		t.Fatalf("close unstarted mDNS service: %v", err)
	}
}
