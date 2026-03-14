package notify

import (
	"context"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/exelban/EndPoll/types"
)

//go:generate moq -out mock_test.go . notify

type notify interface {
	string() string
	send(subject, body string) error
	normalize(host *types.Host, status types.StatusType) (string, string)
}

type Notify struct {
	clients []notify

	mu sync.Mutex
}

func New(ctx context.Context, cfg *types.Cfg) (*Notify, error) {
	n := &Notify{}

	if cfg.Notifications.InitializationMessage == nil {
		t := true
		cfg.Notifications.InitializationMessage = &t
	}

	if cfg.Notifications.Slack != nil {
		slack := &Slack{
			url:     "https://slack.com/api/chat.postMessage",
			token:   cfg.Notifications.Slack.Token,
			channel: cfg.Notifications.Slack.Channel,
			timeout: time.Second * 10,
		}
		n.clients = append(n.clients, slack)
		log.Print("[INFO] Slack notifications enabled")
	}
	if cfg.Notifications.Telegram != nil {
		telegram := &Telegram{
			token:   cfg.Notifications.Telegram.Token,
			chatIDs: cfg.Notifications.Telegram.ChatIDs,
			timeout: time.Second * 10,
		}
		n.clients = append(n.clients, telegram)
		log.Print("[INFO] Telegram notifications enabled")
	}
	if cfg.Notifications.SMTP != nil {
		smtp := &SMTP{
			Host:     cfg.Notifications.SMTP.Host,
			Port:     cfg.Notifications.SMTP.Port,
			Username: cfg.Notifications.SMTP.Username,
			Password: cfg.Notifications.SMTP.Password,
			From:     cfg.Notifications.SMTP.From,
			To:       cfg.Notifications.SMTP.To,
		}
		n.clients = append(n.clients, smtp)
		log.Print("[INFO] SMTP notifications enabled")
	}

	if *cfg.Notifications.InitializationMessage {
		for _, client := range n.clients {
			if err := client.send("EndPoll status", "I'm online"); err != nil {
				log.Printf("[ERROR] send initialization message: %s", err)
			}
		}
	}

	go func() {
	loop:
		for {
			select {
			case <-ctx.Done():
				if cfg.Notifications.ShutdownMessage {
					for _, client := range n.clients {
						if err := client.send("EndPoll status", "Going offline..."); err != nil {
							log.Printf("[ERROR] send shutdown message: %s", err)
						}
					}
				}
				break loop
			}
		}
	}()

	return n, nil
}

func (n *Notify) Send(host *types.Host, status types.StatusType) error {
	clients := host.Alerts

	n.mu.Lock()
	defer n.mu.Unlock()

	for _, c := range n.clients {
		send := false
		if clients != nil && len(clients) > 0 {
			for _, client := range clients {
				if c.string() == client {
					send = true
					break
				}
			}
		} else {
			send = true
		}

		if !send {
			continue
		}

		subject, body := c.normalize(host, status)
		if err := c.send(subject, body); err != nil {
			return err
		}
	}

	return nil
}

func (n *Notify) Set(clients []string, status types.StatusType, name, addr string) error {
	icon := "❌"
	if status == types.UP {
		icon = "✅"
	}

	text := fmt.Sprintf("%s: `%s (%s)` has a new status: %s", icon, name, addr, strings.ToUpper(string(status)))
	subject := fmt.Sprintf("%s: %s is %s", icon, name, strings.ToUpper(string(status)))

	n.mu.Lock()
	defer n.mu.Unlock()

	for _, c := range n.clients {
		send := false
		if clients != nil && len(clients) > 0 {
			for _, client := range clients {
				if c.string() == client {
					send = true
					break
				}
			}
		} else {
			send = true
		}

		if !send {
			continue
		}

		if err := c.send(subject, text); err != nil {
			return err
		}
	}

	return nil
}
