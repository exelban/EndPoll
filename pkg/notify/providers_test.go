package notify

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"sync"
	"testing"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

// capture - test server that records the requests it receives
type capture struct {
	*httptest.Server

	mu   sync.Mutex
	reqs []captured
	code int
}

type captured struct {
	method  string
	path    string
	headers http.Header
	body    []byte
}

func newCapture(code int) *capture {
	c := &capture{code: code}
	c.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		c.mu.Lock()
		c.reqs = append(c.reqs, captured{method: r.Method, path: r.URL.Path, headers: r.Header.Clone(), body: b})
		c.mu.Unlock()
		w.WriteHeader(c.code)
		_, _ = w.Write([]byte("response body"))
	}))
	return c
}

func (c *capture) last() captured {
	c.mu.Lock()
	defer c.mu.Unlock()
	require.NotEmpty(nil, c.reqs)
	return c.reqs[len(c.reqs)-1]
}

func (c *capture) count() int {
	c.mu.Lock()
	defer c.mu.Unlock()
	return len(c.reqs)
}

func jsonBody(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	require.NoError(t, json.Unmarshal(b, &m))
	return m
}

func testHost() *types.Host {
	name := "Google"
	group := "Search"
	return &types.Host{ID: "j_3vve", URL: "https://www.google.com", Name: &name, Group: &group}
}

func TestWebhook(t *testing.T) {
	srv := newCapture(http.StatusOK)
	defer srv.Close()

	w := &Webhook{url: srv.URL, method: http.MethodPut, headers: map[string]string{"Authorization": "Bearer x"}}
	require.Equal(t, "webhook", w.string())

	t.Run("host status", func(t *testing.T) {
		subject, body := w.normalize(testHost(), types.DOWN)
		require.NoError(t, w.send("evt.1", subject, body))

		r := srv.last()
		require.Equal(t, http.MethodPut, r.method)
		require.Equal(t, "Bearer x", r.headers.Get("Authorization"))
		require.Contains(t, r.headers.Get("Content-Type"), "application/json")

		m := jsonBody(t, r.body)
		require.Equal(t, "evt.1", m["id"])
		require.Equal(t, "host", m["type"])
		require.Equal(t, "down", m["status"])
		require.Equal(t, "❌ Google is DOWN", m["subject"])
		require.NotEmpty(t, m["timestamp"])
		h := m["host"].(map[string]any)
		require.Equal(t, "j_3vve", h["id"])
		require.Equal(t, "Google", h["name"])
		require.Equal(t, "https://www.google.com", h["url"])
		require.Equal(t, "Search", h["group"])
	})

	t.Run("system message", func(t *testing.T) {
		require.NoError(t, w.send("evt.2", "EndPoll status", "I'm online"))
		m := jsonBody(t, srv.last().body)
		require.Equal(t, "system", m["type"])
		require.Equal(t, "I'm online", m["message"])
		require.Equal(t, "evt.2", m["id"])
	})

	t.Run("non-2xx is an error", func(t *testing.T) {
		bad := newCapture(http.StatusInternalServerError)
		defer bad.Close()
		err := (&Webhook{url: bad.URL}).send("evt", "s", "b")
		require.Error(t, err)
		require.Contains(t, err.Error(), "500")
		require.Contains(t, err.Error(), "response body")
	})

	t.Run("default method is POST", func(t *testing.T) {
		require.NoError(t, (&Webhook{url: srv.URL}).send("evt", "s", "b"))
		require.Equal(t, http.MethodPost, srv.last().method)
	})
}

func TestDiscord(t *testing.T) {
	srv := newCapture(http.StatusNoContent)
	defer srv.Close()

	d := &Discord{url: srv.URL, username: "EndPoll"}
	require.Equal(t, "discord", d.string())

	subject, body := d.normalize(testHost(), types.UP)
	require.NoError(t, d.send("evt", subject, body))

	m := jsonBody(t, srv.last().body)
	require.Equal(t, "EndPoll", m["username"])
	require.Contains(t, m["content"], "**✅ Google is UP**")
	require.Contains(t, m["content"], "https://www.google.com")

	require.NoError(t, d.send("evt", "EndPoll status", ""))
	require.Equal(t, "EndPoll status", jsonBody(t, srv.last().body)["content"])
}

func TestTeams(t *testing.T) {
	srv := newCapture(http.StatusOK)
	defer srv.Close()

	c := &Teams{url: srv.URL}
	require.Equal(t, "teams", c.string())

	subject, body := c.normalize(testHost(), types.DOWN)
	require.NoError(t, c.send("evt", subject, body))

	m := jsonBody(t, srv.last().body)
	require.Equal(t, "message", m["type"])
	attachments := m["attachments"].([]any)
	require.Len(t, attachments, 1)
	card := attachments[0].(map[string]any)["content"].(map[string]any)
	require.Equal(t, "AdaptiveCard", card["type"])
	blocks := card["body"].([]any)
	require.Len(t, blocks, 2)
	require.Equal(t, "❌ Google is DOWN", blocks[0].(map[string]any)["text"])
	require.Contains(t, blocks[1].(map[string]any)["text"], "Google (https://www.google.com) has a new status: DOWN")
}

func TestMattermost(t *testing.T) {
	srv := newCapture(http.StatusOK)
	defer srv.Close()

	m := &Mattermost{url: srv.URL, channel: "ops", username: "EndPoll"}
	require.Equal(t, "mattermost", m.string())

	subject, body := m.normalize(testHost(), types.DOWN)
	require.NoError(t, m.send("evt", subject, body))

	p := jsonBody(t, srv.last().body)
	require.Equal(t, "ops", p["channel"])
	require.Equal(t, "EndPoll", p["username"])
	require.Contains(t, p["text"], "**❌ Google is DOWN**")
}

func TestPushover(t *testing.T) {
	srv := newCapture(http.StatusOK)
	defer srv.Close()

	p := &Pushover{url: srv.URL, token: "app", user: "usr", priority: 2}
	require.Equal(t, "pushover", p.string())

	subject, body := p.normalize(testHost(), types.DOWN)
	require.NoError(t, p.send("evt", subject, body))

	r := srv.last()
	require.Contains(t, r.headers.Get("Content-Type"), "application/x-www-form-urlencoded")
	values, err := url.ParseQuery(string(r.body))
	require.NoError(t, err)
	require.Equal(t, "app", values.Get("token"))
	require.Equal(t, "usr", values.Get("user"))
	require.Equal(t, "❌ Google is DOWN", values.Get("title"))
	require.Contains(t, values.Get("message"), "has a new status: DOWN")
	require.Equal(t, "2", values.Get("priority"))
	require.Equal(t, "60", values.Get("retry"), "emergency priority requires retry")
	require.Equal(t, "3600", values.Get("expire"), "emergency priority requires expire")

	p.priority = 0
	require.NoError(t, p.send("evt", subject, body))
	values, _ = url.ParseQuery(string(srv.last().body))
	require.Empty(t, values.Get("retry"))
}

func TestNtfy(t *testing.T) {
	srv := newCapture(http.StatusOK)
	defer srv.Close()

	n := &Ntfy{url: srv.URL + "/", topic: "endpoll", token: "tk_1", priority: 4}
	require.Equal(t, "ntfy", n.string())

	subject, body := n.normalize(testHost(), types.DOWN)
	require.NoError(t, n.send("evt", subject, body))

	r := srv.last()
	require.Equal(t, "/", r.path)
	require.Equal(t, "Bearer tk_1", r.headers.Get("Authorization"))
	m := jsonBody(t, r.body)
	require.Equal(t, "endpoll", m["topic"])
	require.Equal(t, "❌ Google is DOWN", m["title"])
	require.Equal(t, float64(4), m["priority"])
	require.Equal(t, []any{"x"}, m["tags"])

	subject, body = n.normalize(testHost(), types.UP)
	require.NoError(t, n.send("evt", subject, body))
	require.Equal(t, []any{"white_check_mark"}, jsonBody(t, srv.last().body)["tags"])

	n.token = ""
	require.NoError(t, n.send("evt", "EndPoll status", "I'm online"))
	require.Empty(t, srv.last().headers.Get("Authorization"))
}

func TestTwilio(t *testing.T) {
	srv := newCapture(http.StatusCreated)
	defer srv.Close()

	tw := &Twilio{url: srv.URL, accountSID: "AC1", authToken: "secret", from: "+1500", to: []string{"+1501", "+1502"}}
	require.Equal(t, "twilio", tw.string())

	subject, body := tw.normalize(testHost(), types.DOWN)
	require.Equal(t, "❌ Google is DOWN\nhttps://www.google.com", body)
	require.NoError(t, tw.send("evt", subject, body))
	require.Equal(t, 2, srv.count(), "one SMS per recipient")

	r := srv.last()
	require.Equal(t, "/2010-04-01/Accounts/AC1/Messages.json", r.path)
	require.Equal(t, "Basic QUMxOnNlY3JldA==", r.headers.Get("Authorization"))
	values, err := url.ParseQuery(string(r.body))
	require.NoError(t, err)
	require.Equal(t, "+1500", values.Get("From"))
	require.Equal(t, "+1502", values.Get("To"))
	require.Equal(t, body, values.Get("Body"))

	t.Run("one failing recipient does not stop the others", func(t *testing.T) {
		bad := newCapture(http.StatusUnauthorized)
		defer bad.Close()
		tw := &Twilio{url: bad.URL, accountSID: "AC1", authToken: "x", from: "+1500", to: []string{"+1501", "+1502"}}
		err := tw.send("evt", "s", "b")
		require.Error(t, err)
		require.Equal(t, 2, bad.count())
		require.Contains(t, err.Error(), "+1501")
		require.Contains(t, err.Error(), "+1502")
	})
}

func TestNew_Providers(t *testing.T) {
	cfg := &types.Cfg{Notifications: types.Notifications{
		Webhook:    &types.Webhook{URL: "http://w"},
		Discord:    &types.Discord{WebhookURL: "http://d"},
		Teams:      &types.Teams{WebhookURL: "http://t"},
		Mattermost: &types.Mattermost{WebhookURL: "http://m"},
		Pushover:   &types.Pushover{Token: "t", User: "u"},
		Ntfy:       &types.Ntfy{Topic: "endpoll"},
		Twilio:     &types.Twilio{AccountSID: "a", AuthToken: "b", From: "c", To: []string{"d"}},
	}}
	n, err := New(cfg)
	require.NoError(t, err)

	names := []string{}
	for _, c := range n.clients {
		names = append(names, c.string())
	}
	require.Equal(t, []string{"webhook", "discord", "teams", "mattermost", "pushover", "ntfy", "twilio"}, names)

	ntfy := n.clients[5].(*Ntfy)
	require.Equal(t, "https://ntfy.sh", ntfy.url)
	require.Equal(t, 3, ntfy.priority)
}

func TestNotifications_Validate(t *testing.T) {
	cases := map[string]types.Notifications{
		"webhook without url":       {Webhook: &types.Webhook{}},
		"discord without url":       {Discord: &types.Discord{}},
		"teams without url":         {Teams: &types.Teams{}},
		"mattermost without url":    {Mattermost: &types.Mattermost{}},
		"pushover without user":     {Pushover: &types.Pushover{Token: "t"}},
		"pushover invalid priority": {Pushover: &types.Pushover{Token: "t", User: "u", Priority: 3}},
		"ntfy without topic":        {Ntfy: &types.Ntfy{}},
		"ntfy invalid priority":     {Ntfy: &types.Ntfy{Topic: "t", Priority: 6}},
		"twilio without to":         {Twilio: &types.Twilio{AccountSID: "a", AuthToken: "b", From: "c"}},
		"slack without token":       {Slack: &types.Slack{Channel: "c"}},
	}
	for name, n := range cases {
		t.Run(name, func(t *testing.T) {
			cfg := &types.Cfg{Notifications: n, FileHosts: []*types.Host{{URL: "https://a"}}}
			require.Error(t, cfg.Validate())
		})
	}

	cfg := &types.Cfg{
		Notifications: types.Notifications{Ntfy: &types.Ntfy{Topic: "t"}, Webhook: &types.Webhook{URL: "http://w"}},
		FileHosts:     []*types.Host{{URL: "https://a"}},
	}
	require.NoError(t, cfg.Validate())
}
