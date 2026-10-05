package dialer

import (
	"context"
	"crypto/tls"
	"errors"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"strings"
	"sync"
	"time"

	"github.com/exelban/EndPoll/types"
)

// maxBody - maximum number of bytes read from the response body
const maxBody = 1 << 20

// httpCall makes an HTTP request to the host
func (d *Dialer) httpCall(ctx context.Context, h *types.Host) (response types.HttpResponse) {
	method := h.Method
	if method == "" {
		method = http.MethodGet
	}
	response.Timestamp = time.Now()

	req, err := http.NewRequestWithContext(ctx, method, h.URL, nil)
	if err != nil {
		log.Printf("[ERROR] prepare request %v", err)
		response.Body = err.Error()
		return
	}

	var tr trace
	req = req.WithContext(httptrace.WithClientTrace(req.Context(), &httptrace.ClientTrace{
		DNSStart:             func(httptrace.DNSStartInfo) { tr.begin(&tr.dnsStart) },
		DNSDone:              func(httptrace.DNSDoneInfo) { tr.end(&tr.dnsStart, &tr.dns) },
		TLSHandshakeStart:    func() { tr.begin(&tr.tlsStart) },
		TLSHandshakeDone:     func(tls.ConnectionState, error) { tr.end(&tr.tlsStart, &tr.tls) },
		ConnectStart:         func(string, string) { tr.begin(&tr.connectStart) },
		ConnectDone:          func(string, string, error) { tr.end(&tr.connectStart, &tr.connect) },
		GotFirstResponseByte: func() { tr.end(&tr.start, &tr.ttfb) },
	}))

	for key, value := range h.Headers {
		req.Header.Set(key, value)
	}

	client := http.Client{
		Transport: d.transport,
		Timeout:   timeout(h),
	}

	tr.begin(&tr.start)
	resp, err := client.Do(req)
	response.Time = time.Since(tr.start)
	response.DNS, response.Connect, response.TLSHandshake, response.TTFB = tr.timings()
	if err != nil {
		response.Code = errorCode(err)
		response.Body = truncate(err.Error(), 512)
		return
	}
	defer func() {
		_ = resp.Body.Close()
	}()
	response.Code = resp.StatusCode

	if resp.TLS != nil && len(resp.TLS.PeerCertificates) > 0 {
		response.SSLCertExpiry = &resp.TLS.PeerCertificates[0].NotAfter
		issuer := resp.TLS.PeerCertificates[0].Issuer
		if len(issuer.Organization) > 0 {
			response.SSLIssuer = issuer.Organization[0]
		} else {
			response.SSLIssuer = issuer.CommonName
		}
		response.TLSVersion = tls.VersionName(resp.TLS.Version)
	}

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody))
	if err != nil {
		log.Printf("[ERROR] read body %v", err)
		response.Body = truncate(err.Error(), 512)
		return
	}
	if len(b) < 1024 {
		response.Bytes = b
	}
	response.OK = true

	return
}

type trace struct {
	mu sync.Mutex

	start, dnsStart, connectStart, tlsStart time.Time
	dns, connect, tls, ttfb                 time.Duration
}

func (t *trace) begin(ts *time.Time) {
	t.mu.Lock()
	defer t.mu.Unlock()
	*ts = time.Now()
}
func (t *trace) end(ts *time.Time, d *time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	if !ts.IsZero() {
		*d = time.Since(*ts)
	}
}
func (t *trace) timings() (dns, connect, tls, ttfb time.Duration) {
	t.mu.Lock()
	defer t.mu.Unlock()
	return t.dns, t.connect, t.tls, t.ttfb
}

// errorCode - maps a transport error to the pseudo status codes used by the UI:
// 522 - timeout, 523 - origin unreachable (dial failed), 521 - connection dropped.
func errorCode(err error) int {
	var netErr net.Error
	if errors.Is(err, context.DeadlineExceeded) || (errors.As(err, &netErr) && netErr.Timeout()) {
		return 522
	}

	var opErr *net.OpError
	if errors.As(err, &opErr) {
		switch opErr.Op {
		case "dial":
			return 523
		case "read":
			return 521
		}
	}

	msg := err.Error()
	if strings.Contains(msg, "refused") || strings.Contains(msg, "unreachable") || strings.Contains(msg, "no such host") {
		return 523
	}
	return http.StatusServiceUnavailable
}
