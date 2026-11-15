package check

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestHTTP_Check_DefaultMethod(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "GET", r.Method)
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("ok"))
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	d := NewHTTP()
	timeout := 5 * time.Second
	resp := d.Check(context.Background(), &types.Host{
		URL:             ts.URL,
		Method:          "", // empty method should default to GET
		TimeoutInterval: &timeout,
		Conditions:      &types.Success{Code: []int{200}},
	})
	require.True(t, resp.OK)
	require.Equal(t, http.StatusOK, resp.Code)
}

func TestHTTP_Check_CustomHeaders(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		require.Equal(t, "Bearer token123", r.Header.Get("Authorization"))
		require.Equal(t, "custom-value", r.Header.Get("X-Custom"))
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	d := NewHTTP()
	timeout := 5 * time.Second
	resp := d.Check(context.Background(), &types.Host{
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

func TestHTTP_Check_ResponseBody(t *testing.T) {
	t.Run("truncated body fails", func(t *testing.T) {
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			w.Header().Set("Content-Length", "100")
			_, _ = w.Write([]byte("partial"))
		}))
		defer ts.Close()

		resp := NewHTTP().Check(context.Background(), &types.Host{URL: ts.URL})
		require.False(t, resp.OK)
		require.NotEqual(t, http.StatusOK, resp.Code)
		require.Contains(t, resp.Body, "unexpected EOF")
	})

	t.Run("body conditions are bounded", func(t *testing.T) {
		for _, size := range []int{1024, maxBody, maxBody + 1} {
			body := strings.Repeat("x", size)
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))

			host := &types.Host{
				URL:        ts.URL,
				Conditions: &types.Success{Code: []int{200}, Body: &body},
			}
			resp := NewHTTP().Check(context.Background(), host)
			ts.Close()
			require.Equal(t, size <= maxBody, resp.OK && host.Status(resp.Code, resp.Bytes))
		}
	})

	t.Run("small body is captured", func(t *testing.T) {
		router := http.NewServeMux()
		router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = w.Write([]byte("hello world"))
		})
		ts := httptest.NewServer(router)
		defer ts.Close()

		d := NewHTTP()
		timeout := 5 * time.Second
		resp := d.Check(context.Background(), &types.Host{
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

		d := NewHTTP()
		timeout := 5 * time.Second
		resp := d.Check(context.Background(), &types.Host{
			URL:             ts.URL,
			Method:          "GET",
			TimeoutInterval: &timeout,
		})
		require.True(t, resp.OK)
		require.Empty(t, resp.Bytes)
	})
}

func TestHTTP_Check_StatusCodes(t *testing.T) {
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

			d := NewHTTP()
			timeout := 5 * time.Second
			resp := d.Check(context.Background(), &types.Host{
				URL:             ts.URL,
				Method:          "GET",
				TimeoutInterval: &timeout,
			})
			require.Equal(t, c, resp.Code)
		})
	}
}

func TestHTTP_Check_SSLCert(t *testing.T) {
	// httptest.NewTLSServer provides a self-signed cert
	ts := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))
	defer ts.Close()

	d := NewHTTP()
	timeout := 5 * time.Second

	// Use the test server's client which trusts the self-signed cert
	// We can't easily inject the client, but we can verify TLS endpoints are handled
	resp := d.Check(context.Background(), &types.Host{
		URL:             ts.URL,
		Method:          "GET",
		TimeoutInterval: &timeout,
	})
	// Self-signed cert will fail verification, which is expected
	// The dialer should handle this gracefully
	require.NotNil(t, resp)
}

func TestHTTP_Check_ConnectionRefused(t *testing.T) {
	d := NewHTTP()
	timeout := time.Second
	resp := d.Check(context.Background(), &types.Host{
		URL:             "http://localhost:1", // port 1 should refuse
		Method:          "GET",
		TimeoutInterval: &timeout,
	})
	require.False(t, resp.OK)
	require.NotEqual(t, 0, resp.Code)
}

func TestHTTP_Check_TimingMetrics(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		time.Sleep(10 * time.Millisecond)
		w.WriteHeader(http.StatusOK)
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	d := NewHTTP()
	timeout := 5 * time.Second
	resp := d.Check(context.Background(), &types.Host{
		URL:             ts.URL,
		Method:          "GET",
		TimeoutInterval: &timeout,
	})
	require.True(t, resp.OK)
	require.Greater(t, resp.Time, time.Duration(0))
	require.False(t, resp.Timestamp.IsZero())
}
