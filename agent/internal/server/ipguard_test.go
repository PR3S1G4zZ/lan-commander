package server

import (
	"testing"
	"time"
)

func newTestGuard(now *time.Time) *ipGuard {
	g := newIPGuard()
	g.maxActive = 2
	g.maxFailures = 3
	g.now = func() time.Time { return *now }
	return g
}

func TestIPGuardLimitsSimultaneousConnectionsPerAddress(t *testing.T) {
	now := time.Now()
	g := newTestGuard(&now)

	for i := 0; i < 2; i++ {
		if ok, _ := g.admit("10.0.0.1"); !ok {
			t.Fatalf("connection %d from a fresh address was refused", i+1)
		}
	}
	if ok, _ := g.admit("10.0.0.1"); ok {
		t.Fatal("third simultaneous connection from the same address was admitted")
	}
	if ok, _ := g.admit("10.0.0.2"); !ok {
		t.Fatal("another address must not be affected by the first one's usage")
	}

	g.release("10.0.0.1")
	if ok, _ := g.admit("10.0.0.1"); !ok {
		t.Fatal("a released slot should be reusable")
	}
}

func TestIPGuardBlocksAfterRepeatedFailuresAndRecovers(t *testing.T) {
	now := time.Now()
	g := newTestGuard(&now)

	g.fail("10.0.0.9")
	g.fail("10.0.0.9")
	if ok, _ := g.admit("10.0.0.9"); !ok {
		t.Fatal("address below the failure limit must still be admitted")
	}
	g.release("10.0.0.9")

	g.fail("10.0.0.9")
	if ok, reason := g.admit("10.0.0.9"); ok || reason == "" {
		t.Fatalf("address over the failure limit was admitted (reason %q)", reason)
	}

	now = now.Add(AuthBlockDuration + time.Second)
	if ok, _ := g.admit("10.0.0.9"); !ok {
		t.Fatal("the block must expire")
	}
}

func TestIPGuardSuccessAndWindowExpiryResetFailures(t *testing.T) {
	now := time.Now()
	g := newTestGuard(&now)

	g.fail("10.0.0.7")
	g.fail("10.0.0.7")
	g.succeed("10.0.0.7")
	g.fail("10.0.0.7")
	if ok, _ := g.admit("10.0.0.7"); !ok {
		t.Fatal("a successful login must clear earlier failures")
	}
	g.release("10.0.0.7")

	now = now.Add(AuthFailureWindow + time.Second)
	g.fail("10.0.0.7")
	g.fail("10.0.0.7")
	if ok, _ := g.admit("10.0.0.7"); !ok {
		t.Fatal("failures older than the window must not count")
	}
}

func TestRemoteIPStripsPort(t *testing.T) {
	for in, want := range map[string]string{
		"192.168.1.5:5555": "192.168.1.5",
		"[fe80::1]:9474":   "fe80::1",
		"nonsense":         "nonsense",
	} {
		if got := remoteIP(in); got != want {
			t.Errorf("remoteIP(%q) = %q, want %q", in, got, want)
		}
	}
}
