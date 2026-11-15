package notify

import (
	"context"
	"net/http"

	"github.com/exelban/EndPoll/types"
)

// Mattermost - Slack-compatible incoming webhook. Works with Mattermost,
// Rocket.Chat and any other service accepting the Slack webhook payload.
type Mattermost struct {
	url      string
	channel  string
	username string

	client *http.Client
}

// NewMattermost - creates a Mattermost notification provider.
func NewMattermost(cfg *types.Mattermost) *Mattermost {
	return &Mattermost{
		url:      cfg.WebhookURL,
		channel:  cfg.Channel,
		username: cfg.Username,
		client:   &http.Client{Timeout: httpTimeout},
	}
}

func (m *Mattermost) String() string {
	return "mattermost"
}

func (m *Mattermost) Send(ctx context.Context, _, subject, body string) error {
	if body == "" {
		body = subject
	}
	payload := struct {
		Text     string `json:"text"`
		Channel  string `json:"channel,omitempty"`
		Username string `json:"username,omitempty"`
	}{
		Text:     body,
		Channel:  m.channel,
		Username: m.username,
	}
	return postJSON(ctx, m.client, http.MethodPost, m.url, nil, payload)
}

func (m *Mattermost) Normalize(host *types.Host, status types.StatusType) (string, string) {
	return boldMessage(host, status)
}
