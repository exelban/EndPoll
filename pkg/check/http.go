package check

import (
	"context"
	"crypto/tls"
	"io"
	"log"
	"net"
	"net/http"
	"net/http/httptrace"
	"sync"
	"time"

	"github.com/exelban/EndPoll/types"
)

// maxBody - maximum number of bytes read from the response body
const maxBody = 1 << 20

// HTTP - performs HTTP checks using a shared transport.
type HTTP struct {
	transport *http.Transport
}

// NewHTTP - creates an HTTP checker.
func NewHTTP() *HTTP {
	return &HTTP{
		transport: &http.Transport{
			Proxy: http.ProxyFromEnvironment,
			DialContext: (&net.Dialer{
				Timeout: 30 * time.Second,
			}).DialContext,
			ForceAttemptHTTP2:   true,
			DisableKeepAlives:   true,
			TLSHandshakeTimeout: 30 * time.Second,
		},
	}
}

// Close - releases the resources held by the HTTP checker.
func (c *HTTP) Close() {
	if c.transport != nil {
		c.transport.CloseIdleConnections()
	}
}

// Check - makes an HTTP request to the host.
func (c *HTTP) Check(ctx context.Context, h *types.Host) (response types.HttpResponse) {
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
		Transport: c.transport,
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

	b, err := io.ReadAll(io.LimitReader(resp.Body, maxBody+1))
	if err != nil {
		log.Printf("[ERROR] read body %v", err)
		response.Code = errorCode(err)
		response.Body = truncate(err.Error(), 512)
		return
	}
	if h.Conditions != nil && h.Conditions.Body != nil {
		if len(b) > maxBody {
			response.Code = http.StatusBadGateway
			response.Body = "response body exceeds limit"
			return
		}
		response.Bytes = b
	} else if len(b) < 1024 {
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
