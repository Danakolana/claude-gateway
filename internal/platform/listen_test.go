package platform

import (
	"net"
	"strconv"
	"strings"
	"testing"
)

func TestListenTCPFallback(t *testing.T) {
	hold, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Close()
	busy := hold.Addr().String()
	_, portStr, _ := net.SplitHostPort(busy)
	port, _ := strconv.Atoi(portStr)

	ln, bound, err := ListenTCP(busy, 5)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	_, gotStr, _ := net.SplitHostPort(bound)
	got, _ := strconv.Atoi(gotStr)
	if got == port {
		t.Fatalf("expected fallback off %d, bound %s", port, bound)
	}
	if got < port || got > port+5 {
		t.Fatalf("bound %s outside fallback window of %s", bound, busy)
	}
}

func TestListenTCPPortZeroNoFallbackNeeded(t *testing.T) {
	ln, bound, err := ListenTCP("127.0.0.1:0", 20)
	if err != nil {
		t.Fatal(err)
	}
	defer ln.Close()
	if bound == "" {
		t.Fatal("empty bound")
	}
}

func TestAddrInUse(t *testing.T) {
	if AddrInUse(nil) {
		t.Fatal("nil")
	}
	if !AddrInUse(errString("listen tcp 127.0.0.1:8080: bind: address already in use")) {
		t.Fatal("unix")
	}
	if !AddrInUse(errString("bind: Only one usage of each socket address")) {
		t.Fatal("windows")
	}
}

type errString string

func (e errString) Error() string { return string(e) }

func TestClaudeDesktopLikelyRunningDoesNotPanic(t *testing.T) {
	_ = ClaudeDesktopLikelyRunning()
}

func TestListenTCPExhausted(t *testing.T) {
	hold, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer hold.Close()
	_, _, err = ListenTCP(hold.Addr().String(), 0)
	if err == nil {
		t.Fatal("expected bind error with extraPorts=0")
	}
	if !strings.Contains(err.Error(), "address already in use") && !AddrInUse(err) {
		// OS wording varies; just require an error.
		t.Log(err)
	}
}
