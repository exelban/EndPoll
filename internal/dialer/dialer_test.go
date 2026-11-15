package dialer

import (
	"context"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestNewDialer(t *testing.T) {
	dialer := New(3)
	require.Equal(t, 3, cap(dialer.sem))
}

func TestDialer_Dial(t *testing.T) {
	dialer := New(3)

	ts, _, shutdown := srv(time.Millisecond * 10)
	defer shutdown()
	ctx := context.Background()

	t.Run("wrong method", func(t *testing.T) {
		resp := dialer.Dial(ctx, &types.Host{
			Method: "?",
		})
		require.False(t, resp.OK)
		require.Equal(t, 0, resp.Code)
		require.Empty(t, resp.Bytes)
	})

	t.Run("wrong url", func(t *testing.T) {
		resp := dialer.Dial(ctx, &types.Host{})
		require.False(t, resp.OK)
		require.Equal(t, 503, resp.Code)
		require.Empty(t, resp.Bytes)
	})

	t.Run("semaphore check", func(t *testing.T) {
		wg := sync.WaitGroup{}
		wg.Add(9)
		start := time.Now()

		for i := 0; i < 9; i++ {
			go func() {
				resp := dialer.Dial(ctx, &types.Host{
					Method: "GET",
					URL:    ts.URL,
				})
				require.True(t, resp.OK)
				require.Equal(t, http.StatusOK, resp.Code)
				wg.Done()
			}()
		}

		wg.Wait()
		require.Less(t, time.Now().Sub(start).Milliseconds(), int64(50))
		require.Greater(t, time.Now().Sub(start).Milliseconds(), int64(30))
	})

	t.Run("check timeout", func(t *testing.T) {
		timeout := time.Millisecond * 5
		resp := dialer.Dial(ctx, &types.Host{
			Method:          "GET",
			URL:             ts.URL,
			TimeoutInterval: &timeout,
		})
		require.False(t, resp.OK)
		require.Equal(t, 522, resp.Code)
		require.Empty(t, resp.Bytes)
	})
}

func srv(timeout time.Duration) (*httptest.Server, *atomic.Value, func()) {
	router := http.NewServeMux()
	status := atomic.Value{}
	status.Store(true)

	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(timeout)
		if status.Load() == true {
			w.WriteHeader(http.StatusOK)
		} else {
			http.Error(w, "error", http.StatusInternalServerError)
		}
	})

	ts := httptest.NewServer(router)
	shutdown := func() {
		ts.Close()
	}

	return ts, &status, shutdown
}

func TestDialer_TypeRouting(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	d := New(1)
	defer d.Close()
	timeout := 5 * time.Second

	t.Run("http type", func(t *testing.T) {
		resp := d.Dial(context.Background(), &types.Host{
			Type:            types.HttpType,
			URL:             ts.URL,
			Method:          "GET",
			TimeoutInterval: &timeout,
		})
		require.Equal(t, http.StatusOK, resp.Code)
	})

	cases := []struct {
		name string
		host types.Host
		code int
	}{
		{name: "tcp", host: types.Host{Type: types.TCPType, URL: "tcp://localhost"}, code: http.StatusBadRequest},
		{name: "dns", host: types.Host{Type: types.DNSType, URL: "dns://"}, code: http.StatusBadRequest},
		{name: "redis", host: types.Host{Type: types.RedisType, URL: "redis://"}, code: http.StatusBadRequest},
		{name: "postgres", host: types.Host{Type: types.PostgresType, URL: "postgres://"}, code: http.StatusBadRequest},
		{name: "mysql", host: types.Host{Type: types.MySQLType, URL: "mysql://"}, code: http.StatusBadRequest},
		{name: "mongo", host: types.Host{Type: types.MongoType, URL: "mongodb://"}, code: 523},
		{name: "icmp", host: types.Host{Type: types.ICMPType, URL: "invalid host name"}, code: http.StatusBadRequest},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			resp := d.Dial(context.Background(), &tc.host)
			require.False(t, resp.OK)
			require.Equal(t, tc.code, resp.Code)
			require.NotEmpty(t, resp.Body)
			require.False(t, resp.Timestamp.IsZero())
		})
	}
}
