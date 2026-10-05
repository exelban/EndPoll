package dialer

import (
	"context"
	"net"
	"net/http"
	"time"

	"github.com/exelban/EndPoll/types"
)

// Dialer - the request maker structure
type Dialer struct {
	sem       chan struct{}
	transport *http.Transport
}

// New - creates a new dialer with maxConn semaphore
func New(maxConn int) *Dialer {
	if maxConn <= 0 {
		maxConn = types.DefaultMaxConn
	}
	return &Dialer{
		sem: make(chan struct{}, maxConn),
		transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:   true,
			DisableKeepAlives:   true,
			TLSHandshakeTimeout: 30 * time.Second,
		},
	}
}

// Close - releases the resources held by the dialer.
func (d *Dialer) Close() {
	if d.transport != nil {
		d.transport.CloseIdleConnections()
	}
}

// Dial - make a request to the provided host based on its type
func (d *Dialer) Dial(ctx context.Context, h *types.Host) types.HttpResponse {
	start := time.Now()

	select {
	case d.sem <- struct{}{}:
	case <-ctx.Done():
		return types.HttpResponse{
			Timestamp: start,
			Code:      http.StatusServiceUnavailable,
			Body:      ctx.Err().Error(),
		}
	}
	defer func() {
		<-d.sem
	}()

	var resp types.HttpResponse
	switch h.Type {
	case types.MongoType:
		resp = d.mongoCall(ctx, h)
	case types.ICMPType:
		resp = d.icmpCall(ctx, h)
	case types.TCPType:
		resp = d.tcpCall(ctx, h)
	case types.DNSType:
		resp = d.dnsCall(ctx, h)
	case types.RedisType:
		resp = d.redisCall(ctx, h)
	case types.PostgresType, types.MySQLType:
		resp = d.sqlCall(ctx, h)
	default:
		resp = d.httpCall(ctx, h)
	}

	if resp.Timestamp.IsZero() {
		resp.Timestamp = start
	}

	return resp
}

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
