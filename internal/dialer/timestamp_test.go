package dialer

import (
	"context"
	"testing"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

// every response must carry a timestamp: it is the storage key, a zero value
// would be stored under a bogus key and returned as the "last" response forever.
func TestDialer_Dial_TimestampOnFailure(t *testing.T) {
	d := New(1)
	ctx := context.Background()

	t.Run("icmp invalid host", func(t *testing.T) {
		resp := d.Dial(ctx, &types.Host{URL: "invalid host name", Type: types.ICMPType})
		require.False(t, resp.OK)
		require.False(t, resp.Timestamp.IsZero())
	})
	t.Run("mongo invalid uri", func(t *testing.T) {
		resp := d.Dial(ctx, &types.Host{URL: "mongodb://", Type: types.MongoType})
		require.False(t, resp.OK)
		require.False(t, resp.Timestamp.IsZero())
	})
	t.Run("http invalid method", func(t *testing.T) {
		resp := d.Dial(ctx, &types.Host{URL: "http://127.0.0.1:1", Method: "bad method"})
		require.False(t, resp.OK)
		require.False(t, resp.Timestamp.IsZero())
	})
	t.Run("cancelled context", func(t *testing.T) {
		cctx, cancel := context.WithCancel(ctx)
		cancel()
		resp := d.Dial(cctx, &types.Host{URL: "http://127.0.0.1:1"})
		require.False(t, resp.OK)
		require.False(t, resp.Timestamp.IsZero())
	})
}
