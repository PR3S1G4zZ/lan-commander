package discovery

import (
	"fmt"
	"net"
	"os"
	"runtime"

	"github.com/hashicorp/mdns"
)

const (
	// ServiceName is the mDNS service type.
	ServiceName = "_lan-commander._tcp"
	// Domain is the mDNS domain.
	Domain = "local."
	// TTLSeconds is the mDNS advertisement TTL.
	TTLSeconds = 120
	// AgentVersion is published in TXT records.
	AgentVersion = "1.0.0"
)

// MDNSService manages mDNS advertisement for the agent.
type MDNSService struct {
	server *mdns.Server
	port   int
}

// NewMDNSService creates an mDNS service advertisement.
// Set hasAuth to true when an auth token is configured.
func NewMDNSService(port int, hasAuth bool) (*MDNSService, error) {
	hostname, err := os.Hostname()
	if err != nil {
		return nil, fmt.Errorf("cannot get hostname: %w", err)
	}

	// Get local IP to advertise
	ip := getLocalIP()

	// Build TXT records
	info := []string{
		fmt.Sprintf("version=%s", AgentVersion),
		fmt.Sprintf("os=%s", runtime.GOOS),
		fmt.Sprintf("arch=%s", runtime.GOARCH),
		fmt.Sprintf("auth=%t", hasAuth),
	}

	// Build IP list (include local IP if found)
	ips := []net.IP{}
	if ip != nil {
		ips = append(ips, ip)
	}

	// Create the mDNS service
	service, err := mdns.NewMDNSService(
		hostname,  // instance
		ServiceName, // service
		Domain,    // domain
		"",        // host (empty = default)
		port,      // port
		ips,       // IPs to advertise
		info,      // TXT records
	)
	if err != nil {
		return nil, fmt.Errorf("failed to create mDNS service: %w", err)
	}

	// Start the mDNS server
	server, err := mdns.NewServer(&mdns.Config{Zone: service})
	if err != nil {
		return nil, fmt.Errorf("failed to start mDNS server: %w", err)
	}

	return &MDNSService{
		server: server,
		port:   port,
	}, nil
}

// Close stops the mDNS advertisement.
func (m *MDNSService) Close() error {
	if m.server != nil {
		return m.server.Shutdown()
	}
	return nil
}

// getLocalIP returns the first non-loopback IPv4 address.
func getLocalIP() net.IP {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil
	}
	for _, iface := range ifaces {
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() {
				continue
			}
			if ipnet.IP.To4() != nil {
				return ipnet.IP
			}
		}
	}
	return nil
}
