package platform

import (
	"fmt"
	"net"
	"strconv"
	"strings"
)

// ListenTCP binds addr. If the port is busy, it tries the next extraPorts
// ports on the same host. Port 0 (OS-assigned) never falls back.
func ListenTCP(addr string, extraPorts int) (net.Listener, string, error) {
	ln, err := net.Listen("tcp", addr)
	if err == nil {
		return ln, ln.Addr().String(), nil
	}
	host, portStr, splitErr := net.SplitHostPort(addr)
	if splitErr != nil {
		return nil, "", err
	}
	if portStr == "0" || extraPorts <= 0 {
		return nil, "", err
	}
	port, convErr := strconv.Atoi(portStr)
	if convErr != nil {
		return nil, "", err
	}
	first := err
	for i := 1; i <= extraPorts; i++ {
		next := net.JoinHostPort(host, strconv.Itoa(port+i))
		ln, err = net.Listen("tcp", next)
		if err == nil {
			return ln, ln.Addr().String(), nil
		}
	}
	return nil, "", fmt.Errorf("%w (also tried +%d ports)", first, extraPorts)
}

// AddrInUse reports whether err looks like a bind conflict (best-effort, OS-agnostic).
func AddrInUse(err error) bool {
	if err == nil {
		return false
	}
	s := strings.ToLower(err.Error())
	return strings.Contains(s, "address already in use") ||
		strings.Contains(s, "only one usage of each socket address") ||
		strings.Contains(s, "already in use")
}
