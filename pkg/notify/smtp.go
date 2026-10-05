package notify

import (
	"crypto/tls"
	"fmt"
	"log"
	"strings"
	"sync"
	"time"

	"github.com/exelban/EndPoll/types"
	gomail "gopkg.in/mail.v2"
)

// smtpIdleTimeout - how long an SMTP connection is kept open after the last message
const smtpIdleTimeout = 10 * time.Second

// smtpMinInterval - minimal interval between two messages
const smtpMinInterval = time.Second

type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string

	InsecureSkipVerify bool

	dialer     *gomail.Dialer
	sendCloser gomail.SendCloser
	timer      *time.Timer
	last       time.Time

	mu sync.Mutex
}

func (s *SMTP) string() string {
	return "smtp"
}

// send - delivers the message reusing an open connection when possible.
// The whole operation is serialized: the connection, the idle timer and the
// rate limiter are all guarded by the same lock.
func (s *SMTP) send(id, subject, body string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.dialer == nil {
		s.dialer = gomail.NewDialer(s.Host, s.Port, s.Username, s.Password)
		s.dialer.TLSConfig = &tls.Config{
			ServerName:         s.Host,
			InsecureSkipVerify: s.InsecureSkipVerify, //nolint:gosec // opt-in via configuration
		}
	}

	if !s.last.IsZero() {
		if wait := smtpMinInterval - time.Since(s.last); wait > 0 {
			time.Sleep(wait)
		}
	}

	if s.sendCloser == nil {
		if err := s.dial(); err != nil {
			return err
		}
	}

	if subject == "" {
		subject = "Status page: event triggered"
	}

	message := gomail.NewMessage()
	message.SetHeader("From", s.From)
	message.SetHeader("To", s.To...)
	message.SetHeader("Subject", subject)
	if mid := s.messageID(id); mid != "" {
		message.SetHeader("Message-ID", mid)
	}
	message.SetBody("text/html", body)

	if err := gomail.Send(s.sendCloser, message); err != nil {
		// the connection is most likely broken: drop it, the next send will reconnect
		s.closeLocked()
		return fmt.Errorf("send email: %w", err)
	}

	s.last = time.Now()
	s.timer.Reset(smtpIdleTimeout)

	return nil
}

// messageID - builds the Message-ID header value "<id@domain>" for the event id.
func (s *SMTP) messageID(id string) string {
	if id == "" {
		return ""
	}
	domain := "endpoll.local"
	if at := strings.LastIndex(s.From, "@"); at >= 0 && at < len(s.From)-1 {
		domain = strings.TrimSuffix(s.From[at+1:], ">")
	}
	return fmt.Sprintf("<%s@%s>", id, domain)
}

// dial - opens a new connection to the SMTP server. Must be called with the lock held.
func (s *SMTP) dial() error {
	sc, err := s.dialer.Dial()
	if err != nil {
		return fmt.Errorf("dialer dial: %w", err)
	}
	s.sendCloser = sc

	if s.timer == nil {
		s.timer = time.AfterFunc(smtpIdleTimeout, s.idleClose)
	} else {
		s.timer.Reset(smtpIdleTimeout)
	}

	return nil
}

// idleClose - closes the connection after the idle timeout
func (s *SMTP) idleClose() {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.closeLocked()
}

// closeLocked - closes the connection. Must be called with the lock held.
func (s *SMTP) closeLocked() {
	if s.sendCloser == nil {
		return
	}
	if err := s.sendCloser.Close(); err != nil {
		log.Printf("[ERROR] smtp close: %v", err)
	}
	s.sendCloser = nil
}

func (s *SMTP) normalize(host *types.Host, status types.StatusType) (string, string) {
	icon := "❌"
	if status == types.UP {
		icon = "✅"
	}

	url := host.SecureURL()
	details := fmt.Sprintf(`
	<li><strong>Address:</strong> <a href="%s">%s</a></li>
	<li><strong>Last check time:</strong> %s</li>
	`, url, url, time.Now().Format(time.RFC1123))

	name := url
	if host.Name != nil && *host.Name != "" {
		name = *host.Name
		details = fmt.Sprintf("<li><strong>Name:</strong> %s</li>%s", name, details)
	}

	subject := fmt.Sprintf("%s %s is %s", icon, name, strings.ToUpper(string(status)))

	text := fmt.Sprintf(`
<h2>%s %s has a new status: %s</h2>

<h3>Details:</h3>
<ul>%s</ul>

<p>Check the status page for more details.</p>
`, icon, name, strings.ToUpper(string(status)), details)

	return subject, text
}
