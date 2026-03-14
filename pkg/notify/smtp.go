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

type SMTP struct {
	Host     string
	Port     int
	Username string
	Password string
	From     string
	To       []string

	dialer     *gomail.Dialer
	sendCloser gomail.SendCloser
	open       bool
	timer      *time.Timer
	last       *time.Time
	once       sync.Once

	sync.Mutex
}

// dial creates a new connection to the SMTP server.
// close closes the connection to the SMTP server.
func (s *SMTP) dial() error {
	sc, err := s.dialer.Dial()
	if err != nil {
		return fmt.Errorf("dialer dial: %w", err)
	}
	s.sendCloser = sc
	s.open = true

	if s.timer != nil {
		s.timer.Stop()
	}
	s.timer = time.AfterFunc(10*time.Second, s.close)

	return nil
}
func (s *SMTP) close() {
	if err := s.sendCloser.Close(); err != nil {
		log.Printf("[ERROR] smpt close: %v", err)
	}
	s.open = false
}

func (s *SMTP) string() string {
	return "smtp"
}

func (s *SMTP) send(subject, body string) error {
	s.once.Do(func() {
		s.dialer = gomail.NewDialer(s.Host, s.Port, s.Username, s.Password)
		s.dialer.TLSConfig = &tls.Config{
			InsecureSkipVerify: true,
		}
	})

	s.Lock()
	defer s.Unlock()

	if s.last != nil && time.Since(*s.last) < time.Second {
		wait := time.Second - time.Since(*s.last)
		s.Unlock()
		time.Sleep(wait)
		s.Lock()
	}

	if !s.open {
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
	message.SetBody("text/html", body)

	if err := gomail.Send(s.sendCloser, message); err != nil {
		return fmt.Errorf("send email: %w", err)
	}

	now := time.Now()
	s.last = &now

	return nil
}

func (s *SMTP) normalize(host *types.Host, status types.StatusType) (string, string) {
	icon := "❌"
	if status == types.UP {
		icon = "✅"
	}

	details := fmt.Sprintf(`
	<li><strong>Address:</strong> <a href="%s">%s</a></li>
	<li><strong>Last check time:</strong> %s</li>
	`, host.URL, host.URL, time.Now().Format(time.RFC1123))

	name := host.URL
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
