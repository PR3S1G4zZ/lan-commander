package system

import (
	"net"
	"os"
	"runtime"
	"sync"

	"github.com/shirou/gopsutil/v4/cpu"
	"github.com/shirou/gopsutil/v4/disk"
	"github.com/shirou/gopsutil/v4/host"
	"github.com/shirou/gopsutil/v4/mem"

	"github.com/mediacode/lan-commander/agent/internal/protocol"
)

// Monitor provides system information with caching for static data.
type Monitor struct {
	hostname string
	os       string
	platform string
	arch     string
	agentVer string

	once sync.Once
}

// NewMonitor creates a Monitor and caches immutable system properties.
func NewMonitor() *Monitor {
	m := &Monitor{
		agentVer: "1.0.0",
	}
	m.initCache()
	return m
}

func (m *Monitor) initCache() {
	m.once.Do(func() {
		m.hostname, _ = os.Hostname()
		m.os = runtime.GOOS
		m.arch = runtime.GOARCH
		if info, err := host.Info(); err == nil {
			m.platform = info.Platform
			if info.PlatformVersion != "" {
				m.platform += " " + info.PlatformVersion
			}
		} else {
			m.platform = m.os
		}
	})
}

// GetSystemInfo collects live system metrics and returns a complete payload.
func (m *Monitor) GetSystemInfo() protocol.SystemInfoPayload {
	cpuPercent, _ := cpu.Percent(0, false)
	cpuCores, _ := cpu.Counts(true)
	cpuInfo, _ := cpu.Info()

	cpuPercentVal := 0.0
	if len(cpuPercent) > 0 {
		cpuPercentVal = cpuPercent[0]
	}

	cpuModel := ""
	if len(cpuInfo) > 0 {
		cpuModel = cpuInfo[0].ModelName
	}

	memInfo, _ := mem.VirtualMemory()

	diskParts, _ := disk.Partitions(false)
	diskInfos := make([]protocol.DiskInfo, 0, len(diskParts))
	for _, d := range diskParts {
		usage, err := disk.Usage(d.Mountpoint)
		if err != nil {
			continue
		}
		diskInfos = append(diskInfos, protocol.DiskInfo{
			Mount:   d.Mountpoint,
			FSType:  d.Fstype,
			Total:   usage.Total,
			Used:    usage.Used,
			Free:    usage.Free,
			Percent: usage.UsedPercent,
		})
	}

	uptime, _ := host.Uptime()

	ip, mac := m.getPrimaryNet()

	var memUsed, memFree uint64
	if memInfo != nil {
		memUsed = memInfo.Used
		memFree = memInfo.Free
	}

	var memTotal uint64
	var memPercent float64
	if memInfo != nil {
		memTotal = memInfo.Total
		memPercent = memInfo.UsedPercent
	}

	return protocol.SystemInfoPayload{
		Hostname: m.hostname,
		OS:       m.os,
		Platform: m.platform,
		Arch:     m.arch,
		CPU: protocol.CPUInfo{
			Percent: cpuPercentVal,
			Cores:   cpuCores,
			Model:   cpuModel,
		},
		Memory: protocol.MemoryInfo{
			Total:   memTotal,
			Used:    memUsed,
			Free:    memFree,
			Percent: memPercent,
		},
		Disks:        diskInfos,
		Uptime:       uptime,
		Net:          protocol.NetInfo{IP: ip, MAC: mac, Hostname: m.hostname},
		AgentVersion: m.agentVer,
	}
}

// getPrimaryNet returns the first non-loopback IPv4 address and MAC.
func (m *Monitor) getPrimaryNet() (ip, mac string) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return "", ""
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
				if iface.HardwareAddr != nil {
					return ipnet.IP.String(), iface.HardwareAddr.String()
				}
				return ipnet.IP.String(), ""
			}
		}
	}
	return "", ""
}
