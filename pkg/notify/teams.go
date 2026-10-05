package notify

import (
	"net/http"

	"github.com/exelban/EndPoll/types"
)

// Teams - Microsoft Teams incoming webhook. The message is an Adaptive Card,
// accepted by both the Workflows webhooks and the legacy Office 365 connectors.
type Teams struct {
	url string

	client *http.Client
}

func (t *Teams) string() string {
	return "teams"
}

func (t *Teams) send(_, subject, body string) error {
	blocks := []map[string]any{}
	if subject != "" {
		blocks = append(blocks, map[string]any{
			"type":   "TextBlock",
			"text":   subject,
			"weight": "Bolder",
			"size":   "Medium",
			"wrap":   true,
		})
	}
	if body != "" {
		blocks = append(blocks, map[string]any{
			"type": "TextBlock",
			"text": body,
			"wrap": true,
		})
	}

	payload := map[string]any{
		"type": "message",
		"attachments": []map[string]any{
			{
				"contentType": "application/vnd.microsoft.card.adaptive",
				"contentUrl":  nil,
				"content": map[string]any{
					"$schema": "http://adaptivecards.io/schemas/adaptive-card.json",
					"type":    "AdaptiveCard",
					"version": "1.4",
					"body":    blocks,
				},
			},
		},
	}
	return postJSON(t.client, http.MethodPost, t.url, nil, payload)
}

func (t *Teams) normalize(host *types.Host, status types.StatusType) (string, string) {
	subject, text := plainMessage(host, status)
	return subject, text
}
