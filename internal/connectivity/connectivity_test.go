package connectivity

import (
	"net"
	"testing"
	"time"

	"github.com/exelban/EndPoll/types"
)

func TestChecker_Disabled(t *testing.T) {
	c := New(&types.Connectivity{Disabled: true, Targets: []string{"127.0.0.1:1"}})
	if !c.Online() {
		t.Fatal("disabled checker must always report online")
	}
}

func TestChecker_Online(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	defer ln.Close()
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			_ = conn.Close()
		}
	}()

	c := New(&types.Connectivity{Targets: []string{ln.Addr().String()}, Timeout: time.Second})
	if !c.Online() {
		t.Fatal("expected online with a reachable target")
	}
}

func TestChecker_Offline(t *testing.T) {
	// 192.0.2.1 is reserved for documentation (TEST-NET-1) and is not routable.
	c := New(&types.Connectivity{Targets: []string{"192.0.2.1:53"}, Timeout: 200 * time.Millisecond})
	if c.Online() {
		t.Fatal("expected offline with an unreachable target")
	}
}

func TestChecker_Cache(t *testing.T) {
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen: %v", err)
	}
	addr := ln.Addr().String()

	c := New(&types.Connectivity{Targets: []string{addr}, Timeout: time.Second, Interval: time.Minute})
	go func() {
		conn, err := ln.Accept()
		if err == nil {
			_ = conn.Close()
		}
	}()
	if !c.Online() {
		t.Fatal("expected online")
	}

	// closing the listener should not change the cached result within the interval
	_ = ln.Close()
	if !c.Online() {
		t.Fatal("expected cached online result")
	}
}
