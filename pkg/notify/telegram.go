package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/exelban/EndPoll/types"
	"golang.org/x/sync/errgroup"
)

type Telegram struct {
	token   string
	chatIDs []string

	timeout time.Duration
}

// NewTelegram - creates a Telegram notification provider.
func NewTelegram(cfg *types.Telegram) *Telegram {
	return &Telegram{
		token:   cfg.Token,
		chatIDs: cfg.ChatIDs,
		timeout: httpTimeout,
	}
}

func (t *Telegram) String() string {
	return "telegram"
}

func (t *Telegram) Send(ctx context.Context, _, subject, body string) error {
	var g errgroup.Group
	for _, chatID := range t.chatIDs {
		id := chatID
		g.Go(func() error {
			return t.sendToChat(ctx, id, body)
		})
	}
	return g.Wait()
}

func (t *Telegram) Normalize(host *types.Host, status types.StatusType) (string, string) {
	return codeMessage(host, status)
}

func (t *Telegram) sendToChat(ctx context.Context, chatID, msg string) error {
	b, err := json.Marshal(struct {
		ChatID string `json:"chat_id"`
		Text   string `json:"text"`
	}{
		ChatID: chatID,
		Text:   msg,
	})
	if err != nil {
		return fmt.Errorf("failed to marshal message: %w", err)
	}

	url := fmt.Sprintf("https://api.telegram.org/bot%s/sendMessage", t.token)
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(b))
	if err != nil {
		return fmt.Errorf("failed to create a new request: %w", err)
	}

	req.Header.Set("Content-Type", "application/json")

	client := http.Client{
		Timeout: t.timeout,
	}
	resp, err := client.Do(req)
	if err != nil {
		return fmt.Errorf("failed to send a command to the device: %w", err)
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	switch resp.StatusCode {
	case http.StatusOK:
		return nil
	}

	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return fmt.Errorf("read body: %w", err)
	}

	response := struct {
		Code        int    `json:"code"`
		Description string `json:"description"`
	}{}
	if err := json.Unmarshal(body, &response); err != nil {
		return fmt.Errorf("unmarshal body to the error: %w", err)
	}

	return errors.New(response.Description)
}
