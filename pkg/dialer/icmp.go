package dialer

import (
	"context"
	"net/http"
	"runtime"
	"time"

	"github.com/exelban/EndPoll/types"
	"github.com/go-ping/ping"
)

func (d *Dialer) icmpCall(ctx context.Context, h *types.Host) (response types.HttpResponse) {
	response.Timestamp = time.Now()
	response.Code = http.StatusBadRequest

	pinger, err := ping.NewPinger(h.URL)
	if err != nil {
		response.Body = err.Error()
		return response
	}
	pinger.Count = 1
	pinger.Timeout = timeout(h)
	// Linux: unprivileged (UDP) ping requires net.ipv4.ping_group_range to be set,
	// which is usually not the case inside a container. Raw sockets need CAP_NET_RAW
	// which docker grants by default. macOS/BSD allow unprivileged ping out of the box.
	pinger.SetPrivileged(runtime.GOOS == "linux")

	done := make(chan struct{})
	defer close(done)
	go func() {
		select {
		case <-ctx.Done():
			pinger.Stop()
		case <-done:
		}
	}()

	start := time.Now()
	if err = pinger.Run(); err != nil {
		response.Time = time.Since(start)
		response.Body = err.Error()
		return response
	}

	stats := pinger.Statistics()
	response.Time = stats.AvgRtt
	response.OK = stats.PacketsRecv > 0
	if response.OK {
		response.Code = http.StatusOK
	} else if ctx.Err() != nil {
		response.Body = ctx.Err().Error()
	} else {
		response.Code = 522
		response.Body = "no reply"
	}

	return response
}
