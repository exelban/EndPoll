package notify

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"

	"github.com/exelban/EndPoll/types"
)

// httpTimeout - default timeout of a request to a notification provider
const httpTimeout = 10 * time.Second

// request - executes the request and fails on a non-2xx response
func request(client *http.Client, req *http.Request) error {
	if client == nil {
		client = &http.Client{Timeout: httpTimeout}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer func() {
		_ = resp.Body.Close()
	}()

	if resp.StatusCode >= 200 && resp.StatusCode < 300 {
		return nil
	}
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
	return fmt.Errorf("non-ok (%d) response: %s", resp.StatusCode, strings.TrimSpace(string(b)))
}

// postJSON - sends the payload as a JSON document
func postJSON(client *http.Client, method, target string, headers map[string]string, payload any) error {
	b, err := json.Marshal(payload)
	if err != nil {
		return fmt.Errorf("marshal payload: %w", err)
	}
	if method == "" {
		method = http.MethodPost
	}

	req, err := http.NewRequest(method, target, bytes.NewReader(b))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json; charset=utf-8")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return request(client, req)
}

// postForm - sends the values as an url-encoded form
func postForm(client *http.Client, target string, headers map[string]string, values url.Values) error {
	req, err := http.NewRequest(http.MethodPost, target, strings.NewReader(values.Encode()))
	if err != nil {
		return fmt.Errorf("create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	for k, v := range headers {
		req.Header.Set(k, v)
	}

	return request(client, req)
}

// statusIcon - the icon of the status
func statusIcon(status types.StatusType) string {
	if status == types.UP {
		return "✅"
	}
	return "❌"
}

// hostName - the display name of the host: the name if available, otherwise the url
func hostName(host *types.Host) string {
	if host.Name != nil && *host.Name != "" {
		return *host.Name
	}
	return host.SecureURL()
}

// plainMessage - the subject and the plain text of a status notification,
// shared by the providers that have no rich formatting.
func plainMessage(host *types.Host, status types.StatusType) (subject, text string) {
	icon := statusIcon(status)
	s := strings.ToUpper(string(status))

	subject = fmt.Sprintf("%s %s is %s", icon, hostName(host), s)
	text = fmt.Sprintf("%s %s has a new status: %s", icon, host.String(), s)

	return subject, text
}
