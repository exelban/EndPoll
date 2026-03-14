package types

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"gopkg.in/yaml.v2"
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
}

type Notifications struct {
	Slack    *Slack    `json:"slack" yaml:"slack"`
	Telegram *Telegram `json:"telegram" yaml:"telegram"`
	SMTP     *SMTP     `json:"smtp" yaml:"smtp"`

	InitializationMessage *bool `json:"initializationMessage" yaml:"initializationMessage"`
	ShutdownMessage       bool  `json:"shutdownMessage" yaml:"shutdownMessage"`
}

type UI struct {
	Title   string `json:"title" yaml:"title"`     // web page title
	HideURL bool   `json:"hideURL" yaml:"hideURL"` // allows to hide URL of the host in the UI
}

type Cfg struct {
	MaxConn int `json:"maxConn" yaml:"maxConn,omitempty"`

	Interval     time.Duration  `json:"interval,omitempty" yaml:"interval,omitempty"`
	Timeout      time.Duration  `json:"timeout,omitempty" yaml:"timeout,omitempty"`
	InitialDelay *time.Duration `json:"initialDelay,omitempty" yaml:"initialDelay,omitempty"`

	SuccessThreshold int `json:"successThreshold" yaml:"successThreshold,omitempty"`
	FailureThreshold int `json:"failureThreshold" yaml:"failureThreshold,omitempty"`

	Conditions *Success          `json:"success" yaml:"success,omitempty"`
	Headers    map[string]string `json:"headers" yaml:"headers,omitempty"`

	UI            UI            `json:"ui" yaml:"ui"`
	Notifications Notifications `json:"notifications" yaml:"notifications,omitempty"`
	FileHosts     []*Host       `json:"hosts" yaml:"hosts"`
	Hosts         []*Host       `json:"-" yaml:"-"`

	path        string    `yaml:"-"`
	initialized bool      `yaml:"-"`
	FW          chan bool `yaml:"-"`

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
		modTimestamp := time.Time{}
		for {
			select {
			case <-ticker.C:
				fi, err := os.Stat(path)
				if err != nil {
					continue
				}
				if fi.ModTime() != modTimestamp {
					log.Printf("[DEBUG] config changed: %s -> %s",
						modTimestamp.Format(time.RFC3339Nano), fi.ModTime().Format(time.RFC3339Nano))
					cfg.FW <- true
					modTimestamp = fi.ModTime()
				}
			case <-ctx.Done():
				ticker.Stop()
				return
			}
		}
	}()

	return cfg, nil
}

// Parse - open a configuration file and parse it into Config based on file type (json or yaml)
func (c *Cfg) Parse() error {
	file, err := os.Open(c.path)
	if err != nil {
		return err
	}
	defer func() {
		_ = file.Close()
	}()

	bytes, err := io.ReadAll(file)
	if err != nil {
		return err
	}

	if strings.HasSuffix(c.path, ".yaml") || strings.HasSuffix(c.path, ".yml") {
		if c.initialized {
			log.Print("[DEBUG] detect yaml config")
		} else {
			c.initialized = true
		}
		return yaml.Unmarshal(bytes, &c)
	} else if strings.HasSuffix(c.path, ".json") {
		if c.initialized {
			log.Print("[DEBUG] detect json config")
		} else {
			c.initialized = true
		}
		return json.Unmarshal(bytes, &c)
	}

	return fmt.Errorf("unknown configuration format `%s`", c.path)
}

// Validate - trying to guess if host is API or Server, also set default timeout and retry values
func (c *Cfg) Validate() error {
	if c.MaxConn == 0 {
		c.MaxConn = 128
	}
	if c.Interval == time.Duration(0) {
		c.Interval = 30 * time.Second
	}
	if c.Timeout == time.Duration(0) {
		c.Timeout = 60 * time.Second
	}
	if c.Conditions == nil {
		c.Conditions = &Success{
			Code: []int{200, 201, 202, 203, 204, 205, 206, 207, 208},
		}
	} else if len(c.Conditions.Code) == 0 {
		c.Conditions.Code = []int{200, 201, 202, 203, 204, 205, 206, 207, 208}
	}
	if c.SuccessThreshold == 0 {
		c.SuccessThreshold = 1
	}
	if c.FailureThreshold == 0 {
		c.FailureThreshold = 2
	}

	// DEPRECATED: migrate Alerts to Notifications
	if c.Alerts != nil {
		log.Print("[WARN] 'alerts' field is deprecated, please use 'notifications' instead")
		c.Notifications = *c.Alerts
	}

	for i, host := range c.FileHosts {
		if host.URL == "" {
			return errors.New("host cannot be without url")
		}

		host.ID = host.GenerateID()

		idx := -1
		for j, h := range c.Hosts {
			if h.ID == host.ID {
				idx = j
				break
			}
		}

		host.Index = i
		host.Type = host.GetType()

		if host.Interval == nil {
			host.Interval = &c.Interval
		}
		if host.TimeoutInterval == nil {
			host.TimeoutInterval = &c.Timeout
		}
		if host.InitialDelay == nil {
			host.InitialDelay = c.InitialDelay
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

		if idx == -1 {
			c.addHost(host)
		} else {
			c.updateHost(idx, host)
			i = idx
		}

		msg := fmt.Sprintf("[DEBUG] id=%s", c.Hosts[i].ID)
		if c.Hosts[i].Name != nil {
			msg += fmt.Sprintf(", name=%s", *c.Hosts[i].Name)
		}
		log.Printf("%s, url=%s, type=%s, initialDelay=%s, interval=%s, timeout=%s, successCode=%v, successThreshold=%d, failureThreshold=%d, hidden=%v",
			msg, c.Hosts[i].URL, c.Hosts[i].Type, c.Hosts[i].InitialDelay, c.Hosts[i].Interval, c.Hosts[i].TimeoutInterval, c.Hosts[i].Conditions.Code, c.Hosts[i].SuccessThreshold, c.Hosts[i].FailureThreshold, c.Hosts[i].Hidden)
	}

	// remove hosts that are not in the config file
	for i := len(c.Hosts) - 1; i >= 0; i-- {
		found := false
		for _, host := range c.FileHosts {
			if c.Hosts[i].ID == host.GenerateID() {
				found = true
				break
			}
		}
		if !found {
			log.Printf("[WARN] remove host id=%s: %s", c.Hosts[i].ID, c.Hosts[i].URL)
			c.Hosts = append(c.Hosts[:i], c.Hosts[i+1:]...)
		}
	}

	if len(c.Hosts) == 0 {
		return errors.New("no hosts for monitoring")
	}

	return nil
}

func (c *Cfg) save() error {
	b, err := yaml.Marshal(c)
	if err != nil {
		return err
	}
	if err := os.WriteFile(c.path, b, 0644); err != nil {
		return err
	}
	return nil
}

func (c *Cfg) addHost(host *Host) {
	c.Hosts = append(c.Hosts, host)
}
func (c *Cfg) updateHost(at int, host *Host) {
	c.Hosts[at].Index = host.Index
	c.Hosts[at].Type = host.Type

	c.Hosts[at].Name = host.Name
	c.Hosts[at].Description = host.Description
	c.Hosts[at].Group = host.Group

	c.Hosts[at].Method = host.Method

	c.Hosts[at].Interval = host.Interval
	c.Hosts[at].InitialDelay = host.InitialDelay
	c.Hosts[at].TimeoutInterval = host.TimeoutInterval

	c.Hosts[at].SuccessThreshold = host.SuccessThreshold
	c.Hosts[at].FailureThreshold = host.FailureThreshold

	c.Hosts[at].Conditions = host.Conditions
	c.Hosts[at].Headers = host.Headers

	c.Hosts[at].Alerts = host.Alerts

	c.Hosts[at].Hidden = host.Hidden
}
