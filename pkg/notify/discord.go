package notify

import (
	"net/http"

	"github.com/exelban/EndPoll/types"
)

// Discord - incoming webhook (https://discord.com/developers/docs/resources/webhook#execute-webhook)
type Discord struct {
	url      string
	username string

	client *http.Client
}

func (d *Discord) string() string {
	return "discord"
}

func (d *Discord) send(_, subject, body string) error {
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
	return postJSON(d.client, http.MethodPost, d.url, nil, payload)
}

func (d *Discord) normalize(host *types.Host, status types.StatusType) (string, string) {
	subject, _ := plainMessage(host, status)
	text := "**" + subject + "**\n" + host.SecureURL()
	return subject, text
}
