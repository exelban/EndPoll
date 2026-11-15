package api

import (
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/http"
	"sort"
	"time"

	"github.com/exelban/EndPoll/internal/monitor"
	"github.com/exelban/EndPoll/types"
)

// hostJSON - the state of a host in the API responses
type hostJSON struct {
	ID          string           `json:"id"`
	Name        *string          `json:"name,omitempty"`
	Description *string          `json:"description,omitempty"`
	URL         string           `json:"url"`
	Group       *string          `json:"group,omitempty"`
	Type        types.HostType   `json:"type"`
	Hidden      bool             `json:"hidden,omitempty"`
	Status      types.StatusType `json:"status"`
	Uptime      int              `json:"uptime"` // last 90 days, percent
	LastCheck   time.Time        `json:"lastCheck,omitzero"`
	Last        *lastCheckJSON   `json:"last,omitempty"`

	Details   *types.Details `json:"details,omitempty"`
	History   []pointJSON    `json:"history,omitzero"`
	Incidents []incidentJSON `json:"incidents,omitzero"`
}

// lastCheckJSON - the result of the last check
type lastCheckJSON struct {
	Code           int        `json:"code,omitempty"`
	ResponseTimeMS int64      `json:"responseTimeMs"`
	Message        string     `json:"message,omitempty"`
	SSLExpiry      *time.Time `json:"sslExpiry,omitempty"`
	SSLIssuer      string     `json:"sslIssuer,omitempty"`
	TLSVersion     string     `json:"tlsVersion,omitempty"`
}

type pointJSON struct {
	Timestamp time.Time        `json:"timestamp"`
	Status    types.StatusType `json:"status"`
}

type incidentJSON struct {
	ID         int        `json:"id"`
	HostID     string     `json:"hostId"`
	HostName   *string    `json:"hostName,omitempty"`
	Start      time.Time  `json:"start"`
	End        *time.Time `json:"end,omitempty"`
	Duration   string     `json:"duration,omitempty"`
	StatusCode int        `json:"statusCode,omitempty"`
	StatusText string     `json:"statusText,omitempty"`
	Response   string     `json:"response,omitempty"`
}

// toHostJSON - converts the snapshot (and the 90-day uptime from the stats) to the API structure
func toHostJSON(snap monitor.Snapshot, uptime int) hostJSON {
	h := hostJSON{
		ID:          snap.Host.ID,
		Name:        snap.Host.Name,
		Description: snap.Host.Description,
		URL:         snap.Host.SecureURL(),
		Group:       snap.Host.Group,
		Type:        snap.Host.Type,
		Hidden:      snap.Host.Hidden,
		Status:      snap.Status,
		Uptime:      uptime,
		LastCheck:   snap.LastCheck,
	}
	if r := snap.LastResponse; r != nil {
		h.Last = &lastCheckJSON{
			Code:           r.Code,
			ResponseTimeMS: r.Time.Milliseconds(),
			SSLExpiry:      r.SSLCertExpiry,
			SSLIssuer:      r.SSLIssuer,
			TLSVersion:     r.TLSVersion,
		}
		if !r.Status {
			h.Last.Message = r.Body
		}
	}
	return h
}

func toIncidentJSON(host *types.Host, e *types.Incident) incidentJSON {
	return incidentJSON{
		ID:         e.ID,
		HostID:     host.ID,
		HostName:   host.Name,
		Start:      e.StartTS,
		End:        e.EndTS,
		Duration:   e.Duration,
		StatusCode: e.Details.StatusCode,
		StatusText: e.Details.StatusText,
		Response:   e.Details.Response,
	}
}

// apiHosts - GET /api/hosts: the state of every host
func (s *Rest) apiHosts(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	hosts := make([]hostJSON, 0)
	for _, snap := range s.Monitor.Snapshots() {
		uptime := 0
		if stats, err := s.Monitor.StatsByID(ctx, snap.Host.ID, true); err == nil && len(stats.Hosts) > 0 {
			uptime = stats.Hosts[0].Uptime
		}
		hosts = append(hosts, toHostJSON(snap, uptime))
	}

	status := types.Unknown
	if stats, err := s.Monitor.Stats(ctx); err == nil {
		status = stats.Status
	}

	writeJSON(w, http.StatusOK, map[string]any{
		"status": status,
		"hosts":  hosts,
	})
}

// apiHost - GET /api/hosts/{id}: the detailed state of a host
func (s *Rest) apiHost(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()
	id := r.PathValue("id")

	snap, err := s.Monitor.Snapshot(id)
	if err != nil {
		jsonError(w, http.StatusNotFound, "host not found")
		return
	}
	stats, err := s.Monitor.StatsByID(ctx, id, false)
	if err != nil {
		if errors.Is(err, types.ErrHostNotFound) {
			jsonError(w, http.StatusNotFound, "host not found")
			return
		}
		log.Printf("[ERROR] get stats: %v", err)
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("get stats: %v", err))
		return
	}

	stat := stats.Hosts[0]
	res := toHostJSON(snap, stat.Uptime)
	res.Details = stat.Details
	res.History = make([]pointJSON, 0, len(stat.Chart.Points))
	res.Incidents = make([]incidentJSON, 0, len(stats.Incidents))
	for _, p := range stat.Chart.Points {
		if p.Status == types.Unknown && p.Tooltip == nil {
			continue // empty slot of the chart
		}
		res.History = append(res.History, pointJSON{Timestamp: p.TS, Status: p.Status})
	}
	for _, e := range stats.Incidents {
		res.Incidents = append(res.Incidents, toIncidentJSON(snap.Host, e))
	}

	writeJSON(w, http.StatusOK, res)
}

// apiResponseTime - GET /api/hosts/{id}/response-time: the average response time per day
func (s *Rest) apiResponseTime(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")

	x, y, err := s.Monitor.ResponseTime(r.Context(), id)
	if err != nil {
		if errors.Is(err, types.ErrHostNotFound) {
			jsonError(w, http.StatusNotFound, "host not found")
			return
		}
		jsonError(w, http.StatusInternalServerError, fmt.Sprintf("get response time: %v", err))
		return
	}

	type point struct {
		Date           string  `json:"date"`
		ResponseTimeMS float64 `json:"responseTimeMs"`
	}
	points := make([]point, 0, len(x))
	for i := range x {
		points = append(points, point{Date: x[i].Format("2006-01-02"), ResponseTimeMS: y[i]})
	}
	writeJSON(w, http.StatusOK, points)
}

// apiIncidents - GET /api/incidents: the recent incidents of every host, newest first
func (s *Rest) apiIncidents(w http.ResponseWriter, r *http.Request) {
	ctx := r.Context()

	incidents := make([]incidentJSON, 0)
	for _, snap := range s.Monitor.Snapshots() {
		stats, err := s.Monitor.StatsByID(ctx, snap.Host.ID, true)
		if err != nil {
			continue
		}
		for _, e := range stats.Incidents {
			incidents = append(incidents, toIncidentJSON(snap.Host, e))
		}
	}
	sort.SliceStable(incidents, func(i, j int) bool {
		return incidents[i].Start.After(incidents[j].Start)
	})

	writeJSON(w, http.StatusOK, map[string]any{"incidents": incidents})
}

func (s *Rest) health(w http.ResponseWriter, r *http.Request) {
	status := s.Monitor.Status()
	code := http.StatusServiceUnavailable
	if status == types.UP {
		code = http.StatusOK
	}
	writeJSON(w, code, map[string]types.StatusType{"status": status})
}

func writeJSON(w http.ResponseWriter, code int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(code)
	if err := json.NewEncoder(w).Encode(v); err != nil {
		log.Printf("[ERROR] write json: %v", err)
	}
}
func jsonError(w http.ResponseWriter, code int, msg string) {
	writeJSON(w, code, map[string]string{"error": msg})
}
