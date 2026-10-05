package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/exelban/EndPoll/pkg/monitor"
	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
	"github.com/stretchr/testify/require"
)

func apiServer(t *testing.T) (*httptest.Server, *types.Host, func()) {
	t.Helper()
	target := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
	}))

	ctx, cancel := context.WithCancel(context.Background())
	interval := 20 * time.Millisecond
	timeout := time.Second
	name := "Target"
	group := "Group"
	host := &types.Host{
		URL: target.URL, Name: &name, Group: &group,
		Interval: &interval, TimeoutInterval: &timeout,
		SuccessThreshold: 1, FailureThreshold: 1,
		Conditions: &types.Success{Code: []int{200}},
	}
	host.ID = host.GenerateID()
	host.Type = host.GetType()

	m := &monitor.Monitor{Store: store.NewMemory(ctx), CacheTTL: time.Second}
	require.NoError(t, m.Run(ctx, &types.Cfg{Hosts: []*types.Host{host}}))
	time.Sleep(80 * time.Millisecond)

	rest := &Rest{Monitor: m, Version: "test"}
	srv := httptest.NewServer(rest.Router())

	return srv, host, func() {
		srv.Close()
		m.Stop()
		cancel()
		target.Close()
	}
}

func getJSON(t *testing.T, url string, v any) int {
	t.Helper()
	resp, err := http.Get(url)
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Contains(t, resp.Header.Get("Content-Type"), "application/json")
	require.NoError(t, json.NewDecoder(resp.Body).Decode(v))
	return resp.StatusCode
}

func TestAPI_Hosts(t *testing.T) {
	srv, host, shutdown := apiServer(t)
	defer shutdown()

	var res struct {
		Status string     `json:"status"`
		Hosts  []hostJSON `json:"hosts"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv.URL+"/api/hosts", &res))
	require.Equal(t, "up", res.Status)
	require.Len(t, res.Hosts, 1)
	h := res.Hosts[0]
	require.Equal(t, host.ID, h.ID)
	require.Equal(t, "Target", *h.Name)
	require.Equal(t, "Group", *h.Group)
	require.Equal(t, types.HttpType, h.Type)
	require.Equal(t, types.UP, h.Status)
	require.Equal(t, 100, h.Uptime)
	require.NotNil(t, h.LastCheck)
	require.NotNil(t, h.Last)
	require.Equal(t, 200, h.Last.Code)
}

func TestAPI_Host(t *testing.T) {
	srv, host, shutdown := apiServer(t)
	defer shutdown()

	var res hostDetailJSON
	require.Equal(t, http.StatusOK, getJSON(t, srv.URL+"/api/hosts/"+host.ID, &res))
	require.Equal(t, host.ID, res.ID)
	require.NotNil(t, res.Details)
	require.NotEmpty(t, res.History)
	require.Equal(t, types.UP, res.History[len(res.History)-1].Status)
	require.NotNil(t, res.Incidents)

	var e map[string]string
	require.Equal(t, http.StatusNotFound, getJSON(t, srv.URL+"/api/hosts/nope", &e))
	require.Equal(t, "host not found", e["error"])

	var points []map[string]any
	require.Equal(t, http.StatusOK, getJSON(t, srv.URL+"/api/hosts/"+host.ID+"/response-time", &points))
	require.Len(t, points, 1)
	require.Equal(t, http.StatusNotFound, getJSON(t, srv.URL+"/api/hosts/nope/response-time", &e))
}

func TestAPI_Incidents(t *testing.T) {
	srv, _, shutdown := apiServer(t)
	defer shutdown()

	var res struct {
		Incidents []incidentJSON `json:"incidents"`
	}
	require.Equal(t, http.StatusOK, getJSON(t, srv.URL+"/api/incidents", &res))
	require.NotNil(t, res.Incidents)
	require.Empty(t, res.Incidents)
}

func TestAPI_Metrics(t *testing.T) {
	srv, host, shutdown := apiServer(t)
	defer shutdown()

	resp, err := http.Get(srv.URL + "/metrics")
	require.NoError(t, err)
	defer resp.Body.Close()
	require.Equal(t, http.StatusOK, resp.StatusCode)
	require.Contains(t, resp.Header.Get("Content-Type"), "text/plain")

	raw, err := io.ReadAll(resp.Body)
	require.NoError(t, err)
	body := string(raw)

	labels := `{id="` + host.ID + `",name="Target",group="Group",type="http"}`
	require.Contains(t, body, `endpoll_build_info{version="test"} 1`)
	require.Contains(t, body, "endpoll_host_up"+labels+" 1\n")
	require.Contains(t, body, `endpoll_host_status{id="`+host.ID+`",name="Target",group="Group",type="http",status="up"} 1`)
	require.Contains(t, body, `endpoll_host_status{id="`+host.ID+`",name="Target",group="Group",type="http",status="down"} 0`)
	require.Contains(t, body, "endpoll_host_response_seconds"+labels+" ")
	require.Contains(t, body, "endpoll_host_response_code"+labels+" 200\n")
	require.Contains(t, body, "endpoll_host_last_check_timestamp_seconds"+labels+" ")
	require.Contains(t, body, "endpoll_host_uptime_ratio"+labels+" 1\n")
	require.NotContains(t, body, "endpoll_host_ssl_expiry_timestamp_seconds"+labels, "plain http host has no certificate")
}

func TestMetrics_labels(t *testing.T) {
	name := "quote \" and \\ back\nline"
	h := &types.Host{ID: "x", Name: &name, Type: types.HttpType}
	l := labels(h, "status", "up")
	require.Equal(t, `{id="x",name="quote \" and \\ back\nline",group="",type="http",status="up"}`, l)
}
