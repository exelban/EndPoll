package notify

import (
	"context"
	"net/http"

	"github.com/exelban/EndPoll/types"
)

// Teams - Microsoft Teams incoming webhook. The message is an Adaptive Card,
// accepted by both the Workflows webhooks and the legacy Office 365 connectors.
type Teams struct {
	url string

	client *http.Client
}

// NewTeams - creates a Teams notification provider.
func NewTeams(cfg *types.Teams) *Teams {
	return &Teams{
		url:    cfg.WebhookURL,
		client: &http.Client{Timeout: httpTimeout},
	}
}

func (t *Teams) String() string {
	return "teams"
}

func (t *Teams) Send(ctx context.Context, _, subject, body string) error {
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
	return postJSON(ctx, t.client, http.MethodPost, t.url, nil, payload)
}

func (t *Teams) Normalize(host *types.Host, status types.StatusType) (string, string) {
	subject, text := plainMessage(host, status)
	return subject, text
}
