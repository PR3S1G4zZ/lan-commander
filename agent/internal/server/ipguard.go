package server

import (
	"net"
	"sync"
	"time"
)

const (
	// MaxConnsPerIP caps simultaneous connections from a single address so one
	// host cannot use up the MaxClients slots.
	MaxConnsPerIP = 16
	// MaxAuthFailuresPerIP is how many failed authentications an address may
	// accumulate within AuthFailureWindow before new connections are refused.
	// The per-connection limit alone is bypassed by simply reconnecting.
	MaxAuthFailuresPerIP = 20
	// AuthFailureWindow is the period over which failures are counted.
	AuthFailureWindow = time.Minute
	// AuthBlockDuration is how long an address stays refused once it trips the limit.
	AuthBlockDuration = time.Minute
)

type failureRecord struct {
	count        int
	windowStart  time.Time
	blockedUntil time.Time
}

// ipGuard tracks live connections and authentication failures per remote IP.
type ipGuard struct {
	mu          sync.Mutex
	active      map[string]int
	failures    map[string]*failureRecord
	maxActive   int
	maxFailures int
	window      time.Duration
	block       time.Duration
	now         func() time.Time
}

func newIPGuard() *ipGuard {
	return &ipGuard{
		active:      make(map[string]int),
		failures:    make(map[string]*failureRecord),
		maxActive:   MaxConnsPerIP,
		maxFailures: MaxAuthFailuresPerIP,
		window:      AuthFailureWindow,
		block:       AuthBlockDuration,
		now:         time.Now,
	}
}

// admit reserves a connection slot for ip. When it returns false the reason is
// suitable for logs; no slot was reserved and release must not be called.
func (g *ipGuard) admit(ip string) (bool, string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	if rec := g.failures[ip]; rec != nil && now.Before(rec.blockedUntil) {
		return false, "too many failed authentication attempts"
	}
	if g.active[ip] >= g.maxActive {
		return false, "too many simultaneous connections"
	}
	g.active[ip]++
	return true, ""
}

// release frees a slot previously granted by admit.
func (g *ipGuard) release(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	if g.active[ip] <= 1 {
		delete(g.active, ip)
		return
	}
	g.active[ip]--
}

// fail records a failed authentication and blocks the address once it exceeds
// the allowed number within the window.
func (g *ipGuard) fail(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()

	now := g.now()
	g.pruneLocked(now)
	rec := g.failures[ip]
	if rec == nil || now.Sub(rec.windowStart) > g.window {
		rec = &failureRecord{windowStart: now}
		g.failures[ip] = rec
	}
	rec.count++
	if rec.count >= g.maxFailures {
		rec.blockedUntil = now.Add(g.block)
		rec.count = 0
		rec.windowStart = now
	}
}

// succeed forgets earlier failures for ip after a correct authentication.
func (g *ipGuard) succeed(ip string) {
	g.mu.Lock()
	defer g.mu.Unlock()
	delete(g.failures, ip)
}

// pruneLocked drops expired records so the map cannot grow without bound under
// a scan from many addresses. It only scans once the map is large.
func (g *ipGuard) pruneLocked(now time.Time) {
	if len(g.failures) < 1024 {
		return
	}
	for ip, rec := range g.failures {
		if now.After(rec.blockedUntil) && now.Sub(rec.windowStart) > g.window {
			delete(g.failures, ip)
		}
	}
}

// remoteIP extracts the host part of a RemoteAddr ("ip:port").
func remoteIP(remoteAddr string) string {
	host, _, err := net.SplitHostPort(remoteAddr)
	if err != nil {
		return remoteAddr
	}
	return host
}
