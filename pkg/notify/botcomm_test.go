package notify

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestBotComm(t *testing.T) {
	t.Run("targeted messages", func(t *testing.T) {
		srv := newCapture(http.StatusOK)
		defer srv.Close()

		b := &BotComm{url: srv.URL + "/message", clientID: "client", clientSecret: "secret", sessionIDs: []string{"session-1", "session-2"}}
		require.Equal(t, "botcomm", b.String())

		subject, body := b.Normalize(testHost(), types.DOWN)
		require.Equal(t, "❌ Google is DOWN", subject)
		require.Equal(t, "❌ Google (https://www.google.com) has a new status: DOWN", body)
		require.NoError(t, b.Send(context.Background(), "evt", subject, body))
		require.Equal(t, 2, srv.count())

		srv.mu.Lock()
		defer srv.mu.Unlock()
		for i, r := range srv.reqs {
			require.Equal(t, http.MethodPost, r.method)
			require.Equal(t, "/message", r.path)
			require.Contains(t, r.headers.Get("Content-Type"), "application/json")
			req := &http.Request{Header: r.headers}
			id, secret, ok := req.BasicAuth()
			require.True(t, ok)
			require.Equal(t, "client", id)
			require.Equal(t, "secret", secret)
			require.Equal(t, map[string]any{"content": body, "subject": subject, "sessionID": b.sessionIDs[i]}, jsonBody(t, r.body))
		}
	})

	t.Run("broadcast accepted", func(t *testing.T) {
		srv := newCapture(http.StatusAccepted)
		defer srv.Close()

		b := &BotComm{url: srv.URL, clientID: "client", clientSecret: "secret"}
		require.NoError(t, b.Send(context.Background(), "evt", "EndPoll status", "I'm online"))
		require.Equal(t, 1, srv.count())
		require.Equal(t, map[string]any{"content": "I'm online", "subject": "EndPoll status", "broadcast": true}, jsonBody(t, srv.last().body))

		b.sessionIDs = []string{}
		require.NoError(t, b.Send(context.Background(), "evt", "EndPoll status", ""))
		require.Equal(t, map[string]any{"content": "EndPoll status", "subject": "EndPoll status", "broadcast": true}, jsonBody(t, srv.last().body))
	})

	t.Run("empty session IDs are rejected before sending", func(t *testing.T) {
		srv := newCapture(http.StatusOK)
		defer srv.Close()

		for _, id := range []string{"", " \t\n"} {
			b := &BotComm{url: srv.URL, sessionIDs: []string{"valid", id}}
			require.EqualError(t, b.Send(context.Background(), "evt", "subject", "body"), "botcomm: empty session ID")
		}
		require.Zero(t, srv.count())
	})

	t.Run("stops at the first failed recipient without retries", func(t *testing.T) {
		var mu sync.Mutex
		var sessions []string
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			var payload struct {
				SessionID string `json:"sessionID"`
			}
			if err := json.NewDecoder(r.Body).Decode(&payload); err != nil {
				http.Error(w, err.Error(), http.StatusBadRequest)
				return
			}
			mu.Lock()
			sessions = append(sessions, payload.SessionID)
			mu.Unlock()
			if payload.SessionID == "session-2" {
				http.Error(w, "unknown session", http.StatusNotFound)
				return
			}
			w.WriteHeader(http.StatusOK)
		}))
		defer srv.Close()

		b := &BotComm{url: srv.URL, sessionIDs: []string{"session-1", "session-2", "session-3"}}
		err := b.Send(context.Background(), "evt", "subject", "body")
		require.ErrorContains(t, err, "404")
		require.ErrorContains(t, err, "unknown session")
		mu.Lock()
		defer mu.Unlock()
		require.Equal(t, []string{"session-1", "session-2"}, sessions)
	})

	t.Run("broadcast limit is an error", func(t *testing.T) {
		srv := newCapture(http.StatusTooManyRequests)
		defer srv.Close()

		b := &BotComm{url: srv.URL}
		require.ErrorContains(t, b.Send(context.Background(), "evt", "subject", "body"), "429")
		require.Equal(t, 1, srv.count())
	})
}

func TestBotComm_Subject(t *testing.T) {
	tests := []struct {
		name    string
		subject string
		body    string
		want    string
	}{
		{name: "empty", body: "Status changed"},
		{name: "whitespace", subject: " \t\n ", body: "Status changed"},
		{name: "trimmed", subject: "  ❌ API is DOWN  ", body: "Status changed", want: "❌ API is DOWN"},
		{name: "control characters", subject: "API\r\nstatus\tDOWN\x00\u0085", body: "Status changed", want: "API  status DOWN"},
		{name: "line separators", subject: "API\u2028status\u2029DOWN", body: "Status changed", want: "API status DOWN"},
		{name: "unicode limit", subject: strings.Repeat("🔥", 256), body: "Status changed", want: strings.Repeat("🔥", 256)},
		{name: "long unicode subject", subject: strings.Repeat("🔥", 257), body: "Status changed", want: strings.Repeat("🔥", 255) + "…"},
		{name: "fallback preserves original subject", subject: "API\n" + strings.Repeat("🔥", 256), want: "API " + strings.Repeat("🔥", 251) + "…"},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			srv := newCapture(http.StatusOK)
			defer srv.Close()

			b := &BotComm{url: srv.URL, sessionIDs: []string{"session"}}
			require.NoError(t, b.Send(context.Background(), "evt", tc.subject, tc.body))

			payload := jsonBody(t, srv.last().body)
			if tc.want == "" {
				require.NotContains(t, payload, "subject")
			} else {
				require.Equal(t, tc.want, payload["subject"])
			}
			body := tc.body
			if body == "" {
				body = tc.subject
			}
			require.Equal(t, body, payload["content"])
		})
	}
}

func TestNewBotComm(t *testing.T) {
	b := NewBotComm(&types.BotComm{ClientID: "client", ClientSecret: "secret", SessionIDs: []string{"session"}})
	require.Equal(t, "https://api.botcomm.app/message", b.url)
	require.Equal(t, "client", b.clientID)
	require.Equal(t, "secret", b.clientSecret)
	require.Equal(t, []string{"session"}, b.sessionIDs)
	require.Equal(t, httpTimeout, b.client.Timeout)
}
