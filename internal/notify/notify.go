package notify

import (
	"context"
	"errors"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	providers "github.com/exelban/EndPoll/pkg/notify"
	"github.com/exelban/EndPoll/types"
)

//go:generate moq -out mock_test.go . provider

type provider interface {
	String() string
	// Send - delivers the message. The id identifies the event that triggered the
	// notification and is stable for the same event (used as the email Message-ID).
	Send(ctx context.Context, id, subject, body string) error
	Normalize(host *types.Host, status types.StatusType) (string, string)
}

// Notify - fan-out of the status notifications to the configured channels.
type Notify struct {
	clients []provider

	cooldown time.Duration
	sent     map[string]time.Time

	initializationMessage bool
	shutdownMessage       bool

	once sync.Once
	gate chan struct{}
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

	if c := cfg.Notifications.Slack; c != nil {
		n.clients = append(n.clients, providers.NewSlack(c))
		log.Print("[INFO] Slack notifications enabled")
	}
	if c := cfg.Notifications.Telegram; c != nil {
		n.clients = append(n.clients, providers.NewTelegram(c))
		log.Print("[INFO] Telegram notifications enabled")
	}
	if c := cfg.Notifications.SMTP; c != nil {
		n.clients = append(n.clients, providers.NewSMTP(c))
		log.Print("[INFO] SMTP notifications enabled")
	}
	if c := cfg.Notifications.Webhook; c != nil {
		n.clients = append(n.clients, providers.NewWebhook(c))
		log.Print("[INFO] Webhook notifications enabled")
	}
	if c := cfg.Notifications.Discord; c != nil {
		n.clients = append(n.clients, providers.NewDiscord(c))
		log.Print("[INFO] Discord notifications enabled")
	}
	if c := cfg.Notifications.Teams; c != nil {
		n.clients = append(n.clients, providers.NewTeams(c))
		log.Print("[INFO] Teams notifications enabled")
	}
	if c := cfg.Notifications.Mattermost; c != nil {
		n.clients = append(n.clients, providers.NewMattermost(c))
		log.Print("[INFO] Mattermost notifications enabled")
	}
	if c := cfg.Notifications.Pushover; c != nil {
		n.clients = append(n.clients, providers.NewPushover(c))
		log.Print("[INFO] Pushover notifications enabled")
	}
	if c := cfg.Notifications.Ntfy; c != nil {
		n.clients = append(n.clients, providers.NewNtfy(c))
		log.Print("[INFO] ntfy notifications enabled")
	}
	if c := cfg.Notifications.Twilio; c != nil {
		n.clients = append(n.clients, providers.NewTwilio(c))
		log.Print("[INFO] Twilio notifications enabled")
	}
	if c := cfg.Notifications.BotComm; c != nil {
		n.clients = append(n.clients, providers.NewBotComm(c))
		log.Print("[INFO] BotComm notifications enabled")
	}

	return n, nil
}

// Startup - sends the "online" message to every channel (if enabled).
// Must be called once per process, not on every configuration reload.
func (n *Notify) Startup(ctx context.Context) {
	if !n.initializationMessage {
		return
	}
	n.broadcast(ctx, "EndPoll status", "I'm online")
}

// Shutdown - sends the "offline" message to every channel (if enabled).
func (n *Notify) Shutdown(ctx context.Context) {
	if !n.shutdownMessage {
		return
	}
	n.broadcast(ctx, "EndPoll status", "Going offline...")
}

func (n *Notify) broadcast(ctx context.Context, subject, body string) {
	if err := n.lock(ctx); err != nil {
		return
	}
	defer n.unlock()

	id := eventID("endpoll", strings.ToLower(strings.ReplaceAll(body, " ", "-")))
	for _, client := range n.clients {
		if ctx.Err() != nil {
			return
		}
		if err := client.Send(ctx, id, subject, body); err != nil {
			log.Printf("[ERROR] send message to %s: %s", client.String(), err)
		}
	}
}

// Send - notifies about a new host status. The channels are filtered by the host alerts
// list (empty list means all channels). Every channel is tried even if a previous one
// failed; the errors are joined. Identical notifications sent within the cooldown
// window are suppressed.
func (n *Notify) Send(ctx context.Context, host *types.Host, status types.StatusType) error {
	id := host.ID
	if id == "" {
		id = host.URL
	}

	if err := n.lock(ctx); err != nil {
		return err
	}
	defer n.unlock()

	evt := eventID(id, string(status))
	var errs []error
	for _, c := range n.clients {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if !n.enabled(c, host.Alerts) {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s", c.String(), id, status)
		if n.suppressed(key) {
			log.Printf("[DEBUG] %s: %s notification (%s) suppressed by cooldown", host.String(), string(status), c.String())
			continue
		}

		subject, body := c.Normalize(host, status)
		if err := c.Send(ctx, evt, subject, body); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.String(), err))
			continue
		}
		n.markSent(key)
	}

	return errors.Join(errs...)
}

// Set - sends a custom status notification for the given name/address.
func (n *Notify) Set(ctx context.Context, clients []string, status types.StatusType, name, addr string) error {
	icon := "❌"
	if status == types.UP {
		icon = "✅"
	}

	text := fmt.Sprintf("%s: `%s (%s)` has a new status: %s", icon, name, addr, strings.ToUpper(string(status)))
	subject := fmt.Sprintf("%s: %s is %s", icon, name, strings.ToUpper(string(status)))

	if err := n.lock(ctx); err != nil {
		return err
	}
	defer n.unlock()

	evt := eventID(name, string(status))
	var errs []error
	for _, c := range n.clients {
		if err := ctx.Err(); err != nil {
			return errors.Join(append(errs, err)...)
		}
		if !n.enabled(c, clients) {
			continue
		}
		key := fmt.Sprintf("%s|%s|%s|%s", c.String(), name, addr, status)
		if n.suppressed(key) {
			log.Printf("[DEBUG] %s (%s): %s notification (%s) suppressed by cooldown", name, addr, string(status), c.String())
			continue
		}

		if err := c.Send(ctx, evt, subject, text); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", c.String(), err))
			continue
		}
		n.markSent(key)
	}

	return errors.Join(errs...)
}

func (n *Notify) lock(ctx context.Context) error {
	n.once.Do(func() {
		n.gate = make(chan struct{}, 1)
	})
	select {
	case n.gate <- struct{}{}:
		return nil
	case <-ctx.Done():
		return ctx.Err()
	}
}

func (n *Notify) unlock() {
	<-n.gate
}

// enabled - reports whether the channel is in the list of allowed channels (empty list allows all)
func (n *Notify) enabled(c provider, allowed []string) bool {
	if len(allowed) == 0 {
		return true
	}
	for _, a := range allowed {
		if c.String() == a {
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
