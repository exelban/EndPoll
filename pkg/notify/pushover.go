package notify

import (
	"net/http"
	"net/url"
	"strconv"

	"github.com/exelban/EndPoll/types"
)

// Pushover - push notifications via pushover.net (https://pushover.net/api)
type Pushover struct {
	url      string
	token    string
	user     string
	priority int

	client *http.Client
}

func (p *Pushover) string() string {
	return "pushover"
}

func (p *Pushover) send(_, subject, body string) error {
	if body == "" {
		body = subject
	}
	values := url.Values{
		"token":    {p.token},
		"user":     {p.user},
		"title":    {subject},
		"message":  {body},
		"priority": {strconv.Itoa(p.priority)},
	}
	if p.priority == 2 {
		// emergency priority requires the retry/expire parameters
		values.Set("retry", "60")
		values.Set("expire", "3600")
	}
	return postForm(p.client, p.url, nil, values)
}

func (p *Pushover) normalize(host *types.Host, status types.StatusType) (string, string) {
	subject, text := plainMessage(host, status)
	return subject, text
}
