package notify

import (
	"encoding/json"
	"net/http"
	"time"

	"github.com/exelban/EndPoll/types"
)

// Webhook - generic HTTP webhook. Every notification is delivered as a JSON document:
//
//	{
//	  "id": "<event id>", "type": "host", "subject": "...", "status": "down",
//	  "host": {"id": "...", "name": "...", "url": "...", "group": "..."},
//	  "timestamp": "2006-01-02T15:04:05Z"
//	}
//
// The system messages (online/offline) have "type": "system" and a "message" field.
type Webhook struct {
	url     string
	method  string
	headers map[string]string

	client *http.Client
}

func (w *Webhook) string() string {
	return "webhook"
}

func (w *Webhook) send(id, subject, body string) error {
	payload := map[string]any{}
	if err := json.Unmarshal([]byte(body), &payload); err != nil {
		payload = map[string]any{
			"type":      "system",
			"message":   body,
			"timestamp": time.Now().UTC().Format(time.RFC3339),
		}
	}
	payload["id"] = id
	payload["subject"] = subject

	return postJSON(w.client, w.method, w.url, w.headers, payload)
}

func (w *Webhook) normalize(host *types.Host, status types.StatusType) (string, string) {
	subject, _ := plainMessage(host, status)

	h := map[string]any{
		"id":  host.ID,
		"url": host.SecureURL(),
	}
	if host.Name != nil {
		h["name"] = *host.Name
	}
	if host.Group != nil {
		h["group"] = *host.Group
	}

	payload := map[string]any{
		"type":      "host",
		"status":    status,
		"host":      h,
		"timestamp": time.Now().UTC().Format(time.RFC3339),
	}
	b, _ := json.Marshal(payload)

	return subject, string(b)
}
