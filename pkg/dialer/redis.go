package dialer

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/exelban/EndPoll/types"
)

// redisCall - sends PING to a redis server (url: redis://[user:password@]host:port[/db], rediss:// for TLS).
// The RESP protocol is simple enough to speak directly, no client library is needed.
func (d *Dialer) redisCall(ctx context.Context, h *types.Host) (response types.HttpResponse) {
	response.Timestamp = time.Now()

	u, err := url.Parse(h.URL)
	if err != nil || u.Host == "" {
		response.Code = http.StatusBadRequest
		response.Body = "invalid address, expected redis://host:port"
		return
	}
	addr := u.Host
	if _, _, err := net.SplitHostPort(addr); err != nil {
		addr = net.JoinHostPort(addr, "6379")
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(h))
	defer cancel()

	start := time.Now()
	defer func() {
		response.Time = time.Since(start)
	}()

	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	if err != nil {
		response.Code = errorCode(err)
		response.Body = truncate(err.Error(), 512)
		return
	}
	defer func() {
		_ = conn.Close()
	}()
	if u.Scheme == "rediss" {
		host, _, _ := net.SplitHostPort(addr)
		tlsConn := tls.Client(conn, &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12})
		if err := tlsConn.HandshakeContext(ctx); err != nil {
			response.Code = 525
			response.Body = truncate(err.Error(), 512)
			return
		}
		conn = tlsConn
	}
	if deadline, ok := ctx.Deadline(); ok {
		_ = conn.SetDeadline(deadline)
	}
	rw := bufio.NewReadWriter(bufio.NewReader(conn), bufio.NewWriter(conn))

	// AUTH (optional), SELECT (optional), PING
	if u.User != nil {
		if password, ok := u.User.Password(); ok {
			args := []string{"AUTH", password}
			if u.User.Username() != "" {
				args = []string{"AUTH", u.User.Username(), password}
			}
			if err := redisCommand(rw, args...); err != nil {
				response.Code = http.StatusUnauthorized
				response.Body = truncate(err.Error(), 512)
				return
			}
		}
	}
	if db := strings.Trim(u.Path, "/"); db != "" {
		if err := redisCommand(rw, "SELECT", db); err != nil {
			response.Code = http.StatusBadGateway
			response.Body = truncate(err.Error(), 512)
			return
		}
	}
	if err := redisCommand(rw, "PING"); err != nil {
		response.Code = http.StatusBadGateway
		response.Body = truncate(err.Error(), 512)
		return
	}

	response.OK = true
	response.Code = http.StatusOK
	return
}

// redisCommand - sends the command in the RESP format and fails on an error reply
func redisCommand(rw *bufio.ReadWriter, args ...string) error {
	var b strings.Builder
	fmt.Fprintf(&b, "*%d\r\n", len(args))
	for _, a := range args {
		fmt.Fprintf(&b, "$%d\r\n%s\r\n", len(a), a)
	}
	if _, err := rw.WriteString(b.String()); err != nil {
		return err
	}
	if err := rw.Flush(); err != nil {
		return err
	}

	line, err := rw.ReadString('\n')
	if err != nil {
		return fmt.Errorf("read reply: %w", err)
	}
	line = strings.TrimRight(line, "\r\n")
	if line == "" {
		return fmt.Errorf("empty reply")
	}
	if line[0] == '-' {
		return fmt.Errorf("%s: %s", args[0], strings.TrimPrefix(line, "-"))
	}
	return nil
}
