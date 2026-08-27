package wol

import (
	"fmt"
	"net"
	"strings"
)

// Sender handles sending Wake-on-LAN magic packets.
type Sender struct {
	localAddr string
}

// NewSender creates a new WOL sender.
func NewSender() *Sender {
	return &Sender{}
}

// NewSenderWithLocalAddr creates a new WOL sender with a specific local address binding.
func NewSenderWithLocalAddr(localAddr string) *Sender {
	return &Sender{localAddr: localAddr}
}

// Send sends a Wake-on-LAN magic packet to the specified MAC address on the given broadcast IP.
// If broadcastIP is empty, it defaults to "255.255.255.255".
func (s *Sender) Send(macAddr string, broadcastIP string) error {
	mac, err := ParseMAC(macAddr)
	if err != nil {
		return fmt.Errorf("invalid MAC address %q: %w", macAddr, err)
	}

	if broadcastIP == "" {
		broadcastIP = "255.255.255.255"
	}
	broadcastAddr := net.ParseIP(broadcastIP)
	if broadcastAddr == nil || broadcastAddr.To4() == nil {
		return fmt.Errorf("invalid broadcast IP %q", broadcastIP)
	}
	broadcastAddr = broadcastAddr.To4()

	// Build magic packet: 6 bytes of 0xFF + 16 repetitions of the MAC address
	packet := make([]byte, 102)
	for i := 0; i < 6; i++ {
		packet[i] = 0xFF
	}
	for i := 1; i <= 16; i++ {
		copy(packet[i*6:], mac)
	}

	// Resolve UDP address
	addr := &net.UDPAddr{
		IP:   broadcastAddr,
		Port: 9,
	}

	// Dial UDP connection
	var conn net.Conn
	if s.localAddr != "" {
		localIP := net.ParseIP(s.localAddr)
		if localIP == nil || localIP.To4() == nil {
			return fmt.Errorf("invalid local IP %q", s.localAddr)
		}
		localAddr := &net.UDPAddr{
			IP:   localIP.To4(),
			Port: 0,
		}
		conn, err = net.DialUDP("udp", localAddr, addr)
	} else {
		conn, err = net.DialUDP("udp", nil, addr)
	}
	if err != nil {
		return fmt.Errorf("failed to dial UDP: %w", err)
	}
	defer conn.Close()

	n, err := conn.Write(packet)
	if err != nil {
		return fmt.Errorf("failed to send WOL packet: %w", err)
	}
	if n != len(packet) {
		return fmt.Errorf("incomplete write: sent %d of %d bytes", n, len(packet))
	}

	return nil
}

// SendToAll sends WOL packets to all broadcast interfaces.
func (s *Sender) SendToAll(macAddr string) error {
	interfaces, err := net.Interfaces()
	if err != nil {
		return fmt.Errorf("failed to list interfaces: %w", err)
	}

	var lastErr error
	for _, iface := range interfaces {
		if iface.Flags&net.FlagUp == 0 || iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}
		for _, addr := range addrs {
			ipnet, ok := addr.(*net.IPNet)
			if !ok || ipnet.IP.IsLoopback() || ipnet.IP.To4() == nil {
				continue
			}
			broadcast := make(net.IP, 4)
			for i := range ipnet.IP.To4() {
				broadcast[i] = ipnet.IP.To4()[i] | ^ipnet.Mask[i]
			}
			if err := s.Send(macAddr, broadcast.String()); err != nil {
				lastErr = err
			}
		}
	}
	return lastErr
}

// ParseMAC parses a MAC address string, accepting ":" and "-" as separators
// or no separators (12 hex characters).
func ParseMAC(macStr string) (net.HardwareAddr, error) {
	// Remove common separators
	macStr = strings.ReplaceAll(macStr, "-", "")
	macStr = strings.ReplaceAll(macStr, ":", "")
	macStr = strings.ReplaceAll(macStr, ".", "")

	if len(macStr) != 12 {
		return nil, fmt.Errorf("MAC address must be 12 hex characters, got %d", len(macStr))
	}

	// Reinsert colons for parsing
	parts := make([]string, 6)
	for i := 0; i < 6; i++ {
		parts[i] = macStr[i*2 : i*2+2]
	}
	return net.ParseMAC(strings.Join(parts, ":"))
}

// ValidateMAC checks if a MAC address string is valid.
func ValidateMAC(macStr string) bool {
	_, err := ParseMAC(macStr)
	return err == nil
}
