package notify

import (
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

func (m *Mattermost) string() string {
	return "mattermost"
}

func (m *Mattermost) send(_, subject, body string) error {
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
	return postJSON(m.client, http.MethodPost, m.url, nil, payload)
}

func (m *Mattermost) normalize(host *types.Host, status types.StatusType) (string, string) {
	subject, _ := plainMessage(host, status)
	text := "**" + subject + "**\n" + host.SecureURL()
	return subject, text
}
