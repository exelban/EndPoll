package dialer

import (
	"context"
	"errors"
	"fmt"
	"net"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/exelban/EndPoll/types"
)

// dnsCall - resolves the name (url: dns://name) and checks the expected records
func (d *Dialer) dnsCall(ctx context.Context, h *types.Host) (response types.HttpResponse) {
	response.Timestamp = time.Now()

	name := strings.TrimSuffix(strings.TrimPrefix(h.URL, "dns://"), ".")
	if name == "" {
		response.Code = http.StatusBadRequest
		response.Body = "invalid address, expected dns://name"
		return
	}

	recordType := "A"
	var expect []string
	resolver := net.DefaultResolver
	if h.DNS != nil {
		if h.DNS.Type != "" {
			recordType = strings.ToUpper(h.DNS.Type)
		}
		expect = h.DNS.Expect
		if h.DNS.Resolver != "" {
			server := h.DNS.Resolver
			if _, _, err := net.SplitHostPort(server); err != nil {
				server = net.JoinHostPort(server, "53")
			}
			resolver = &net.Resolver{
				PreferGo: true,
				Dial: func(ctx context.Context, network, _ string) (net.Conn, error) {
					return (&net.Dialer{}).DialContext(ctx, network, server)
				},
			}
		}
	}

	ctx, cancel := context.WithTimeout(ctx, timeout(h))
	defer cancel()

	start := time.Now()
	records, err := lookup(ctx, resolver, recordType, name)
	response.Time = time.Since(start)
	if err != nil {
		response.Code = errorCode(err)
		var dnsErr *net.DNSError
		if errors.As(err, &dnsErr) && dnsErr.IsNotFound {
			response.Code = http.StatusNotFound
		}
		response.Body = truncate(err.Error(), 512)
		return
	}
	if len(records) == 0 {
		response.Code = http.StatusNotFound
		response.Body = fmt.Sprintf("no %s records", recordType)
		return
	}

	sort.Strings(records)
	response.Body = strings.Join(records, ", ")

	for _, e := range expect {
		found := false
		for _, r := range records {
			if strings.EqualFold(strings.TrimSuffix(r, "."), strings.TrimSuffix(e, ".")) {
				found = true
				break
			}
		}
		if !found {
			response.Code = http.StatusExpectationFailed
			response.Body = fmt.Sprintf("expected %s record %q not found, got: %s", recordType, e, strings.Join(records, ", "))
			return
		}
	}

	response.OK = true
	response.Code = http.StatusOK
	return
}

// lookup - resolves the records of the given type
func lookup(ctx context.Context, r *net.Resolver, recordType, name string) ([]string, error) {
	switch recordType {
	case "A", "AAAA":
		network := "ip4"
		if recordType == "AAAA" {
			network = "ip6"
		}
		ips, err := r.LookupIP(ctx, network, name)
		if err != nil {
			return nil, err
		}
		res := make([]string, 0, len(ips))
		for _, ip := range ips {
			res = append(res, ip.String())
		}
		return res, nil
	case "CNAME":
		cname, err := r.LookupCNAME(ctx, name)
		if err != nil {
			return nil, err
		}
		return []string{cname}, nil
	case "MX":
		mxs, err := r.LookupMX(ctx, name)
		if err != nil {
			return nil, err
		}
		res := make([]string, 0, len(mxs))
		for _, mx := range mxs {
			res = append(res, mx.Host)
		}
		return res, nil
	case "NS":
		nss, err := r.LookupNS(ctx, name)
		if err != nil {
			return nil, err
		}
		res := make([]string, 0, len(nss))
		for _, ns := range nss {
			res = append(res, ns.Host)
		}
		return res, nil
	case "TXT":
		return r.LookupTXT(ctx, name)
	default:
		return nil, fmt.Errorf("unsupported record type %q (A, AAAA, CNAME, MX, NS, TXT)", recordType)
	}
}
