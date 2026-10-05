package types

import (
	"bytes"
	"crypto/md5"
	"encoding/base64"
	"errors"
	"fmt"
	"net/http"
	"net/url"
	"reflect"
	"strconv"
	"strings"
	"time"
)

type Success struct {
	Code []int   `json:"code" yaml:"code"`
	Body *string `json:"body" yaml:"body"`
}

// DNSCheck - options of a dns host (url: dns://name)
type DNSCheck struct {
	Type     string   `json:"type,omitempty" yaml:"type,omitempty"`         // A, AAAA, CNAME, MX, NS, TXT (default: A)
	Expect   []string `json:"expect,omitempty" yaml:"expect,omitempty"`     // values that must be present in the answer (optional)
	Resolver string   `json:"resolver,omitempty" yaml:"resolver,omitempty"` // host:port of the dns server (default: system resolver)
}

// Host - host structure
type Host struct {
	ID   string   `json:"id" yaml:"-"`
	Type HostType `json:"type" yaml:"type"`

	Name        *string `json:"name,omitempty" yaml:"name,omitempty"`
	Description *string `json:"description,omitempty" yaml:"description,omitempty"`
	Group       *string `json:"group,omitempty" yaml:"group,omitempty"`

	Method string `json:"method,omitempty" yaml:"method,omitempty"`
	URL    string `json:"url" yaml:"url"`

	Interval        *time.Duration `json:"interval" yaml:"interval,omitempty"` // minimum 1s
	TimeoutInterval *time.Duration `json:"timeout" yaml:"timeout,omitempty"`
	InitialDelay    *time.Duration `json:"initialDelay" yaml:"initialDelay,omitempty"`

	SuccessThreshold int `json:"successThreshold,omitempty" yaml:"successThreshold,omitempty"`
	FailureThreshold int `json:"failureThreshold,omitempty" yaml:"failureThreshold,omitempty"`

	Conditions *Success          `json:"conditions,omitempty" yaml:"conditions,omitempty"`
	Headers    map[string]string `json:"headers,omitempty" yaml:"headers,omitempty"`

	Alerts []string `json:"alerts,omitempty" yaml:"alerts,omitempty"`

	DNS *DNSCheck `json:"dns,omitempty" yaml:"dns,omitempty"` // dns check options

	Hidden bool `json:"hidden" yaml:"hidden"` // acceptable only if group is defined

	Index int `json:"-" yaml:"-"`
}

var ErrHostNotFound = errors.New("host not found")

// GenerateID - returns a host id based on the url hash
func (h *Host) GenerateID() string {
	hasher := md5.New()
	input := []byte(h.URL)
	if h.Group != nil {
		input = append(input, []byte(*h.Group)...)
	}
	hasher.Write(input)
	hash := hasher.Sum(nil)
	return base64.URLEncoding.EncodeToString(hash)[:6]
}

// Status - checking if provided code present in the success code list and body is equal
func (h *Host) Status(code int, b []byte) bool {
	ok := false
	if h.Conditions == nil {
		h.Conditions = &Success{
			Code: []int{http.StatusOK},
		}
	}

	for _, v := range h.Conditions.Code {
		if v == code {
			ok = true
		}
	}

	if ok && h.Conditions.Body != nil {
		ok = bytes.Compare([]byte(*h.Conditions.Body), b) == 0
	}

	return ok
}

// String - returns a name if available, otherwise returns the url.
// Credentials in the url are masked: the value is used in logs and notifications.
func (h *Host) String() string {
	if h.Name == nil {
		return h.SecureURL()
	}
	return fmt.Sprintf("%s (%s)", *h.Name, h.SecureURL())
}

// GetType - return a host type based on url
func (h *Host) GetType() HostType {
	if h.Type != "" {
		return h.Type
	}

	scheme := ""
	if i := strings.Index(h.URL, "://"); i > 0 {
		scheme = strings.ToLower(h.URL[:i])
	}
	switch scheme {
	case "mongodb", "mongodb+srv":
		return MongoType
	case "tcp":
		return TCPType
	case "dns":
		return DNSType
	case "redis", "rediss":
		return RedisType
	case "postgres", "postgresql":
		return PostgresType
	case "mysql":
		return MySQLType
	case "http", "https":
		return HttpType
	}
	if isIPv4(h.URL) {
		return ICMPType
	}

	return HttpType
}

// SecureURL - returns a secure url that can be used in logs or alerts. It will hide the password if present.
func (h *Host) SecureURL() string {
	u, err := url.Parse(h.URL)
	if err != nil || u.User == nil {
		return h.URL
	}
	if _, ok := u.User.Password(); !ok {
		return h.URL
	}
	u.User = url.UserPassword(u.User.Username(), "*****")
	return strings.ReplaceAll(u.String(), "%2A%2A%2A%2A%2A", "*****")
}

// Changed - reports whether the monitoring-relevant configuration of the host differs
// from the other one. The position in the list (Index) is ignored.
func (h *Host) Changed(other *Host) bool {
	if other == nil {
		return true
	}
	a, b := *h, *other
	a.Index, b.Index = 0, 0
	return !reflect.DeepEqual(a, b)
}

func isIPv4(host string) bool {
	parts := strings.Split(host, ".")

	if len(parts) != 4 {
		return false
	}

	for _, x := range parts {
		i, err := strconv.Atoi(x)
		if err != nil {
			return false
		}
		if i < 0 || i > 255 {
			return false
		}
	}
	return true
}
