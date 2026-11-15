package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/exelban/EndPoll/types"
)

type Slack struct {
	url     string
	token   string
	channel string

	timeout time.Duration
}

// NewSlack - creates a Slack notification provider.
func NewSlack(cfg *types.Slack) *Slack {
	return &Slack{
		url:     "https://slack.com/api/chat.postMessage",
		token:   cfg.Token,
		channel: cfg.Channel,
		timeout: httpTimeout,
	}
}

func (s *Slack) String() string {
	return "slack"
}

func (s *Slack) Send(ctx context.Context, _, subject, body string) error {
	b, _ := json.Marshal(struct {
		Username string `json:"username,omitempty"`
		Channel  string `json:"channel,omitempty"`
		Text     string `json:"text,omitempty"`
	}{
		Channel: s.channel,
		Text:    body,
	})

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, s.url, bytes.NewBuffer(b))
	if err != nil {
		return err
	}

	req.Header.Add("Content-Type", "application/json; charset=utf-8")
	req.Header.Add("Authorization", fmt.Sprintf("Bearer %s", s.token))

	client := &http.Client{
		Timeout: s.timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	buf := new(bytes.Buffer)
	_, err = buf.ReadFrom(resp.Body)
	if err != nil {
		return err
	}

	if !strings.Contains(buf.String(), "\"ok\":true") && !strings.Contains(buf.String(), "\"ok\": true") {
		return fmt.Errorf("non-ok (%d) response from Slack: %s", resp.StatusCode, buf.String())
	}

	return nil
}

func (s *Slack) Normalize(host *types.Host, status types.StatusType) (string, string) {
	return codeMessage(host, status)
}
