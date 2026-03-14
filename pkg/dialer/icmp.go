package dialer

import (
	"context"
	"net/http"
	"time"

	"github.com/exelban/EndPoll/types"
	"github.com/go-ping/ping"
)

func (d *Dialer) icmpCall(ctx context.Context, h *types.Host) (response types.HttpResponse) {
	pinger, err := ping.NewPinger(h.URL)
	if err != nil {
		response.OK = false
		response.Body = err.Error()
		return response
	}
	pinger.Count = 1
	if h.TimeoutInterval != nil {
		pinger.Timeout = *h.TimeoutInterval
	}

	if err = pinger.Run(); err != nil {
		response.OK = false
		response.Body = err.Error()
		return response
	}

	stats := pinger.Statistics()
	response.Timestamp = time.Now()
	response.Time = stats.AvgRtt
	response.OK = stats.PacketsRecv > 0
	if response.OK {
		response.Code = http.StatusOK
	} else {
		response.Code = http.StatusBadRequest
	}

	return response
}
