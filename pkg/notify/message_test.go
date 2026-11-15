package notify

import (
	"testing"

	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func TestProviders_Normalize_MessageFormat(t *testing.T) {
	name, empty := "My API", ""
	tests := []struct {
		name   string
		host   types.Host
		status types.StatusType
		code   [2]string
		bold   [2]string
	}{
		{
			name:   "named host",
			host:   types.Host{URL: "https://example.com", Name: &name},
			status: types.UP,
			code:   [2]string{"✅: My API is UP", "✅: `My API (https://example.com)` has a new status: UP"},
			bold:   [2]string{"✅ My API is UP", "**✅ My API is UP**\nhttps://example.com"},
		},
		{
			name:   "unnamed host",
			host:   types.Host{URL: "https://example.com"},
			status: types.DOWN,
			code:   [2]string{"❌: https://example.com is DOWN", "❌: `https://example.com` has a new status: DOWN"},
			bold:   [2]string{"❌ https://example.com is DOWN", "**❌ https://example.com is DOWN**\nhttps://example.com"},
		},
		{
			name:   "empty name",
			host:   types.Host{URL: "https://example.com", Name: &empty},
			status: types.Unknown,
			code:   [2]string{"❌: https://example.com is UNKNOWN", "❌: ` (https://example.com)` has a new status: UNKNOWN"},
			bold:   [2]string{"❌ https://example.com is UNKNOWN", "**❌ https://example.com is UNKNOWN**\nhttps://example.com"},
		},
		{
			name:   "credentials",
			host:   types.Host{URL: "https://user:password@example.com"},
			status: types.DEGRADED,
			code:   [2]string{"❌: https://user:password@example.com is DEGRADED", "❌: `https://user:*****@example.com` has a new status: DEGRADED"},
			bold:   [2]string{"❌ https://user:*****@example.com is DEGRADED", "**❌ https://user:*****@example.com is DEGRADED**\nhttps://user:*****@example.com"},
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			providers := []struct {
				name      string
				normalize func(*types.Host, types.StatusType) (string, string)
				expected  [2]string
			}{
				{"slack", (&Slack{}).Normalize, tc.code},
				{"telegram", (&Telegram{}).Normalize, tc.code},
				{"discord", (&Discord{}).Normalize, tc.bold},
				{"mattermost", (&Mattermost{}).Normalize, tc.bold},
			}
			for _, p := range providers {
				t.Run(p.name, func(t *testing.T) {
					subject, body := p.normalize(&tc.host, tc.status)
					require.Equal(t, p.expected, [2]string{subject, body})
				})
			}
		})
	}
}
