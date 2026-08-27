package system

import (
	"math"
	"net"
	"os"
	"runtime"
	"testing"
)

func assertSystemPercentage(t *testing.T, name string, value float64) {
	t.Helper()
	if math.IsNaN(value) || math.IsInf(value, 0) || value < 0 || value > 100 {
		t.Fatalf("%s = %v, want a finite percentage in [0, 100]", name, value)
	}
}

func assertPrimaryNetworkResult(t *testing.T, ip, mac string) {
	t.Helper()
	if ip == "" {
		if mac != "" {
			t.Fatalf("MAC address %q was reported without an IP address", mac)
		}
		return
	}

	parsedIP := net.ParseIP(ip)
	if parsedIP == nil || parsedIP.To4() == nil || parsedIP.IsLoopback() {
		t.Fatalf("primary IP = %q, want a non-loopback IPv4 address", ip)
	}
	if mac != "" {
		parsedMAC, err := net.ParseMAC(mac)
		if err != nil || len(parsedMAC) == 0 {
			t.Fatalf("primary MAC = %q, want a valid hardware address: %v", mac, err)
		}
	}
}

func TestMonitorReportsStableHostPropertiesAndMetricInvariants(t *testing.T) {
	monitor := NewMonitor()
	first := monitor.GetSystemInfo()
	second := monitor.GetSystemInfo()

	if first.OS != runtime.GOOS {
		t.Fatalf("OS = %q, want %q", first.OS, runtime.GOOS)
	}
	if first.Arch != runtime.GOARCH {
		t.Fatalf("arch = %q, want %q", first.Arch, runtime.GOARCH)
	}
	if first.Platform == "" {
		t.Fatal("platform is empty, want the host platform or OS fallback")
	}
	if first.AgentVersion == "" {
		t.Fatal("agent version is empty")
	}
	if hostname, err := os.Hostname(); err == nil && first.Hostname != hostname {
		t.Fatalf("hostname = %q, want %q", first.Hostname, hostname)
	}

	if first.Hostname != second.Hostname || first.OS != second.OS || first.Platform != second.Platform || first.Arch != second.Arch || first.AgentVersion != second.AgentVersion {
		t.Fatalf("cached host properties changed between samples: first=%+v second=%+v", first, second)
	}
	if first.Net.Hostname != first.Hostname || second.Net.Hostname != second.Hostname {
		t.Fatalf("network hostnames = %q and %q, want the cached hostnames %q and %q", first.Net.Hostname, second.Net.Hostname, first.Hostname, second.Hostname)
	}

	assertSystemPercentage(t, "CPU percent", first.CPU.Percent)
	if first.CPU.Cores < 0 {
		t.Fatalf("CPU cores = %d, want non-negative", first.CPU.Cores)
	}
	assertSystemPercentage(t, "memory percent", first.Memory.Percent)
	if first.Memory.Total > 0 {
		if first.Memory.Used > first.Memory.Total {
			t.Fatalf("memory used = %d exceeds total = %d", first.Memory.Used, first.Memory.Total)
		}
		if first.Memory.Free > first.Memory.Total {
			t.Fatalf("memory free = %d exceeds total = %d", first.Memory.Free, first.Memory.Total)
		}
	}

	for i, disk := range first.Disks {
		if disk.Mount == "" {
			t.Fatalf("disk %d has an empty mount point", i)
		}
		assertSystemPercentage(t, "disk percent", disk.Percent)
		if disk.Total > 0 {
			if disk.Used > disk.Total {
				t.Fatalf("disk %q used = %d exceeds total = %d", disk.Mount, disk.Used, disk.Total)
			}
			if disk.Free > disk.Total {
				t.Fatalf("disk %q free = %d exceeds total = %d", disk.Mount, disk.Free, disk.Total)
			}
		}
	}

	assertPrimaryNetworkResult(t, first.Net.IP, first.Net.MAC)
}

func TestMonitorPrimaryNetworkResultIsIPv4OrEmpty(t *testing.T) {
	monitor := NewMonitor()
	ip, mac := monitor.getPrimaryNet()
	assertPrimaryNetworkResult(t, ip, mac)
}
