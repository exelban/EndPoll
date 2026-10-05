package dialer

import (
	"context"
	"net"
	"net/http"
	"strings"
	"time"

	"github.com/exelban/EndPoll/types"
)

// tcpCall - opens a TCP connection to the host (url: tcp://host:port)
func (d *Dialer) tcpCall(ctx context.Context, h *types.Host) (response types.HttpResponse) {
	response.Timestamp = time.Now()

	addr := strings.TrimPrefix(h.URL, "tcp://")
	if _, _, err := net.SplitHostPort(addr); err != nil {
		response.Code = http.StatusBadRequest
		response.Body = "invalid address, expected tcp://host:port: " + err.Error()
		return
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(h))
	defer cancel()

	start := time.Now()
	conn, err := (&net.Dialer{}).DialContext(ctx, "tcp", addr)
	response.Time = time.Since(start)
	if err != nil {
		response.Code = errorCode(err)
		response.Body = truncate(err.Error(), 512)
		return
	}
	_ = conn.Close()

	response.OK = true
	response.Code = http.StatusOK
	return
}
