package notify

import (
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"strings"

	"github.com/exelban/EndPoll/types"
)

// Twilio - SMS via the Twilio Messages API
// (https://www.twilio.com/docs/messaging/api/message-resource#create-a-message-resource)
type Twilio struct {
	url        string // base url of the API, the account path is appended
	accountSID string
	authToken  string
	from       string
	to         []string

	client *http.Client
}

func (t *Twilio) string() string {
	return "twilio"
}

func (t *Twilio) send(_, subject, body string) error {
	if body == "" {
		body = subject
	}
	target := fmt.Sprintf("%s/2010-04-01/Accounts/%s/Messages.json", strings.TrimSuffix(t.url, "/"), t.accountSID)
	headers := map[string]string{
		"Authorization": "Basic " + base64.StdEncoding.EncodeToString([]byte(t.accountSID+":"+t.authToken)),
	}

	var errs []error
	for _, to := range t.to {
		values := url.Values{
			"To":   {to},
			"From": {t.from},
			"Body": {body},
		}
		if err := postForm(t.client, target, headers, values); err != nil {
			errs = append(errs, fmt.Errorf("%s: %w", to, err))
		}
	}
	return errors.Join(errs...)
}

// normalize - an SMS is short: the subject line plus the address
func (t *Twilio) normalize(host *types.Host, status types.StatusType) (string, string) {
	subject, _ := plainMessage(host, status)
	text := subject
	if host.Name != nil && *host.Name != "" {
		text += "\n" + host.SecureURL()
	}
	return subject, text
}
