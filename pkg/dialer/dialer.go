package dialer

import (
	"context"

	"github.com/exelban/EndPoll/types"
)

// Dialer - the request maker structure
type Dialer struct {
	sem chan int
}

// New - creates a new dialer with maxConn semaphore
func New(maxConn int) *Dialer {
	return &Dialer{
		sem: make(chan int, maxConn),
	}
}

// Dial - make http request to the provided host
func (d *Dialer) Dial(ctx context.Context, h *types.Host) types.HttpResponse {
	d.sem <- 1
	defer func() {
		<-d.sem
	}()

	resp := make(chan types.HttpResponse, 1)
	go func() {
		switch h.Type {
		case types.MongoType:
			resp <- d.mongoCall(ctx, h)
		case types.ICMPType:
			resp <- d.icmpCall(ctx, h)
		default:
			resp <- d.httpCall(ctx, h)
		}
	}()

	return <-resp
}
