package notify

import (
	"encoding/base64"
	"errors"
	"net/http"
	"strings"

	"github.com/exelban/EndPoll/types"
)

// BotComm - messages via the BotComm message API.
type BotComm struct {
	url          string
	clientID     string
	clientSecret string
	sessionIDs   []string

	client *http.Client
}

func (b *BotComm) string() string {
	return "botcomm"
}

func (b *BotComm) send(_, subject, body string) error {
	for _, id := range b.sessionIDs {
		if strings.TrimSpace(id) == "" {
			return errors.New("botcomm: empty session ID")
		}
	}
	if body == "" {
		body = subject
	}

	broadcast := len(b.sessionIDs) == 0
	to := b.sessionIDs
	if broadcast {
		to = []string{""}
	}
	headers := map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(b.clientID+":"+b.clientSecret)),
	}

	for _, id := range to {
		payload := struct {
			Content   string `json:"content"`
			SessionID string `json:"sessionID,omitempty"`
			Broadcast bool   `json:"broadcast,omitempty"`
		}{
			Content:   body,
			SessionID: id,
			Broadcast: broadcast,
		}
		if err := postJSON(b.client, http.MethodPost, b.url, headers, payload); err != nil {
			return err
		}
	}
	return nil
}

func (b *BotComm) normalize(host *types.Host, status types.StatusType) (string, string) {
	subject, text := plainMessage(host, status)
	return subject, text
}
