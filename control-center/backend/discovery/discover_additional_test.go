package discovery

import (
	"net"
	"sync"
	"sync/atomic"
	"testing"

	"github.com/hashicorp/mdns"
)

func TestParseAgentInfoFromMDNSUsesIPv4AndTXTMetadata(t *testing.T) {
	entry := &mdns.ServiceEntry{
		Host:       "agent-host.local.",
		AddrV4:     net.ParseIP("192.0.2.10"),
		AddrV6:     net.ParseIP("2001:db8::10"),
		Port:       8080,
		InfoFields: []string{" NAME = TXT agent ", "OS= windows ", "Arch = amd64", "version= 1.2.3 ", "MAC=00:11:22:33:44:55", "ignored"},
	}

	got := ParseAgentInfoFromMDNS(entry)
	if got == nil {
		t.Fatal("ParseAgentInfoFromMDNS() returned nil")
	}
	want := AgentDiscovered{
		Host:    "192.0.2.10",
		Port:    8080,
		Name:    "TXT agent",
		OS:      "windows",
		Arch:    "amd64",
		Version: "1.2.3",
		MAC:     "00:11:22:33:44:55",
	}
	if *got != want {
		t.Fatalf("parsed agent = %#v, want %#v", *got, want)
	}
}

func TestParseAgentInfoFromMDNSFallsBackToIPv6AndHostName(t *testing.T) {
	entry := &mdns.ServiceEntry{
		Host:   "ipv6-agent.local.",
		AddrV6: net.ParseIP("2001:db8::20"),
		Port:   9090,
	}

	got := ParseAgentInfoFromMDNS(entry)
	if got == nil {
		t.Fatal("ParseAgentInfoFromMDNS() returned nil for IPv6 entry")
	}
	if got.Host != "2001:db8::20" || got.Port != 9090 || got.Name != "ipv6-agent.local" {
		t.Fatalf("IPv6 parsed agent = %#v", *got)
	}
}

func TestParseAgentInfoFromMDNSRejectsNilAndAddresslessEntries(t *testing.T) {
	if got := ParseAgentInfoFromMDNS(nil); got != nil {
		t.Fatalf("ParseAgentInfoFromMDNS(nil) = %#v, want nil", got)
	}
	if got := ParseAgentInfoFromMDNS(&mdns.ServiceEntry{Host: "missing-address", Port: 8080}); got != nil {
		t.Fatalf("addressless entry parsed as %#v, want nil", got)
	}
}

func TestParsePortAcceptsOnlyValidTCPPortRange(t *testing.T) {
	cases := []struct {
		input string
		want  int
	}{
		{input: "1", want: 1},
		{input: "8080", want: 8080},
		{input: "65535", want: 65535},
		{input: "", want: 0},
		{input: "0", want: 0},
		{input: "65536", want: 0},
		{input: "-1", want: 0},
		{input: "not-a-port", want: 0},
	}
	for _, tc := range cases {
		if got := ParsePort(tc.input); got != tc.want {
			t.Errorf("ParsePort(%q) = %d, want %d", tc.input, got, tc.want)
		}
	}
}

func TestHandleEntryDeduplicatesByHostAndPort(t *testing.T) {
	var discovered []AgentDiscovered
	d := NewDiscovery(func(info AgentDiscovered) {
		discovered = append(discovered, info)
	})

	first := &mdns.ServiceEntry{
		AddrV4:     net.ParseIP("192.0.2.30"),
		Port:       8080,
		Host:       "first.local.",
		InfoFields: []string{"name=first", "os=windows"},
	}
	duplicate := &mdns.ServiceEntry{
		AddrV4:     net.ParseIP("192.0.2.30"),
		Port:       8080,
		Host:       "updated.local.",
		InfoFields: []string{"name=updated", "os=linux"},
	}
	differentHost := &mdns.ServiceEntry{
		AddrV4: net.ParseIP("192.0.2.31"),
		Port:   8080,
		Host:   "second.local.",
	}
	differentPort := &mdns.ServiceEntry{
		AddrV4: net.ParseIP("192.0.2.30"),
		Port:   8081,
		Host:   "third.local.",
	}

	d.handleEntry(first)
	d.handleEntry(duplicate)
	d.handleEntry(differentHost)
	d.handleEntry(differentPort)

	if len(discovered) != 3 {
		t.Fatalf("discovered callback count = %d, want 3: %#v", len(discovered), discovered)
	}
	if discovered[0].Name != "first" || discovered[0].OS != "windows" {
		t.Fatalf("duplicate replaced first agent metadata: %#v", discovered[0])
	}
	if discovered[1].Host != "192.0.2.31" || discovered[2].Port != 8081 {
		t.Fatalf("distinct host/port identities = %#v", discovered)
	}
}

func TestHandleEntryConcurrentDuplicatesNotifyOnce(t *testing.T) {
	var callbacks atomic.Int32
	d := NewDiscovery(func(AgentDiscovered) {
		callbacks.Add(1)
	})
	entry := &mdns.ServiceEntry{AddrV4: net.ParseIP("192.0.2.40"), Port: 8080}

	const calls = 64
	var wg sync.WaitGroup
	for i := 0; i < calls; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			d.handleEntry(entry)
		}()
	}
	wg.Wait()
	if got := callbacks.Load(); got != 1 {
		t.Fatalf("concurrent duplicate callback count = %d, want 1", got)
	}
}

func TestNewDiscoveryStartsStoppedAndStopIsIdempotent(t *testing.T) {
	d := NewDiscovery(nil)
	if d.IsRunning() {
		t.Fatal("new Discovery reports running")
	}
	d.Stop()
	if d.IsRunning() {
		t.Fatal("Discovery reports running after Stop before Start")
	}
}
