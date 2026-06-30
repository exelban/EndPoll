package connectivity

import (
	"context"
	"log"
	"net"
	"sync"
	"time"

	"github.com/exelban/EndPoll/types"
)

// Checker - verifies that the monitor itself has internet connectivity.
// It is used to avoid false-positive DOWN statuses when the host running
// EndPoll loses its own connection (e.g. home internet outage).
type Checker struct {
	enabled bool
	targets []string
	timeout time.Duration
	ttl     time.Duration

	mu        sync.Mutex
	online    bool
	lastCheck time.Time
}

// New - creates a connectivity checker from the configuration.
func New(cfg *types.Connectivity) *Checker {
	c := &Checker{
		enabled: true,
		targets: []string{"1.1.1.1:53", "8.8.8.8:53"},
		timeout: 2 * time.Second,
		ttl:     5 * time.Second,
		online:  true,
	}

	if cfg != nil {
		if cfg.Disabled {
			c.enabled = false
		}
		if len(cfg.Targets) > 0 {
			c.targets = cfg.Targets
		}
		if cfg.Timeout > 0 {
			c.timeout = cfg.Timeout
		}
		if cfg.Interval > 0 {
			c.ttl = cfg.Interval
		}
	}

	return c
}

// Online - reports whether the monitor currently has connectivity.
// The result is cached for the configured interval to avoid probing on
// every failed host check. When disabled it always returns true.
func (c *Checker) Online() bool {
	if !c.enabled {
		return true
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	if !c.lastCheck.IsZero() && time.Since(c.lastCheck) < c.ttl {
		return c.online
	}

	online := c.probe()
	if online != c.online {
		if online {
			log.Print("[INFO] monitor connectivity restored")
		} else {
			log.Print("[WARN] monitor lost connectivity, suppressing host status changes")
		}
	}
	c.online = online
	c.lastCheck = time.Now()

	return c.online
}

// probe - tries to reach any of the configured targets over TCP.
// A single successful connection means the monitor is online.
func (c *Checker) probe() bool {
	ctx, cancel := context.WithTimeout(context.Background(), c.timeout)
	defer cancel()

	result := make(chan bool, len(c.targets))
	var wg sync.WaitGroup
	for _, target := range c.targets {
		wg.Add(1)
		go func(addr string) {
			defer wg.Done()
			var d net.Dialer
			conn, err := d.DialContext(ctx, "tcp", addr)
			if err == nil {
				_ = conn.Close()
				result <- true
			}
		}(target)
	}
	go func() {
		wg.Wait()
		close(result)
	}()

	for range result {
		return true
	}
	return false
}
