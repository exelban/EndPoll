package notify

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/exelban/EndPoll/types"
	"golang.org/x/sync/errgroup"
)

type Telegram struct {
	token   string
	chatIDs []string

	timeout time.Duration
}

func (t *Telegram) string() string {
	return "telegram"
}

func (t *Telegram) send(subject, body string) error {
	g, _ := errgroup.WithContext(context.Background())
	for _, chatID := range t.chatIDs {
		id := chatID
		g.Go(func() error {
			return t.sendToChat(id, body)
		})
	}
	return g.Wait()
}

func (t *Telegram) normalize(host *types.Host, status types.StatusType) (string, string) {
	icon := "❌"
	if status == types.UP {
		icon = "✅"
	}

	name := host.URL
	if host.Name != nil && *host.Name != "" {
		name = *host.Name
	}

	text := fmt.Sprintf("%s: `%s` has a new status: %s", icon, host.String(), strings.ToUpper(string(status)))
	subject := fmt.Sprintf("%s: %s is %s", icon, name, strings.ToUpper(string(status)))

	return subject, text
}

func (t *Telegram) sendToChat(chatID, msg string) error {
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
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(b))
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
