package dialer

import (
	"context"
	"net/http"
	"time"

	"github.com/exelban/EndPoll/pkg/check"
	"github.com/exelban/EndPoll/types"
)

// Dialer - the request maker structure
type Dialer struct {
	sem  chan struct{}
	http *check.HTTP
}

// New - creates a new dialer with maxConn semaphore
func New(maxConn int) *Dialer {
	if maxConn <= 0 {
		maxConn = types.DefaultMaxConn
	}
	return &Dialer{
		sem:  make(chan struct{}, maxConn),
		http: check.NewHTTP(),
	}
}

// Close - releases the resources held by the dialer.
func (d *Dialer) Close() {
	if d.http != nil {
		d.http.Close()
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
		resp = check.Mongo(ctx, h)
	case types.ICMPType:
		resp = check.ICMP(ctx, h)
	case types.TCPType:
		resp = check.TCP(ctx, h)
	case types.DNSType:
		resp = check.DNS(ctx, h)
	case types.RedisType:
		resp = check.Redis(ctx, h)
	case types.PostgresType, types.MySQLType:
		resp = check.SQL(ctx, h)
	default:
		resp = d.http.Check(ctx, h)
	}

	if resp.Timestamp.IsZero() {
		resp.Timestamp = start
	}

	return resp
}
