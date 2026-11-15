package check

import (
	"context"
	"errors"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/exelban/EndPoll/types"
)

func timeout(h *types.Host) time.Duration {
	if h.TimeoutInterval != nil && *h.TimeoutInterval > 0 {
		return *h.TimeoutInterval
	}
	return types.DefaultTimeout
}
func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n] + "..."
}

// errorCode - maps a transport error to the pseudo status codes used by the UI:
// 522 - timeout, 523 - origin unreachable (dial failed), 521 - connection dropped.
func errorCode(err error) int {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return StatusConnectionTimeout
	}

	if opErr, ok := errors.AsType[*net.OpError](err); ok {
		switch opErr.Op {
		case "dial":
			return StatusOriginUnreachable
		case "read":
			return StatusWebServerDown
		}
	}

	msg := err.Error()
	if strings.Contains(msg, "refused") || strings.Contains(msg, "unreachable") || strings.Contains(msg, "no such host") {
		return StatusOriginUnreachable
	}
	return http.StatusServiceUnavailable
}
