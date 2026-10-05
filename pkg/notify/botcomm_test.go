package notify

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
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
		require.Equal(t, "botcomm", b.string())

		subject, body := b.normalize(testHost(), types.DOWN)
		require.Equal(t, "❌ Google is DOWN", subject)
		require.Equal(t, "❌ Google (https://www.google.com) has a new status: DOWN", body)
		require.NoError(t, b.send("evt", subject, body))
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
			require.Equal(t, map[string]any{"content": body, "sessionID": b.sessionIDs[i]}, jsonBody(t, r.body))
		}
	})

	t.Run("broadcast accepted", func(t *testing.T) {
		srv := newCapture(http.StatusAccepted)
		defer srv.Close()

		b := &BotComm{url: srv.URL, clientID: "client", clientSecret: "secret"}
		require.NoError(t, b.send("evt", "EndPoll status", "I'm online"))
		require.Equal(t, 1, srv.count())
		require.Equal(t, map[string]any{"content": "I'm online", "broadcast": true}, jsonBody(t, srv.last().body))

		b.sessionIDs = []string{}
		require.NoError(t, b.send("evt", "EndPoll status", ""))
		require.Equal(t, map[string]any{"content": "EndPoll status", "broadcast": true}, jsonBody(t, srv.last().body))
	})

	t.Run("empty session IDs are rejected before sending", func(t *testing.T) {
		srv := newCapture(http.StatusOK)
		defer srv.Close()

		for _, id := range []string{"", " \t\n"} {
			b := &BotComm{url: srv.URL, sessionIDs: []string{"valid", id}}
			require.EqualError(t, b.send("evt", "subject", "body"), "botcomm: empty session ID")
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
		err := b.send("evt", "subject", "body")
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
		require.ErrorContains(t, b.send("evt", "subject", "body"), "429")
		require.Equal(t, 1, srv.count())
	})
}

func TestNew_BotComm(t *testing.T) {
	cfg := &types.Cfg{Notifications: types.Notifications{
		BotComm: &types.BotComm{ClientID: "client", ClientSecret: "secret", SessionIDs: []string{"session"}},
	}}
	n, err := New(cfg)
	require.NoError(t, err)
	require.Len(t, n.clients, 1)

	b := n.clients[0].(*BotComm)
	require.Equal(t, "https://api.botcomm.app/message", b.url)
	require.Equal(t, "client", b.clientID)
	require.Equal(t, "secret", b.clientSecret)
	require.Equal(t, []string{"session"}, b.sessionIDs)
	require.Equal(t, httpTimeout, b.client.Timeout)

	srv := newCapture(http.StatusOK)
	defer srv.Close()
	b.url = srv.URL

	host := testHost()
	host.Alerts = []string{"telegram"}
	require.NoError(t, n.Send(host, types.DOWN))
	require.Zero(t, srv.count())

	host.Alerts = []string{"botcomm"}
	require.NoError(t, n.Send(host, types.UP))
	require.Equal(t, "✅ Google (https://www.google.com) has a new status: UP", jsonBody(t, srv.last().body)["content"])

	n.Startup()
	require.Equal(t, "I'm online", jsonBody(t, srv.last().body)["content"])
}
