package api

import (
	"net/http"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestServer_RunAndShutdown(t *testing.T) {
	s := &Server{
		Port: 0, // will get default
	}

	// Use a high port to avoid conflicts
	s.Port = 19876

	router := http.NewServeMux()
	router.HandleFunc("GET /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	})

	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Run(router)
	}()

	// Give server time to start
	time.Sleep(100 * time.Millisecond)

	// Verify server is running
	resp, err := http.Get("http://localhost:19876/")
	require.NoError(t, err)
	require.Equal(t, http.StatusOK, resp.StatusCode)
	resp.Body.Close()

	// Shutdown
	require.NoError(t, s.Shutdown())

	// Wait for Run to return
	err = <-errCh
	require.NoError(t, err)
}

func TestServer_Defaults(t *testing.T) {
	s := &Server{}

	router := http.NewServeMux()
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Run(router)
	}()

	time.Sleep(100 * time.Millisecond)

	// Default port should be 8080
	require.Equal(t, 8080, s.Port)
	require.Equal(t, 10*time.Second, s.ReadHeaderTimeout)
	require.Equal(t, 30*time.Second, s.WriteTimeout)
	require.Equal(t, 60*time.Second, s.IdleTimeout)

	require.NoError(t, s.Shutdown())
	<-errCh
}

func TestServer_WildcardAddress(t *testing.T) {
	s := &Server{
		Address: "*",
		Port:    19877,
	}

	router := http.NewServeMux()
	errCh := make(chan error, 1)
	go func() {
		errCh <- s.Run(router)
	}()

	time.Sleep(100 * time.Millisecond)
	require.Equal(t, "", s.Address) // * should be converted to empty

	require.NoError(t, s.Shutdown())
	<-errCh
}
