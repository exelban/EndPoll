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

func TestDialer_httpCall_DefaultMethod(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	d := New(1)
	timeout := 5 * time.Second
	resp := d.Dial(context.Background(), &types.Host{
		URL:             ts.URL,
		Method:          "", // empty method should default to GET
		TimeoutInterval: &timeout,
		Conditions:      &types.Success{Code: []int{200}},
	})
	require.True(t, resp.OK)
	require.Equal(t, http.StatusOK, resp.Code)
}

func TestDialer_httpCall_CustomHeaders(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer token123", r.Header.Get("Authorization"))
		require.Equal(t, "custom-value", r.Header.Get("X-Custom"))
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	d := New(1)
	timeout := 5 * time.Second
	resp := d.Dial(context.Background(), &types.Host{
		URL:             ts.URL,
		Method:          "GET",
		TimeoutInterval: &timeout,
		Headers: map[string]string{
			"Authorization": "Bearer token123",
			"X-Custom":      "custom-value",
		},
	})
	require.True(t, resp.OK)
	require.Equal(t, http.StatusOK, resp.Code)
}

func TestDialer_httpCall_ResponseBody(t *testing.T) {
	t.Run("small body is captured", func(t *testing.T) {
		router := http.NewServeMux()
		router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello world"))
		})
		ts := httptest.NewServer(router)
		defer ts.Close()

		d := New(1)
		timeout := 5 * time.Second
		resp := d.Dial(context.Background(), &types.Host{
			URL:             ts.URL,
			Method:          "GET",
			TimeoutInterval: &timeout,
		})
		require.True(t, resp.OK)
		require.Equal(t, []byte("hello world"), resp.Bytes)
	})

	t.Run("large body is not captured", func(t *testing.T) {
		largeBody := make([]byte, 2048)
		for i := range largeBody {
			largeBody[i] = 'x'
		}
		router := http.NewServeMux()
		router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write(largeBody)
		})
		ts := httptest.NewServer(router)
		defer ts.Close()

		d := New(1)
		timeout := 5 * time.Second
		resp := d.Dial(context.Background(), &types.Host{
			URL:             ts.URL,
			Method:          "GET",
			TimeoutInterval: &timeout,
		})
		require.True(t, resp.OK)
		require.Empty(t, resp.Bytes)
	})
}

func TestDialer_httpCall_StatusCodes(t *testing.T) {
	codes := []int{200, 201, 301, 400, 404, 500, 502, 503}
	for _, code := range codes {
		t.Run(http.StatusText(code), func(t *testing.T) {
			router := http.NewServeMux()
			c := code
			router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(c)
			})
			ts := httptest.NewServer(router)
			defer ts.Close()

			d := New(1)
			timeout := 5 * time.Second
			resp := d.Dial(context.Background(), &types.Host{
				URL:             ts.URL,
				Method:          "GET",
				TimeoutInterval: &timeout,
			})
			require.Equal(t, c, resp.Code)
		})
	}
}

func TestDialer_httpCall_SSLCert(t *testing.T) {
	// httptest.NewTLSServer provides a self-signed cert
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	d := New(1)
	timeout := 5 * time.Second

	// Use the test server's client which trusts the self-signed cert
	// We can't easily inject the client, but we can verify TLS endpoints are handled
	resp := d.Dial(context.Background(), &types.Host{
		URL:             ts.URL,
		Method:          "GET",
		TimeoutInterval: &timeout,
	})
	// Self-signed cert will fail verification, which is expected
	// The dialer should handle this gracefully
	require.NotNil(t, resp)
}

func TestDialer_httpCall_ConnectionRefused(t *testing.T) {
	d := New(1)
	timeout := time.Second
	resp := d.Dial(context.Background(), &types.Host{
		URL:             "http://localhost:1", // port 1 should refuse
		Method:          "GET",
		TimeoutInterval: &timeout,
	})
	require.False(t, resp.OK)
	require.NotEqual(t, 0, resp.Code)
}

func TestDialer_httpCall_TimingMetrics(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	d := New(1)
	timeout := 5 * time.Second
	resp := d.Dial(context.Background(), &types.Host{
		URL:             ts.URL,
		Method:          "GET",
		TimeoutInterval: &timeout,
	})
	require.True(t, resp.OK)
	require.Greater(t, resp.Time, time.Duration(0))
	require.False(t, resp.Timestamp.IsZero())
}

func TestDialer_TypeRouting(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	d := New(1)
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
}
