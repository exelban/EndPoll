package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestSlack_send(t *testing.T) {
	router := http.NewServeMux()

	router.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		req := struct {
			Text string `json:"text,omitempty"`
		}{}
		_ = json.Unmarshal(b, &req)

		if req.Text == "timeout" {
			time.Sleep(time.Millisecond * 20)
		} else if req.Text == "error" {
			http.Error(w, "error", http.StatusInternalServerError)
			return
		}

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("{\"ok\": true}"))
	})
	ts := httptest.NewServer(router)
	defer func() {
		ts.Close()
	}()

	slack := &Slack{
		url:     ts.URL,
		token:   "test",
		channel: "test",
		timeout: time.Millisecond * 10,
	}

	require.NoError(t, slack.send("", "test"))
	require.Error(t, slack.send("", "error"))
	require.Error(t, slack.send("", "timeout"))
}

func TestSlack_normalize(t *testing.T) {
	s := &Slack{}

	t.Run("host with name uses name", func(t *testing.T) {
		name := "My API"
		host := &types.Host{URL: "http://example.com", Name: &name}
		subject, body := s.normalize(host, types.UP)
		require.Contains(t, subject, "My API")
		require.Contains(t, body, "UP")
		require.Contains(t, subject, "✅")
	})

	t.Run("host without name uses URL", func(t *testing.T) {
		host := &types.Host{URL: "http://example.com"}
		subject, _ := s.normalize(host, types.DOWN)
		require.Contains(t, subject, "http://example.com")
		require.Contains(t, subject, "❌")
		require.Contains(t, subject, "DOWN")
	})

	t.Run("host with empty name uses URL", func(t *testing.T) {
		empty := ""
		host := &types.Host{URL: "http://example.com", Name: &empty}
		subject, _ := s.normalize(host, types.UP)
		require.Contains(t, subject, "http://example.com")
	})
}

func TestSlack_string(t *testing.T) {
	s := &Slack{}
	require.Equal(t, "slack", s.string())
}

func TestSlack_send_ResponseBodyClosed(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		req := struct {
			Channel string `json:"channel,omitempty"`
			Text    string `json:"text,omitempty"`
		}{}
		_ = json.Unmarshal(b, &req)

		require.Equal(t, "test-channel", req.Channel)

		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":true}`))
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	s := &Slack{
		url:     ts.URL,
		token:   "test-token",
		channel: "test-channel",
		timeout: time.Second,
	}
	require.NoError(t, s.send("subject", "body"))
}

func TestSlack_send_NonOkResponse(t *testing.T) {
	router := http.NewServeMux()
	router.HandleFunc("POST /", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok":false,"error":"invalid_auth"}`))
	})
	ts := httptest.NewServer(router)
	defer ts.Close()

	s := &Slack{
		url:     ts.URL,
		token:   "bad-token",
		channel: "test",
		timeout: time.Second,
	}
	err := s.send("subject", "body")
	require.Error(t, err)
	require.Contains(t, err.Error(), "non-ok")
}
