package notify

import (
	"testing"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestSMTP_string(t *testing.T) {
	s := &SMTP{}
	require.Equal(t, "smtp", s.string())
}

func TestSMTP_normalize(t *testing.T) {
	s := &SMTP{}

	t.Run("host with name", func(t *testing.T) {
		name := "Production API"
		host := &types.Host{URL: "https://api.example.com", Name: &name}
		subject, body := s.normalize(host, types.DOWN)

		require.Contains(t, subject, "Production API")
		require.Contains(t, subject, "DOWN")
		require.Contains(t, subject, "❌")

		require.Contains(t, body, "Production API")
		require.Contains(t, body, "DOWN")
		require.Contains(t, body, "https://api.example.com")
		require.Contains(t, body, "<h2>")
	})

	t.Run("host without name", func(t *testing.T) {
		host := &types.Host{URL: "https://api.example.com"}
		subject, body := s.normalize(host, types.UP)

		require.Contains(t, subject, "https://api.example.com")
		require.Contains(t, subject, "UP")
		require.Contains(t, subject, "✅")
		require.Contains(t, body, "https://api.example.com")
	})

	t.Run("body contains HTML", func(t *testing.T) {
		host := &types.Host{URL: "https://test.com"}
		_, body := s.normalize(host, types.DOWN)

		require.Contains(t, body, "<h2>")
		require.Contains(t, body, "<li>")
		require.Contains(t, body, "href=")
	})
}
