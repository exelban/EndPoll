package notify

import (
	"context"
	"encoding/base64"
	"errors"
	"net/http"
	"strings"
	"unicode"

	"github.com/exelban/EndPoll/types"
)

// botCommMaxSubjectLength - the subject limit in Unicode characters.
const botCommMaxSubjectLength = 256

// BotComm - messages via the BotComm message API.
type BotComm struct {
	url          string
	clientID     string
	clientSecret string
	sessionIDs   []string

	client *http.Client
}

// NewBotComm - creates a BotComm notification provider.
func NewBotComm(cfg *types.BotComm) *BotComm {
	return &BotComm{
		url:          "https://api.botcomm.app/message",
		clientID:     cfg.ClientID,
		clientSecret: cfg.ClientSecret,
		sessionIDs:   cfg.SessionIDs,
		client:       &http.Client{Timeout: httpTimeout},
	}
}

func (b *BotComm) String() string {
	return "botcomm"
}

func (b *BotComm) Send(ctx context.Context, _, subject, body string) error {
	for _, id := range b.sessionIDs {
		if strings.TrimSpace(id) == "" {
			return errors.New("botcomm: empty session ID")
		}
	}
	if body == "" {
		body = subject
	}
	subject = strings.TrimSpace(strings.Map(func(r rune) rune {
		if unicode.IsControl(r) || r == '\u2028' || r == '\u2029' {
			return ' '
		}
		return r
	}, subject))
	if chars := []rune(subject); len(chars) > botCommMaxSubjectLength {
		subject = string(chars[:botCommMaxSubjectLength-1]) + "…"
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
			Subject   string `json:"subject,omitempty"`
			SessionID string `json:"sessionID,omitempty"`
			Broadcast bool   `json:"broadcast,omitempty"`
		}{
			Content:   body,
			Subject:   subject,
			SessionID: id,
			Broadcast: broadcast,
		}
		if err := postJSON(ctx, b.client, http.MethodPost, b.url, headers, payload); err != nil {
			return err
		}
	}
	return nil
}

func (b *BotComm) Normalize(host *types.Host, status types.StatusType) (string, string) {
	subject, text := plainMessage(host, status)
	return subject, text
}
