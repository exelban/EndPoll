package types

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"os"
	"strings"
	"sync"
	"time"

	"gopkg.in/yaml.v3"
)

type Slack struct {
	Channel string `json:"channel" yaml:"channel"`
	Token   string `json:"token" yaml:"token"`
}

type Telegram struct {
	Token   string   `json:"token" yaml:"token"`
	ChatIDs []string `json:"chatIDs" yaml:"chatIDs"`
}

type SMTP struct {
	Host     string   `json:"host" yaml:"host"`
	Port     int      `json:"port" yaml:"port"`
	Username string   `json:"username" yaml:"username"`
	Password string   `json:"password" yaml:"password"`
	From     string   `json:"from" yaml:"from"`
	To       []string `json:"to" yaml:"to"`

	// InsecureSkipVerify disables TLS certificate verification for the SMTP connection.
	InsecureSkipVerify bool `json:"insecureSkipVerify" yaml:"insecureSkipVerify"`
}

// Webhook - generic HTTP webhook, receives a JSON payload with the event details.
type Webhook struct {
	URL     string            `json:"url" yaml:"url"`
	Method  string            `json:"method" yaml:"method"`   // default: POST
	Headers map[string]string `json:"headers" yaml:"headers"` // extra headers (e.g. Authorization)
}

// Discord - incoming webhook.
type Discord struct {
	WebhookURL string `json:"webhookURL" yaml:"webhookURL"`
	Username   string `json:"username" yaml:"username"` // overrides the webhook name (optional)
}

// Teams - Microsoft Teams incoming webhook (Workflows or legacy connector).
type Teams struct {
	WebhookURL string `json:"webhookURL" yaml:"webhookURL"`
}

// Mattermost - Slack-compatible incoming webhook (Mattermost, Rocket.Chat, ...).
type Mattermost struct {
	WebhookURL string `json:"webhookURL" yaml:"webhookURL"`
	Channel    string `json:"channel" yaml:"channel"`   // overrides the webhook channel (optional)
	Username   string `json:"username" yaml:"username"` // overrides the webhook username (optional)
}

// Pushover - push notifications via pushover.net.
type Pushover struct {
	Token    string `json:"token" yaml:"token"`       // application token
	User     string `json:"user" yaml:"user"`         // user or group key
	Priority int    `json:"priority" yaml:"priority"` // -2..2, 2 = emergency (repeats until acknowledged)
}

// Ntfy - push notifications via ntfy.sh or a self-hosted ntfy server.
type Ntfy struct {
	URL      string `json:"url" yaml:"url"`           // server url, default: https://ntfy.sh
	Topic    string `json:"topic" yaml:"topic"`       // topic name
	Token    string `json:"token" yaml:"token"`       // access token (optional)
	Priority int    `json:"priority" yaml:"priority"` // 1..5, default: 3
}

// Twilio - SMS via Twilio.
type Twilio struct {
	AccountSID string   `json:"accountSID" yaml:"accountSID"`
	AuthToken  string   `json:"authToken" yaml:"authToken"`
	From       string   `json:"from" yaml:"from"` // sender number or messaging service sid
	To         []string `json:"to" yaml:"to"`     // recipient numbers
}

// BotComm - messages via botcomm.app.
type BotComm struct {
	ClientID     string   `json:"clientID" yaml:"clientID"`
	ClientSecret string   `json:"clientSecret" yaml:"clientSecret"`
	SessionIDs   []string `json:"sessionIDs" yaml:"sessionIDs"` // omit to broadcast to everyone connected to the bot
}

type Notifications struct {
	Slack      *Slack      `json:"slack" yaml:"slack"`
	Telegram   *Telegram   `json:"telegram" yaml:"telegram"`
	SMTP       *SMTP       `json:"smtp" yaml:"smtp"`
	Webhook    *Webhook    `json:"webhook" yaml:"webhook"`
	Discord    *Discord    `json:"discord" yaml:"discord"`
	Teams      *Teams      `json:"teams" yaml:"teams"`
	Mattermost *Mattermost `json:"mattermost" yaml:"mattermost"`
	Pushover   *Pushover   `json:"pushover" yaml:"pushover"`
	Ntfy       *Ntfy       `json:"ntfy" yaml:"ntfy"`
	Twilio     *Twilio     `json:"twilio" yaml:"twilio"`
	BotComm    *BotComm    `json:"botcomm" yaml:"botcomm"`

	InitializationMessage *bool `json:"initializationMessage" yaml:"initializationMessage"`
	ShutdownMessage       bool  `json:"shutdownMessage" yaml:"shutdownMessage"`

	// Cooldown - minimal time between two identical notifications (same host, same status, same channel).
	// Prevents spamming when a host flaps. Default: 5m. Set to 0 to disable.
	Cooldown *time.Duration `json:"cooldown,omitempty" yaml:"cooldown,omitempty"`
}

// validate - checks that every configured provider has its required fields
func (n *Notifications) validate() error {
	if n.Slack != nil && (n.Slack.Token == "" || n.Slack.Channel == "") {
		return errors.New("slack: token and channel are required")
	}
	if n.Telegram != nil && (n.Telegram.Token == "" || len(n.Telegram.ChatIDs) == 0) {
		return errors.New("telegram: token and chatIDs are required")
	}
	if n.SMTP != nil && (n.SMTP.Host == "" || n.SMTP.From == "" || len(n.SMTP.To) == 0) {
		return errors.New("smtp: host, from and to are required")
	}
	if n.Webhook != nil && n.Webhook.URL == "" {
		return errors.New("webhook: url is required")
	}
	if n.Discord != nil && n.Discord.WebhookURL == "" {
		return errors.New("discord: webhookURL is required")
	}
	if n.Teams != nil && n.Teams.WebhookURL == "" {
		return errors.New("teams: webhookURL is required")
	}
	if n.Mattermost != nil && n.Mattermost.WebhookURL == "" {
		return errors.New("mattermost: webhookURL is required")
	}
	if n.Pushover != nil {
		if n.Pushover.Token == "" || n.Pushover.User == "" {
			return errors.New("pushover: token and user are required")
		}
		if n.Pushover.Priority < -2 || n.Pushover.Priority > 2 {
			return fmt.Errorf("pushover: priority must be between -2 and 2, got %d", n.Pushover.Priority)
		}
	}
	if n.Ntfy != nil {
		if n.Ntfy.Topic == "" {
			return errors.New("ntfy: topic is required")
		}
		if n.Ntfy.Priority < 0 || n.Ntfy.Priority > 5 {
			return fmt.Errorf("ntfy: priority must be between 1 and 5, got %d", n.Ntfy.Priority)
		}
	}
	if n.Twilio != nil && (n.Twilio.AccountSID == "" || n.Twilio.AuthToken == "" || n.Twilio.From == "" || len(n.Twilio.To) == 0) {
		return errors.New("twilio: accountSID, authToken, from and to are required")
	}
	if n.BotComm != nil {
		if n.BotComm.ClientID == "" || n.BotComm.ClientSecret == "" {
			return errors.New("botcomm: clientID and clientSecret are required")
		}
		for _, id := range n.BotComm.SessionIDs {
			if strings.TrimSpace(id) == "" {
				return errors.New("botcomm: sessionIDs must not contain empty IDs")
			}
		}
	}
	return nil
}

type BasicAuth struct {
	Username string `json:"username" yaml:"username"`
	Password string `json:"password" yaml:"password"`
}

type UI struct {
	Title     string     `json:"title" yaml:"title"`         // web page title
	HideURL   bool       `json:"hideURL" yaml:"hideURL"`     // allows to hide URL of the host in the UI
	BasicAuth *BasicAuth `json:"basicAuth" yaml:"basicAuth"` // enables basic authentication for the web UI
}

// Connectivity - configures the self connectivity check. When the monitor
// loses its own internet connection, failed host checks are skipped instead
// of being reported as DOWN, avoiding false positives (e.g. home internet outage).
type Connectivity struct {
	Disabled bool          `json:"disabled" yaml:"disabled"` // disables the connectivity check (enabled by default)
	Targets  []string      `json:"targets" yaml:"targets"`   // host:port endpoints to probe over TCP
	Interval time.Duration `json:"interval" yaml:"interval"` // how long a connectivity result is cached
	Timeout  time.Duration `json:"timeout" yaml:"timeout"`   // timeout for a single connectivity probe
}

const (
	DefaultInterval         = 30 * time.Second
	DefaultTimeout          = 60 * time.Second
	DefaultMaxConn          = 128
	DefaultSuccessThreshold = 1
	DefaultFailureThreshold = 2
	DefaultCooldown         = 5 * time.Minute
)

var defaultSuccessCodes = []int{200, 201, 202, 203, 204, 205, 206, 207, 208}

type Cfg struct {
	Interval     time.Duration  `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout      time.Duration  `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	InitialDelay *time.Duration `json:"initialDelay,omitempty" yaml:"initialDelay,omitempty"`
	MaxConn      int            `json:"maxConn,omitempty" yaml:"maxConn,omitempty"` // maximum concurrent dial connections

	SuccessThreshold int `json:"successThreshold" yaml:"successThreshold,omitempty"`
	FailureThreshold int `json:"failureThreshold" yaml:"failureThreshold,omitempty"`

	Conditions *Success          `json:"success" yaml:"success,omitempty"`
	Headers    map[string]string `json:"headers" yaml:"headers,omitempty"`

	UI            UI            `json:"ui" yaml:"ui"`
	Connectivity  *Connectivity `json:"connectivity" yaml:"connectivity,omitempty"`
	Notifications Notifications `json:"notifications" yaml:"notifications,omitempty"`
	FileHosts     []*Host       `json:"hosts" yaml:"hosts"`
	Hosts         []*Host       `json:"-" yaml:"-"`

	// DefaultSMTP - SMTP settings provided via CLI flags/env. Used when the
	// configuration file does not define notifications.smtp.
	DefaultSMTP *SMTP `json:"-" yaml:"-"`

	path string    `yaml:"-"`
	FW   chan bool `yaml:"-"`

	// mu guards every field that can be read concurrently with a reload
	// (UI is read by HTTP handlers, the rest by the reload loop only).
	mu sync.RWMutex

	// DEPRECATED: use Notifications instead of Alerts
	Alerts *Notifications `json:"alerts,omitempty" yaml:"alerts,omitempty"`
}

func NewConfig(ctx context.Context, path string) (*Cfg, error) {
	cfg := &Cfg{
		path: path,
		FW:   make(chan bool, 1),
	}

	if _, err := os.Stat(path); errors.Is(err, os.ErrNotExist) {
		if err := cfg.save(); err != nil {
			return nil, fmt.Errorf("save config: %w", err)
		}
	}
	if err := cfg.Parse(); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}

	go func() {
		ticker := time.NewTicker(time.Second)
		defer ticker.Stop()
		modTimestamp := time.Time{}
		for {
			select {
			case <-ticker.C:
				fi, err := os.Stat(path)
				if err != nil {
					continue
				}
				if fi.ModTime() != modTimestamp {
					if modTimestamp.IsZero() {
						log.Print("[DEBUG] loading config")
					} else {
						log.Printf("[DEBUG] config changed: %s -> %s",
							modTimestamp.Format(time.RFC3339Nano), fi.ModTime().Format(time.RFC3339Nano))
					}
					modTimestamp = fi.ModTime()
					select {
					case cfg.FW <- true:
					default: // a reload is already pending
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return cfg, nil
}

// Parse - reads the configuration file (json or yaml) into a fresh structure and,
// only if it was decoded successfully, replaces the current configuration with it.
// Parsing into a fresh structure guarantees that sections removed from the file
// are removed from the running configuration too (decoding into the existing
// structure would only merge new values on top of the old ones).
func (c *Cfg) Parse() error {
	bytes, err := os.ReadFile(c.path)
	if err != nil {
		return err
	}

	fresh := &Cfg{}
	switch {
	case strings.HasSuffix(c.path, ".yaml") || strings.HasSuffix(c.path, ".yml"):
		if err := yaml.Unmarshal(bytes, fresh); err != nil {
			return err
		}
	case strings.HasSuffix(c.path, ".json"):
		if err := json.Unmarshal(bytes, fresh); err != nil {
			return err
		}
	default:
		return fmt.Errorf("unknown configuration format `%s`", c.path)
	}

	c.mu.Lock()
	defer c.mu.Unlock()

	c.Interval = fresh.Interval
	c.Timeout = fresh.Timeout
	c.InitialDelay = fresh.InitialDelay
	c.MaxConn = fresh.MaxConn
	c.SuccessThreshold = fresh.SuccessThreshold
	c.FailureThreshold = fresh.FailureThreshold
	c.Conditions = fresh.Conditions
	c.Headers = fresh.Headers
	c.UI = fresh.UI
	c.Connectivity = fresh.Connectivity
	c.Notifications = fresh.Notifications
	c.FileHosts = fresh.FileHosts
	c.Alerts = fresh.Alerts

	return nil
}

// Validate - fills the defaults, validates the values and builds the list of hosts
// for monitoring. Hosts are always rebuilt from scratch: the previous list is left
// untouched (watchers may still be using it) and replaced only on success.
func (c *Cfg) Validate() error {
	c.mu.Lock()
	defer c.mu.Unlock()

	if c.Interval == 0 {
		c.Interval = DefaultInterval
	}
	if c.Interval < 0 {
		return fmt.Errorf("interval must be positive, got %s", c.Interval)
	}
	if c.Timeout == 0 {
		c.Timeout = DefaultTimeout
	}
	if c.Timeout < 0 {
		return fmt.Errorf("timeout must be positive, got %s", c.Timeout)
	}
	if c.InitialDelay != nil && *c.InitialDelay < 0 {
		return fmt.Errorf("initialDelay must be positive, got %s", *c.InitialDelay)
	}
	if c.MaxConn <= 0 {
		c.MaxConn = DefaultMaxConn
	}
	if c.Conditions == nil {
		c.Conditions = &Success{Code: defaultSuccessCodes}
	} else if len(c.Conditions.Code) == 0 {
		c.Conditions.Code = defaultSuccessCodes
	}
	if c.SuccessThreshold == 0 {
		c.SuccessThreshold = DefaultSuccessThreshold
	}
	if c.FailureThreshold == 0 {
		c.FailureThreshold = DefaultFailureThreshold
	}

	// DEPRECATED: migrate Alerts to Notifications
	if c.Alerts != nil {
		log.Print("[WARN] 'alerts' field is deprecated, please use 'notifications' instead")
		c.Notifications = *c.Alerts
	}
	if c.Notifications.SMTP == nil && c.DefaultSMTP != nil {
		smtp := *c.DefaultSMTP
		c.Notifications.SMTP = &smtp
	}
	if c.Notifications.Cooldown == nil {
		d := DefaultCooldown
		c.Notifications.Cooldown = &d
	} else if *c.Notifications.Cooldown < 0 {
		return fmt.Errorf("notifications.cooldown must not be negative, got %s", *c.Notifications.Cooldown)
	}
	if err := c.Notifications.validate(); err != nil {
		return fmt.Errorf("notifications: %w", err)
	}

	hosts := make([]*Host, 0, len(c.FileHosts))
	for i, host := range c.FileHosts {
		if host.URL == "" {
			return errors.New("host cannot be without url")
		}
		if host.Interval != nil && *host.Interval <= 0 {
			return fmt.Errorf("host %s: interval must be positive, got %s", host.URL, *host.Interval)
		}
		if host.TimeoutInterval != nil && *host.TimeoutInterval <= 0 {
			return fmt.Errorf("host %s: timeout must be positive, got %s", host.URL, *host.TimeoutInterval)
		}
		if host.InitialDelay != nil && *host.InitialDelay < 0 {
			return fmt.Errorf("host %s: initialDelay must be positive, got %s", host.URL, *host.InitialDelay)
		}

		host.ID = host.GenerateID()
		host.Index = i
		host.Type = host.GetType()

		// copy the global values instead of pointing into the config, so a later
		// reload cannot change the value under a running watcher.
		if host.Interval == nil {
			d := c.Interval
			host.Interval = &d
		}
		if host.TimeoutInterval == nil {
			d := c.Timeout
			host.TimeoutInterval = &d
		}
		if host.InitialDelay == nil && c.InitialDelay != nil {
			d := *c.InitialDelay
			host.InitialDelay = &d
		}
		if host.Conditions == nil {
			host.Conditions = c.Conditions
		} else if len(host.Conditions.Code) == 0 {
			host.Conditions.Code = c.Conditions.Code
		}

		if host.SuccessThreshold == 0 {
			host.SuccessThreshold = c.SuccessThreshold
		}
		if host.FailureThreshold == 0 {
			host.FailureThreshold = c.FailureThreshold
		}

		if host.Headers == nil {
			host.Headers = make(map[string]string)
		}
		for key, value := range c.Headers {
			if _, ok := host.Headers[key]; !ok {
				host.Headers[key] = value
			}
		}

		for _, h := range hosts {
			if h.ID == host.ID {
				return fmt.Errorf("duplicate host: %s", host.String())
			}
		}
		hosts = append(hosts, host)

		msg := fmt.Sprintf("[DEBUG] id=%s", host.ID)
		if host.Name != nil {
			msg += fmt.Sprintf(", name=%s", *host.Name)
		}
		log.Printf("%s, url=%s, type=%s, initialDelay=%s, interval=%s, timeout=%s, successCode=%v, successThreshold=%d, failureThreshold=%d, hidden=%v",
			msg, host.SecureURL(), host.Type, host.InitialDelay, host.Interval, host.TimeoutInterval, host.Conditions.Code, host.SuccessThreshold, host.FailureThreshold, host.Hidden)
	}

	for _, old := range c.Hosts {
		found := false
		for _, h := range hosts {
			if h.ID == old.ID {
				found = true
				break
			}
		}
		if !found {
			log.Printf("[WARN] remove host id=%s: %s", old.ID, old.SecureURL())
		}
	}

	if len(hosts) == 0 {
		return errors.New("no hosts for monitoring")
	}
	c.Hosts = hosts

	return nil
}

// GetUI - returns a copy of the UI settings. Safe to call concurrently with a reload.
func (c *Cfg) GetUI() UI {
	c.mu.RLock()
	defer c.mu.RUnlock()

	ui := c.UI
	if ui.BasicAuth != nil {
		auth := *ui.BasicAuth
		ui.BasicAuth = &auth
	}
	return ui
}

func (c *Cfg) save() error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	return os.WriteFile(c.path, b, 0644)
}
