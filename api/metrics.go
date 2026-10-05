package api

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/exelban/EndPoll/types"
)

// metrics - GET /metrics: the state of the hosts in the Prometheus exposition format.
// The format is simple enough to write by hand, no client library is needed.
func (s *Rest) metrics(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	var b strings.Builder

	fmt.Fprintf(&b, "# HELP endpoll_build_info Build information.\n# TYPE endpoll_build_info gauge\nendpoll_build_info{version=%q} 1\n", s.Version)

	snapshots := s.Monitor.Snapshots()

	b.WriteString("# HELP endpoll_host_up 1 if the host is up, 0 if it is down or degraded. Absent while the status is unknown.\n# TYPE endpoll_host_up gauge\n")
	for _, snap := range snapshots {
		switch snap.Status {
		case types.UP:
			fmt.Fprintf(&b, "endpoll_host_up%s 1\n", labels(snap.Host))
		case types.DOWN, types.DEGRADED:
			fmt.Fprintf(&b, "endpoll_host_up%s 0\n", labels(snap.Host))
		}
	}

	b.WriteString("# HELP endpoll_host_status 1 for the current status of the host.\n# TYPE endpoll_host_status gauge\n")
	for _, snap := range snapshots {
		for _, st := range []types.StatusType{types.UP, types.DEGRADED, types.DOWN, types.Unknown} {
			v := 0
			if snap.Status == st {
				v = 1
			}
			fmt.Fprintf(&b, "endpoll_host_status%s %d\n", labels(snap.Host, "status", string(st)), v)
		}
	}

	b.WriteString("# HELP endpoll_host_response_seconds Duration of the last check.\n# TYPE endpoll_host_response_seconds gauge\n")
	for _, snap := range snapshots {
		if snap.LastResponse != nil {
			fmt.Fprintf(&b, "endpoll_host_response_seconds%s %g\n", labels(snap.Host), snap.LastResponse.Time.Seconds())
		}
	}

	b.WriteString("# HELP endpoll_host_response_code Status code of the last check (http code or 521/522/523 pseudo codes).\n# TYPE endpoll_host_response_code gauge\n")
	for _, snap := range snapshots {
		if snap.LastResponse != nil {
			fmt.Fprintf(&b, "endpoll_host_response_code%s %d\n", labels(snap.Host), snap.LastResponse.Code)
		}
	}

	b.WriteString("# HELP endpoll_host_last_check_timestamp_seconds Unix time of the last check.\n# TYPE endpoll_host_last_check_timestamp_seconds gauge\n")
	for _, snap := range snapshots {
		if !snap.LastCheck.IsZero() {
			fmt.Fprintf(&b, "endpoll_host_last_check_timestamp_seconds%s %d\n", labels(snap.Host), snap.LastCheck.Unix())
		}
	}

	b.WriteString("# HELP endpoll_host_ssl_expiry_timestamp_seconds Unix time of the TLS certificate expiration.\n# TYPE endpoll_host_ssl_expiry_timestamp_seconds gauge\n")
	for _, snap := range snapshots {
		if snap.LastResponse != nil && snap.LastResponse.SSLCertExpiry != nil {
			fmt.Fprintf(&b, "endpoll_host_ssl_expiry_timestamp_seconds%s %d\n", labels(snap.Host), snap.LastResponse.SSLCertExpiry.Unix())
		}
	}

	b.WriteString("# HELP endpoll_host_uptime_ratio Uptime of the last 90 days.\n# TYPE endpoll_host_uptime_ratio gauge\n")
	for _, snap := range snapshots {
		if stats, err := s.Monitor.StatsByID(ctx, snap.Host.ID, true); err == nil && len(stats.Hosts) > 0 {
			fmt.Fprintf(&b, "endpoll_host_uptime_ratio%s %g\n", labels(snap.Host), float64(stats.Hosts[0].Uptime)/100)
		}
	}

	w.Header().Set("Content-Type", "text/plain; version=0.0.4; charset=utf-8")
	_, _ = w.Write([]byte(b.String()))
}

// labels - the label set of a host, with optional extra label pairs
func labels(h *types.Host, extra ...string) string {
	pairs := []string{
		fmt.Sprintf("id=%q", h.ID),
		fmt.Sprintf("name=%q", escapeLabel(hostLabelName(h))),
		fmt.Sprintf("group=%q", escapeLabel(deref(h.Group))),
		fmt.Sprintf("type=%q", h.Type),
	}
	for i := 0; i+1 < len(extra); i += 2 {
		pairs = append(pairs, fmt.Sprintf("%s=%q", extra[i], escapeLabel(extra[i+1])))
	}
	return "{" + strings.Join(pairs, ",") + "}"
}

func hostLabelName(h *types.Host) string {
	if h.Name != nil && *h.Name != "" {
		return *h.Name
	}
	return h.SecureURL()
}

func deref(s *string) string {
	if s == nil {
		return ""
	}
	return *s
}

// escapeLabel - %q already escapes quotes, backslashes and newlines as required
// by the exposition format; other control characters are dropped.
func escapeLabel(s string) string {
	return strings.Map(func(r rune) rune {
		if r < 0x20 && r != '\n' {
			return -1
		}
		return r
	}, s)
}
