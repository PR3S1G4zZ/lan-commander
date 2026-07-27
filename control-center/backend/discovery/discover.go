package discovery

import (
	"log"
	"net"
	"strconv"
	"strings"
	"sync"
	"time"

	"github.com/hashicorp/mdns"
)

const (
	serviceName = "_lan-commander._tcp"
	scanInterval = 30 * time.Second
)

// AgentDiscovered holds information about a discovered agent.
type AgentDiscovered struct {
	Host     string `json:"host"`
	Port     int    `json:"port"`
	Name     string `json:"name"`
	OS       string `json:"os"`
	Arch     string `json:"arch"`
	Version  string `json:"version"`
	MAC      string `json:"mac"`
}

// agentKey uniquely identifies a discovered agent by IP:Port.
type agentKey struct {
	host string
	port int
}

// Discovery handles mDNS-based agent discovery on the LAN.
type Discovery struct {
	onAgentDiscovered func(AgentDiscovered)
	known             map[agentKey]bool
	mu                sync.RWMutex
	stopCh            chan struct{}
	running           bool
}

// NewDiscovery creates a new mDNS discovery service.
// The callback is invoked whenever a new agent is discovered.
func NewDiscovery(onAgentDiscovered func(AgentDiscovered)) *Discovery {
	return &Discovery{
		onAgentDiscovered: onAgentDiscovered,
		known:             make(map[agentKey]bool),
		stopCh:            make(chan struct{}),
	}
}

// Start begins the mDNS discovery loop. It performs an immediate scan and
// then re-scans every 30 seconds.
func (d *Discovery) Start() {
	d.mu.Lock()
	if d.running {
		d.mu.Unlock()
		return
	}
	d.running = true
	d.mu.Unlock()

	go d.loop()
}

// Stop gracefully stops the discovery loop.
func (d *Discovery) Stop() {
	d.mu.Lock()
	defer d.mu.Unlock()

	if d.running {
		close(d.stopCh)
		d.running = false
	}
}

// IsRunning returns whether the discovery loop is active.
func (d *Discovery) IsRunning() bool {
	d.mu.RLock()
	defer d.mu.RUnlock()
	return d.running
}

func (d *Discovery) loop() {
	// Perform initial scan immediately
	d.scan()

	ticker := time.NewTicker(scanInterval)
	defer ticker.Stop()

	for {
		select {
		case <-ticker.C:
			d.scan()
		case <-d.stopCh:
			return
		}
	}
}

func (d *Discovery) scan() {
	entriesCh := make(chan *mdns.ServiceEntry, 20)

	go func() {
		for entry := range entriesCh {
			d.handleEntry(entry)
		}
	}()

	// Query for the LAN Commander service
	err := mdns.Query(&mdns.QueryParam{
		Service:   serviceName,
		Timeout:   time.Second * 5,
		Entries:   entriesCh,
		Interface: nil, // all interfaces
	})
	if err != nil {
		log.Printf("discovery: mDNS query failed: %v", err)
	}

	close(entriesCh)
}

func (d *Discovery) handleEntry(entry *mdns.ServiceEntry) {
	info := ParseAgentInfoFromMDNS(entry)
	if info == nil {
		return
	}

	key := agentKey{host: info.Host, port: info.Port}

	d.mu.Lock()
	if d.known[key] {
		d.mu.Unlock()
		return
	}
	d.known[key] = true
	d.mu.Unlock()

	log.Printf("discovery: new agent found: %s @ %s:%d (os=%s, arch=%s)",
		info.Name, info.Host, info.Port, info.OS, info.Arch)

	if d.onAgentDiscovered != nil {
		d.onAgentDiscovered(*info)
	}
}

// ParseAgentInfoFromMDNS extracts agent information from an mDNS service entry.
// It reads properties from the TXT records and info field.
func ParseAgentInfoFromMDNS(entry *mdns.ServiceEntry) *AgentDiscovered {
	if entry == nil {
		return nil
	}

	info := &AgentDiscovered{}

	// Determine host (use IPv4 if available)
	if entry.AddrV4 != nil {
		info.Host = entry.AddrV4.String()
	} else if entry.AddrV6 != nil {
		info.Host = entry.AddrV6.String()
	} else {
		return nil
	}

	info.Port = entry.Port

	// Use hostname from service entry
	if entry.Host != "" {
		info.Name = strings.TrimSuffix(entry.Host, ".")
	}

	// Parse TXT records for metadata
	for _, txt := range entry.InfoFields {
		parts := strings.SplitN(txt, "=", 2)
		if len(parts) != 2 {
			continue
		}
		key := strings.ToLower(strings.TrimSpace(parts[0]))
		value := strings.TrimSpace(parts[1])

		switch key {
		case "name":
			info.Name = value
		case "os":
			info.OS = value
		case "arch":
			info.Arch = value
		case "version":
			info.Version = value
		case "mac":
			info.MAC = value
		}
	}

	return info
}

// ResolveHostname attempts to resolve a hostname from the given IP.
func ResolveHostname(ip string) string {
	names, err := net.LookupAddr(ip)
	if err != nil || len(names) == 0 {
		return ip
	}
	return strings.TrimSuffix(names[0], ".")
}

// ParsePort parses a port number from a string, returning 0 on failure.
func ParsePort(portStr string) int {
	port, err := strconv.Atoi(portStr)
	if err != nil {
		return 0
	}
	if port < 1 || port > 65535 {
		return 0
	}
	return port
}
