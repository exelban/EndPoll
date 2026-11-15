package notify

import (
	"context"
	"net/http"

	"github.com/exelban/EndPoll/types"
)

// Discord - incoming webhook (https://discord.com/developers/docs/resources/webhook#execute-webhook)
type Discord struct {
	url      string
	username string

	client *http.Client
}

// NewDiscord - creates a Discord notification provider.
func NewDiscord(cfg *types.Discord) *Discord {
	return &Discord{
		url:      cfg.WebhookURL,
		username: cfg.Username,
		client:   &http.Client{Timeout: httpTimeout},
	}
}

func (d *Discord) String() string {
	return "discord"
}

func (d *Discord) Send(ctx context.Context, _, subject, body string) error {
	if body == "" {
		body = subject
	}
	payload := struct {
		Content  string `json:"content"`
		Username string `json:"username,omitempty"`
	}{
		Content:  body,
		Username: d.username,
	}
	return postJSON(ctx, d.client, http.MethodPost, d.url, nil, payload)
}

func (d *Discord) Normalize(host *types.Host, status types.StatusType) (string, string) {
	return boldMessage(host, status)
}
