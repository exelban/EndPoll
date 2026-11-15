package notify

import (
	"context"
	"net/http"
	"strings"

	"github.com/exelban/EndPoll/types"
)

// Ntfy - push notifications via ntfy.sh or a self-hosted ntfy server.
// The JSON publishing endpoint is used, so non-ASCII titles are supported.
type Ntfy struct {
	url      string
	topic    string
	token    string
	priority int

	client *http.Client
}

// NewNtfy - creates a Ntfy notification provider.
func NewNtfy(cfg *types.Ntfy) *Ntfy {
	server := cfg.URL
	if server == "" {
		server = "https://ntfy.sh"
	}
	priority := cfg.Priority
	if priority == 0 {
		priority = 3
	}

	return &Ntfy{
		url:      server,
		topic:    cfg.Topic,
		token:    cfg.Token,
		priority: priority,
		client:   &http.Client{Timeout: httpTimeout},
	}
}

func (n *Ntfy) String() string {
	return "ntfy"
}

func (n *Ntfy) Send(ctx context.Context, _, subject, body string) error {
	if body == "" {
		body = subject
	}
	payload := struct {
		Topic    string   `json:"topic"`
		Title    string   `json:"title,omitempty"`
		Message  string   `json:"message"`
		Priority int      `json:"priority,omitempty"`
		Tags     []string `json:"tags,omitempty"`
	}{
		Topic:    n.topic,
		Title:    subject,
		Message:  body,
		Priority: n.priority,
	}
	switch {
	case strings.Contains(subject, "✅"):
		payload.Tags = []string{"white_check_mark"}
	case strings.Contains(subject, "❌"):
		payload.Tags = []string{"x"}
	}

	headers := map[string]string{}
	if n.token != "" {
		headers["Authorization"] = "Bearer " + n.token
	}
	return postJSON(ctx, n.client, http.MethodPost, strings.TrimSuffix(n.url, "/"), headers, payload)
}

func (n *Ntfy) Normalize(host *types.Host, status types.StatusType) (string, string) {
	subject, _ := plainMessage(host, status)
	text := host.String() + " has a new status: " + strings.ToUpper(string(status))
	return subject, text
}
