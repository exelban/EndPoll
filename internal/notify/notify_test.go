package notify

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	providers "github.com/exelban/EndPoll/pkg/notify"
	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("no providers", func(t *testing.T) {
		n, err := New(&types.Cfg{})
		require.NoError(t, err)
		require.Empty(t, n.clients)
	})
	t.Run("init slack error", func(t *testing.T) {
		n, err := New(&types.Cfg{
			Notifications: types.Notifications{
				Slack: &types.Slack{
					Channel: "test",
					Token:   "test",
				},
			},
		})
		require.NoError(t, err)
		require.NotNil(t, n)
	})
}

func TestNotify_Set(t *testing.T) {
	m := &providerMock{
		StringFunc: func() string {
			return "mock"
		},
		SendFunc: func(_ context.Context, _, subject, body string) error {
			if strings.Contains(body, "test_ok") {
				return nil
			}
			return errors.New("error")
		},
	}

	n := &Notify{
		clients: []provider{m},
	}

	require.NoError(t, n.Set(context.Background(), nil, types.UP, "test_ok", "addr"))
	require.Error(t, n.Set(context.Background(), nil, types.UP, "error", "addr"))
}

func TestNotify_Send(t *testing.T) {
	t.Run("cancels active and waiting sends", func(t *testing.T) {
		started := make(chan struct{})
		release := make(chan struct{})
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = io.Copy(io.Discard, r.Body)
			close(started)
			select {
			case <-r.Context().Done():
			case <-release:
			}
		}))
		defer srv.Close()
		defer close(release)

		n := &Notify{clients: []provider{providers.NewWebhook(&types.Webhook{URL: srv.URL})}}
		ctx, cancel := context.WithCancel(context.Background())
		defer cancel()

		done := make(chan error, 2)
		host := &types.Host{ID: "host", URL: "http://example.com"}
		go func() {
			done <- n.Send(ctx, host, types.DOWN)
		}()

		select {
		case <-started:
		case <-time.After(time.Second):
			t.Fatal("notification did not start")
		}
		go func() {
			done <- n.Send(ctx, host, types.UP)
		}()
		cancel()

		for i := 0; i < 2; i++ {
			select {
			case err := <-done:
				require.ErrorIs(t, err, context.Canceled)
			case <-time.After(time.Second):
				t.Fatal("notification did not stop")
			}
		}
	})

	t.Run("sends to all clients when no host alerts", func(t *testing.T) {
		callCount := 0
		m := &providerMock{
			StringFunc: func() string { return "mock" },
			NormalizeFunc: func(host *types.Host, status types.StatusType) (string, string) {
				return "subject", "body"
			},
			SendFunc: func(_ context.Context, _, subject, body string) error {
				callCount++
				return nil
			},
		}
		n := &Notify{clients: []provider{m}}
		require.NoError(t, n.Send(context.Background(), &types.Host{URL: "http://test.com"}, types.UP))
		require.Equal(t, 1, callCount)
	})

	t.Run("filters by host alerts", func(t *testing.T) {
		slackCalls := 0
		telegramCalls := 0

		slack := &providerMock{
			StringFunc: func() string { return "slack" },
			NormalizeFunc: func(host *types.Host, status types.StatusType) (string, string) {
				return "s", "b"
			},
			SendFunc: func(_ context.Context, _, subject, body string) error {
				slackCalls++
				return nil
			},
		}
		telegram := &providerMock{
			StringFunc: func() string { return "telegram" },
			NormalizeFunc: func(host *types.Host, status types.StatusType) (string, string) {
				return "s", "b"
			},
			SendFunc: func(_ context.Context, _, subject, body string) error {
				telegramCalls++
				return nil
			},
		}

		n := &Notify{clients: []provider{slack, telegram}}
		host := &types.Host{
			URL:    "http://test.com",
			Alerts: []string{"slack"}, // only slack
		}
		require.NoError(t, n.Send(context.Background(), host, types.DOWN))
		require.Equal(t, 1, slackCalls)
		require.Equal(t, 0, telegramCalls)
	})

	t.Run("returns error from client", func(t *testing.T) {
		m := &providerMock{
			StringFunc: func() string { return "mock" },
			NormalizeFunc: func(host *types.Host, status types.StatusType) (string, string) {
				return "s", "b"
			},
			SendFunc: func(_ context.Context, _, subject, body string) error {
				return errors.New("send failed")
			},
		}
		n := &Notify{clients: []provider{m}}
		err := n.Send(context.Background(), &types.Host{URL: "http://test.com"}, types.DOWN)
		require.Error(t, err)
		require.Contains(t, err.Error(), "send failed")
	})

	t.Run("no clients does not error", func(t *testing.T) {
		n := &Notify{}
		require.NoError(t, n.Send(context.Background(), &types.Host{URL: "http://test.com"}, types.UP))
	})
}

func TestNotify_New_InitializationMessage(t *testing.T) {
	t.Run("default sends init message", func(t *testing.T) {
		cfg := &types.Cfg{}
		n, err := New(cfg)
		require.NoError(t, err)
		require.NotNil(t, n)
		require.True(t, *cfg.Notifications.InitializationMessage)
	})

	t.Run("disabled does not send init message", func(t *testing.T) {
		f := false
		cfg := &types.Cfg{
			Notifications: types.Notifications{
				InitializationMessage: &f,
			},
		}
		n, err := New(cfg)
		require.NoError(t, err)
		require.NotNil(t, n)
	})
}

func TestNotify_Set_WithClients(t *testing.T) {
	t.Run("sends to matching client", func(t *testing.T) {
		sent := false
		m := &providerMock{
			StringFunc: func() string { return "slack" },
			SendFunc: func(_ context.Context, _, subject, body string) error {
				sent = true
				require.Contains(t, body, "test-host")
				return nil
			},
		}
		n := &Notify{clients: []provider{m}}
		require.NoError(t, n.Set(context.Background(), []string{"slack"}, types.UP, "test-host", "http://test.com"))
		require.True(t, sent)
	})

	t.Run("skips non-matching client", func(t *testing.T) {
		sent := false
		m := &providerMock{
			StringFunc: func() string { return "telegram" },
			SendFunc: func(_ context.Context, _, subject, body string) error {
				sent = true
				return nil
			},
		}
		n := &Notify{clients: []provider{m}}
		require.NoError(t, n.Set(context.Background(), []string{"slack"}, types.UP, "test-host", "http://test.com"))
		require.False(t, sent)
	})

	t.Run("nil clients sends to all", func(t *testing.T) {
		callCount := 0
		m := &providerMock{
			StringFunc: func() string { return "mock" },
			SendFunc: func(_ context.Context, _, subject, body string) error {
				callCount++
				return nil
			},
		}
		n := &Notify{clients: []provider{m}}
		require.NoError(t, n.Set(context.Background(), nil, types.DOWN, "host", "addr"))
		require.Equal(t, 1, callCount)
	})
}

func TestNew_Providers(t *testing.T) {
	cfg := &types.Cfg{Notifications: types.Notifications{
		Slack:      &types.Slack{Token: "t", Channel: "c"},
		Telegram:   &types.Telegram{Token: "t", ChatIDs: []string{"c"}},
		SMTP:       &types.SMTP{Host: "smtp.local", From: "a@local", To: []string{"b@local"}},
		Webhook:    &types.Webhook{URL: "http://w"},
		Discord:    &types.Discord{WebhookURL: "http://d"},
		Teams:      &types.Teams{WebhookURL: "http://t"},
		Mattermost: &types.Mattermost{WebhookURL: "http://m"},
		Pushover:   &types.Pushover{Token: "t", User: "u"},
		Ntfy:       &types.Ntfy{Topic: "endpoll"},
		Twilio:     &types.Twilio{AccountSID: "a", AuthToken: "b", From: "c", To: []string{"d"}},
		BotComm:    &types.BotComm{ClientID: "c", ClientSecret: "s"},
	}}
	n, err := New(cfg)
	require.NoError(t, err)

	names := []string{}
	for _, c := range n.clients {
		names = append(names, c.String())
	}
	require.Equal(t, []string{"slack", "telegram", "smtp", "webhook", "discord", "teams", "mattermost", "pushover", "ntfy", "twilio", "botcomm"}, names)
}

func TestNew_BotComm(t *testing.T) {
	cfg := &types.Cfg{Notifications: types.Notifications{
		BotComm: &types.BotComm{ClientID: "client", ClientSecret: "secret", SessionIDs: []string{"session"}},
	}}
	n, err := New(cfg)
	require.NoError(t, err)
	require.Len(t, n.clients, 1)
	b, ok := n.clients[0].(*providers.BotComm)
	require.True(t, ok)

	var messages []string
	n.clients[0] = &providerMock{
		StringFunc:    b.String,
		NormalizeFunc: b.Normalize,
		SendFunc: func(_ context.Context, _, subject, body string) error {
			messages = append(messages, body)
			return nil
		},
	}

	name := "Google"
	host := &types.Host{URL: "https://www.google.com", Name: &name, Alerts: []string{"telegram"}}
	require.NoError(t, n.Send(context.Background(), host, types.DOWN))
	require.Empty(t, messages)

	host.Alerts = []string{"botcomm"}
	require.NoError(t, n.Send(context.Background(), host, types.UP))
	require.Equal(t, []string{"✅ Google (https://www.google.com) has a new status: UP"}, messages)

	n.Startup(context.Background())
	require.Equal(t, []string{"✅ Google (https://www.google.com) has a new status: UP", "I'm online"}, messages)
}
