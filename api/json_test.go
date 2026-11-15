package api

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/exelban/EndPoll/internal/monitor"
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
	require.False(t, h.LastCheck.IsZero())
	require.NotNil(t, h.Last)
	require.Equal(t, 200, h.Last.Code)
}

func TestAPI_Host(t *testing.T) {
	srv, host, shutdown := apiServer(t)
	defer shutdown()

	var res hostJSON
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

func TestAPI_HostWithoutChecks(t *testing.T) {
	ctx := context.Background()
	interval := time.Hour
	host := &types.Host{
		ID: "test", URL: "https://user:secret@example.com", Type: types.HttpType,
		Interval: &interval, InitialDelay: &interval,
	}
	m := &monitor.Monitor{Store: store.NewMemory(ctx)}
	require.NoError(t, m.Run(ctx, &types.Cfg{Hosts: []*types.Host{host}}))
	defer m.Stop()

	rest := &Rest{Monitor: m}
	router := rest.Router()
	w := httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/hosts", nil))
	require.Equal(t, http.StatusOK, w.Code)
	require.JSONEq(t, `{
		"status": "unknown",
		"hosts": [{
			"id": "test", "url": "https://user:*****@example.com",
			"type": "http", "status": "unknown", "uptime": 0
		}]
	}`, w.Body.String())

	w = httptest.NewRecorder()
	router.ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/hosts/test", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var res map[string]json.RawMessage
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	require.NotContains(t, res, "lastCheck")
	require.NotContains(t, res, "last")
	require.Contains(t, res, "details")
	require.JSONEq(t, `[]`, string(res["history"]))
	require.JSONEq(t, `[]`, string(res["incidents"]))
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

func TestAPI_IncidentsOrder(t *testing.T) {
	ctx := context.Background()
	interval := time.Hour
	hosts := []*types.Host{
		{ID: "first", URL: "https://first.example.com", Interval: &interval, InitialDelay: &interval, Index: 0},
		{ID: "second", URL: "https://second.example.com", Interval: &interval, InitialDelay: &interval, Index: 1},
	}
	now := time.Now()
	s := store.NewMemory(ctx)
	require.NoError(t, s.AddIncident(ctx, "first", &types.Incident{StartTS: now.Add(-2 * time.Hour)}))
	require.NoError(t, s.AddIncident(ctx, "first", &types.Incident{StartTS: now.Add(-time.Hour)}))
	require.NoError(t, s.AddIncident(ctx, "second", &types.Incident{StartTS: now.Add(-time.Hour)}))
	require.NoError(t, s.AddIncident(ctx, "second", &types.Incident{StartTS: now.Add(-3 * time.Hour)}))

	m := &monitor.Monitor{Store: s}
	require.NoError(t, m.Run(ctx, &types.Cfg{Hosts: hosts}))
	defer m.Stop()

	rest := &Rest{Monitor: m}
	w := httptest.NewRecorder()
	rest.Router().ServeHTTP(w, httptest.NewRequest(http.MethodGet, "/api/incidents", nil))
	require.Equal(t, http.StatusOK, w.Code)
	var res struct {
		Incidents []incidentJSON `json:"incidents"`
	}
	require.NoError(t, json.Unmarshal(w.Body.Bytes(), &res))
	want := []struct {
		hostID string
		id     int
	}{
		{hostID: "first", id: 2},
		{hostID: "second", id: 1},
		{hostID: "first", id: 1},
		{hostID: "second", id: 2},
	}
	require.Len(t, res.Incidents, len(want))
	for i, e := range want {
		require.Equal(t, e.hostID, res.Incidents[i].HostID)
		require.Equal(t, e.id, res.Incidents[i].ID)
	}
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
