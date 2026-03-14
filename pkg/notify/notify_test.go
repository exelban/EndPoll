package notify

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestNew(t *testing.T) {
	t.Run("no providers", func(t *testing.T) {
		n, err := New(context.Background(), &types.Cfg{})
		require.NoError(t, err)
		require.Empty(t, n.clients)
	})
	t.Run("init slack error", func(t *testing.T) {
		n, err := New(context.Background(), &types.Cfg{
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
	m := &notifyMock{
		stringFunc: func() string {
			return "mock"
		},
		sendFunc: func(subject, body string) error {
			if strings.Contains(body, "test_ok") {
				return nil
			}
			return errors.New("error")
		},
	}

	n := &Notify{
		clients: []notify{m},
	}

	require.NoError(t, n.Set(nil, types.UP, "test_ok", "addr"))
	require.Error(t, n.Set(nil, types.UP, "error", "addr"))
}

func TestNotify_Send(t *testing.T) {
	t.Run("sends to all clients when no host alerts", func(t *testing.T) {
		callCount := 0
		m := &notifyMock{
			stringFunc: func() string { return "mock" },
			normalizeFunc: func(host *types.Host, status types.StatusType) (string, string) {
				return "subject", "body"
			},
			sendFunc: func(subject, body string) error {
				callCount++
				return nil
			},
		}
		n := &Notify{clients: []notify{m}}
		require.NoError(t, n.Send(&types.Host{URL: "http://test.com"}, types.UP))
		require.Equal(t, 1, callCount)
	})

	t.Run("filters by host alerts", func(t *testing.T) {
		slackCalls := 0
		telegramCalls := 0

		slack := &notifyMock{
			stringFunc: func() string { return "slack" },
			normalizeFunc: func(host *types.Host, status types.StatusType) (string, string) {
				return "s", "b"
			},
			sendFunc: func(subject, body string) error {
				slackCalls++
				return nil
			},
		}
		telegram := &notifyMock{
			stringFunc: func() string { return "telegram" },
			normalizeFunc: func(host *types.Host, status types.StatusType) (string, string) {
				return "s", "b"
			},
			sendFunc: func(subject, body string) error {
				telegramCalls++
				return nil
			},
		}

		n := &Notify{clients: []notify{slack, telegram}}
		host := &types.Host{
			URL:    "http://test.com",
			Alerts: []string{"slack"}, // only slack
		}
		require.NoError(t, n.Send(host, types.DOWN))
		require.Equal(t, 1, slackCalls)
		require.Equal(t, 0, telegramCalls)
	})

	t.Run("returns error from client", func(t *testing.T) {
		m := &notifyMock{
			stringFunc: func() string { return "mock" },
			normalizeFunc: func(host *types.Host, status types.StatusType) (string, string) {
				return "s", "b"
			},
			sendFunc: func(subject, body string) error {
				return errors.New("send failed")
			},
		}
		n := &Notify{clients: []notify{m}}
		err := n.Send(&types.Host{URL: "http://test.com"}, types.DOWN)
		require.Error(t, err)
		require.Contains(t, err.Error(), "send failed")
	})

	t.Run("no clients does not error", func(t *testing.T) {
		n := &Notify{}
		require.NoError(t, n.Send(&types.Host{URL: "http://test.com"}, types.UP))
	})
}

func TestNotify_New_InitializationMessage(t *testing.T) {
	t.Run("default sends init message", func(t *testing.T) {
		cfg := &types.Cfg{}
		n, err := New(context.Background(), cfg)
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
		n, err := New(context.Background(), cfg)
		require.NoError(t, err)
		require.NotNil(t, n)
	})
}

func TestNotify_Set_WithClients(t *testing.T) {
	t.Run("sends to matching client", func(t *testing.T) {
		sent := false
		m := &notifyMock{
			stringFunc: func() string { return "slack" },
			sendFunc: func(subject, body string) error {
				sent = true
				require.Contains(t, body, "test-host")
				return nil
			},
		}
		n := &Notify{clients: []notify{m}}
		require.NoError(t, n.Set([]string{"slack"}, types.UP, "test-host", "http://test.com"))
		require.True(t, sent)
	})

	t.Run("skips non-matching client", func(t *testing.T) {
		sent := false
		m := &notifyMock{
			stringFunc: func() string { return "telegram" },
			sendFunc: func(subject, body string) error {
				sent = true
				return nil
			},
		}
		n := &Notify{clients: []notify{m}}
		require.NoError(t, n.Set([]string{"slack"}, types.UP, "test-host", "http://test.com"))
		require.False(t, sent)
	})

	t.Run("nil clients sends to all", func(t *testing.T) {
		callCount := 0
		m := &notifyMock{
			stringFunc: func() string { return "mock" },
			sendFunc: func(subject, body string) error {
				callCount++
				return nil
			},
		}
		n := &Notify{clients: []notify{m}}
		require.NoError(t, n.Set(nil, types.DOWN, "host", "addr"))
		require.Equal(t, 1, callCount)
	})
}
