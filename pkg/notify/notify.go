package notify

import (
	"errors"
	"fmt"
	"log"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/exelban/EndPoll/types"
)

//go:generate moq -out mock_test.go . notify

type notify interface {
	string() string
	// send - delivers the message. The id identifies the event that triggered the
	// notification and is stable for the same event (used as the email Message-ID).
	send(id, subject, body string) error
	normalize(host *types.Host, status types.StatusType) (string, string)
}

// Notify - fan-out of the status notifications to the configured channels.
type Notify struct {
	clients []notify

	cooldown time.Duration
	sent     map[string]time.Time

	initializationMessage bool
	shutdownMessage       bool

	mu sync.Mutex
}

func New(cfg *types.Cfg) (*Notify, error) {
	n := &Notify{
		sent: make(map[string]time.Time),
	}

	if cfg.Notifications.InitializationMessage == nil {
		t := true
		cfg.Notifications.InitializationMessage = &t
	}
	n.initializationMessage = *cfg.Notifications.InitializationMessage
	n.shutdownMessage = cfg.Notifications.ShutdownMessage
	if cfg.Notifications.Cooldown != nil {
		n.cooldown = *cfg.Notifications.Cooldown
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
			Host:               cfg.Notifications.SMTP.Host,
			Port:               cfg.Notifications.SMTP.Port,
			Username:           cfg.Notifications.SMTP.Username,
			Password:           cfg.Notifications.SMTP.Password,
			From:               cfg.Notifications.SMTP.From,
			To:                 cfg.Notifications.SMTP.To,
			InsecureSkipVerify: cfg.Notifications.SMTP.InsecureSkipVerify,
		}
		n.clients = append(n.clients, smtp)
		log.Print("[INFO] SMTP notifications enabled")
	}
	if c := cfg.Notifications.Webhook; c != nil {
		n.clients = append(n.clients, &Webhook{
			url:     c.URL,
			method:  c.Method,
			headers: c.Headers,
			client:  &http.Client{Timeout: httpTimeout},
		})
		log.Print("[INFO] Webhook notifications enabled")
	}
	if c := cfg.Notifications.Discord; c != nil {
		n.clients = append(n.clients, &Discord{
			url:      c.WebhookURL,
			username: c.Username,
			client:   &http.Client{Timeout: httpTimeout},
		})
		log.Print("[INFO] Discord notifications enabled")
	}
	if c := cfg.Notifications.Teams; c != nil {
		n.clients = append(n.clients, &Teams{
			url:    c.WebhookURL,
			client: &http.Client{Timeout: httpTimeout},
		})
		log.Print("[INFO] Teams notifications enabled")
	}
	if c := cfg.Notifications.Mattermost; c != nil {
		n.clients = append(n.clients, &Mattermost{
			url:      c.WebhookURL,
			channel:  c.Channel,
			username: c.Username,
			client:   &http.Client{Timeout: httpTimeout},
		})
		log.Print("[INFO] Mattermost notifications enabled")
	}
	if c := cfg.Notifications.Pushover; c != nil {
		n.clients = append(n.clients, &Pushover{
			url:      "https://api.pushover.net/1/messages.json",
			token:    c.Token,
			user:     c.User,
			priority: c.Priority,
			client:   &http.Client{Timeout: httpTimeout},
		})
		log.Print("[INFO] Pushover notifications enabled")
	}
	if c := cfg.Notifications.Ntfy; c != nil {
		server := c.URL
		if server == "" {
			server = "https://ntfy.sh"
		}
		priority := c.Priority
		if priority == 0 {
			priority = 3
		}
		n.clients = append(n.clients, &Ntfy{
			url:      server,
			topic:    c.Topic,
			token:    c.Token,
			priority: priority,
			client:   &http.Client{Timeout: httpTimeout},
		})
		log.Print("[INFO] ntfy notifications enabled")
	}
	if c := cfg.Notifications.Twilio; c != nil {
		n.clients = append(n.clients, &Twilio{
			url:        "https://api.twilio.com",
			accountSID: c.AccountSID,
			authToken:  c.AuthToken,
			from:       c.From,
			to:         c.To,
			client:     &http.Client{Timeout: httpTimeout},
		})
		log.Print("[INFO] Twilio notifications enabled")
	}
	if c := cfg.Notifications.BotComm; c != nil {
		n.clients = append(n.clients, &BotComm{
			url:          "https://api.botcomm.app/message",
			clientID:     c.ClientID,
			clientSecret: c.ClientSecret,
			sessionIDs:   c.SessionIDs,
			client:       &http.Client{Timeout: httpTimeout},
		})
		log.Print("[INFO] BotComm notifications enabled")
	}

	return n, nil
}

// Startup - sends the "online" message to every channel (if enabled).
// Must be called once per process, not on every configuration reload.
func (n *Notify) Startup() {
	if !n.initializationMessage {
		return
	}
	n.broadcast("EndPoll status", "I'm online")
}

// Shutdown - sends the "offline" message to every channel (if enabled).
func (n *Notify) Shutdown() {
	if !n.shutdownMessage {
		return
	}
	n.broadcast("EndPoll status", "Going offline...")
}

func (n *Notify) broadcast(subject, body string) {
	n.mu.Lock()
	defer n.mu.Unlock()

	id := eventID("endpoll", strings.ToLower(strings.ReplaceAll(body, " ", "-")))
	for _, client := range n.clients {
		if err := client.send(id, subject, body); err != nil {
			log.Printf("[ERROR] send message to %s: %s", client.string(), err)
		}
	}
}

// Send - notifies about a new host status. The channels are filtered by the host alerts
// list (empty list means all channels). Every channel is tried even if a previous one
// failed; the errors are joined. Identical notifications sent within the cooldown
// window are suppressed.
func (n *Notify) Send(host *types.Host, status types.StatusType) error {
	id := host.ID
	if id == "" {
		id = host.URL
	}

	n.mu.Lock()
	defer n.mu.Unlock()

	evt := eventID(id, string(status))
	var errs []error
	for _, c := range n.clients {
		if !n.enabled(c, host.Alerts) {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s", c.string(), id, status)
		if n.suppressed(key) {
			log.Printf("[DEBUG] %s: %s notification (%s) suppressed by cooldown", host.String(), string(status), c.string())
			continue
		}

		subject, body := c.normalize(host, status)
		if err := c.send(evt, subject, body); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.string(), err))
			continue
		}
		n.markSent(key)
	}

	return errors.Join(errs...)
}

// Set - sends a custom status notification for the given name/address.
func (n *Notify) Set(clients []string, status types.StatusType, name, addr string) error {
	icon := "❌"
	if status == types.UP {
		icon = "✅"
	}

	text := fmt.Sprintf("%s: `%s (%s)` has a new status: %s", icon, name, addr, strings.ToUpper(string(status)))
	subject := fmt.Sprintf("%s: %s is %s", icon, name, strings.ToUpper(string(status)))

	n.mu.Lock()
	defer n.mu.Unlock()

	evt := eventID(name, string(status))
	var errs []error
	for _, c := range n.clients {
		if !n.enabled(c, clients) {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s|%s", c.string(), name, addr, status)
		if n.suppressed(key) {
			log.Printf("[DEBUG] %s (%s): %s notification (%s) suppressed by cooldown", name, addr, string(status), c.string())
			continue
		}

		if err := c.send(evt, subject, text); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.string(), err))
			continue
		}
		n.markSent(key)
	}

	return errors.Join(errs...)
}

// enabled - reports whether the channel is in the list of allowed channels (empty list allows all)
func (n *Notify) enabled(c notify, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if c.string() == a {
			return true
		}
	}
	return false
}

// suppressed - reports whether the same notification was already sent within the cooldown.
// Must be called with the lock held.
func (n *Notify) suppressed(key string) bool {
	if n.cooldown <= 0 {
		return false
	}
	last, ok := n.sent[key]
	return ok && time.Since(last) < n.cooldown
}

// markSent - remembers when the notification was sent and drops the expired entries.
// Must be called with the lock held.
func (n *Notify) markSent(key string) {
	if n.cooldown <= 0 {
		return
	}
	if n.sent == nil {
		n.sent = make(map[string]time.Time)
	}
	now := time.Now()
	for k, ts := range n.sent {
		if now.Sub(ts) >= n.cooldown {
			delete(n.sent, k)
		}
	}
	n.sent[key] = now
}

// eventID - builds an identifier of the notification event: <parts>.<unix time>.
// It is used as the email Message-ID, so only the characters allowed in a
// dot-atom (RFC 5322) are kept, everything else is replaced by a dash.
func eventID(parts ...string) string {
	var b strings.Builder
	for _, p := range parts {
		if p == "" {
			continue
		}
		if b.Len() > 0 {
			b.WriteByte('.')
		}
		for _, r := range p {
			switch {
			case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
				b.WriteRune(r)
			default:
				b.WriteByte('-')
			}
		}
	}
	if b.Len() == 0 {
		b.WriteString("endpoll")
	}
	return fmt.Sprintf("%s.%d", b.String(), time.Now().UnixNano())
}
