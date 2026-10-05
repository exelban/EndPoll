package notify

import (
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestNotify_Send_Cooldown(t *testing.T) {
	var sent atomic.Int32
	m := &notifyMock{
		stringFunc:    func() string { return "smtp" },
		normalizeFunc: func(host *types.Host, status types.StatusType) (string, string) { return "s", "b" },
		sendFunc: func(_, subject, body string) error {
			sent.Add(1)
			return nil
		},
	}
	n := &Notify{clients: []notify{m}, cooldown: time.Hour}
	host := &types.Host{ID: "h1", URL: "http://test"}

	require.NoError(t, n.Send(host, types.DOWN))
	require.NoError(t, n.Send(host, types.DOWN))
	require.NoError(t, n.Send(host, types.DOWN))
	require.Equal(t, int32(1), sent.Load(), "identical notification must be sent once within the cooldown")

	require.NoError(t, n.Send(host, types.UP))
	require.Equal(t, int32(2), sent.Load(), "a different status is a different notification")

	other := &types.Host{ID: "h2", URL: "http://other"}
	require.NoError(t, n.Send(other, types.DOWN))
	require.Equal(t, int32(3), sent.Load(), "a different host is a different notification")

	// expired entry: sent again
	n.mu.Lock()
	n.sent["smtp|h1|down"] = time.Now().Add(-2 * time.Hour)
	n.mu.Unlock()
	require.NoError(t, n.Send(host, types.DOWN))
	require.Equal(t, int32(4), sent.Load())
}

func TestNotify_Send_CooldownDisabled(t *testing.T) {
	var sent atomic.Int32
	m := &notifyMock{
		stringFunc:    func() string { return "slack" },
		normalizeFunc: func(host *types.Host, status types.StatusType) (string, string) { return "s", "b" },
		sendFunc: func(_, subject, body string) error {
			sent.Add(1)
			return nil
		},
	}
	n := &Notify{clients: []notify{m}}
	host := &types.Host{ID: "h1", URL: "http://test"}

	require.NoError(t, n.Send(host, types.DOWN))
	require.NoError(t, n.Send(host, types.DOWN))
	require.Equal(t, int32(2), sent.Load())
}

func TestNotify_Send_FailedSendIsNotCached(t *testing.T) {
	var calls atomic.Int32
	m := &notifyMock{
		stringFunc:    func() string { return "slack" },
		normalizeFunc: func(host *types.Host, status types.StatusType) (string, string) { return "s", "b" },
		sendFunc: func(_, subject, body string) error {
			if calls.Add(1) == 1 {
				return errors.New("boom")
			}
			return nil
		},
	}
	n := &Notify{clients: []notify{m}, cooldown: time.Hour}
	host := &types.Host{ID: "h1", URL: "http://test"}

	require.Error(t, n.Send(host, types.DOWN))
	require.NoError(t, n.Send(host, types.DOWN), "a failed delivery must be retried on the next status change")
	require.Equal(t, int32(2), calls.Load())
}

func TestNotify_Send_ContinuesAfterError(t *testing.T) {
	var second atomic.Int32
	failing := &notifyMock{
		stringFunc:    func() string { return "slack" },
		normalizeFunc: func(host *types.Host, status types.StatusType) (string, string) { return "s", "b" },
		sendFunc:      func(_, subject, body string) error { return errors.New("slack down") },
	}
	ok := &notifyMock{
		stringFunc:    func() string { return "telegram" },
		normalizeFunc: func(host *types.Host, status types.StatusType) (string, string) { return "s", "b" },
		sendFunc: func(_, subject, body string) error {
			second.Add(1)
			return nil
		},
	}
	n := &Notify{clients: []notify{failing, ok}}

	err := n.Send(&types.Host{ID: "h1", URL: "http://test"}, types.DOWN)
	require.Error(t, err)
	require.Contains(t, err.Error(), "slack down")
	require.Equal(t, int32(1), second.Load(), "the remaining channels must be notified even if one failed")
}

func TestNotify_StartupShutdown(t *testing.T) {
	var messages []string
	m := &notifyMock{
		stringFunc: func() string { return "slack" },
		sendFunc: func(_, subject, body string) error {
			messages = append(messages, body)
			return nil
		},
	}

	n := &Notify{clients: []notify{m}, initializationMessage: true, shutdownMessage: false}
	n.Startup()
	n.Shutdown()
	require.Equal(t, []string{"I'm online"}, messages)

	messages = nil
	n = &Notify{clients: []notify{m}, initializationMessage: false, shutdownMessage: true}
	n.Startup()
	n.Shutdown()
	require.Equal(t, []string{"Going offline..."}, messages)
}
