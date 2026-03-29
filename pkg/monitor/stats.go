package monitor

import (
	"context"
	"fmt"
	"math"
	"net/http"
	"sort"
	"strings"
	"time"

	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
)

// Stats - returns the stats of all hosts grouped by groups
func (m *Monitor) Stats(ctx context.Context) (*types.Stats, error) {
	s := &types.Stats{
		IsHost: false,
		Status: types.Unknown,
	}

	groups := make(map[string][]*types.Stats)
	hiddenHosts := make([]string, 0)
	m.mu.RLock()
	for _, w := range m.watchers {
		stats, err := m.StatsByID(ctx, w.host.ID, true)
		if err != nil {
			return nil, err
		}
		host := stats.Hosts[0]
		if w.host.Group == nil {
			s.Hosts = append(s.Hosts, host)
		} else {
			if _, ok := groups[*w.host.Group]; !ok {
				groups[*w.host.Group] = []*types.Stats{}
			}
			groups[*w.host.Group] = append(groups[*w.host.Group], stats)
			if w.host.Hidden {
				hiddenHosts = append(hiddenHosts, w.host.ID)
			}
		}
	}
	m.mu.RUnlock()

	for group, stats := range groups {
		g := types.Stat{
			ID:   group,
			Name: &group,
			Chart: types.Chart{
				Points: make([]*types.Point, 0),
			},
		}

		uptime := 0
		for _, stat := range stats {
			g.Hosts = append(g.Hosts, stat.Hosts[0])
			uptime += stat.Hosts[0].Uptime
		}
		if len(stats) != 0 {
			uptime /= len(stats)
		}
		now := time.Now()
		start := now.Add(-time.Hour * 24 * 90)
		for i := 0; i < 91; i++ {
			ts := start.Add(time.Hour * 24 * time.Duration(i))
			status := generateGroupStatus(&g.Hosts, &i)

			upCount := 0
			downCount := 0
			for _, h := range g.Hosts {
				if i < len(h.Chart.Points) {
					switch h.Chart.Points[i].Status {
					case types.UP:
						upCount++
					case types.DOWN, types.DEGRADED:
						downCount++
					}
				}
			}

			tooltip := ts.Format("2006-01-02") + "\nStatus: " + string(status)
			tooltip += fmt.Sprintf("\nHosts: %d/%d up", upCount, len(g.Hosts))

			g.Chart.Points = append(g.Chart.Points, &types.Point{
				Timestamp: ts.Format("2006-01-02"),
				Status:    status,
				Tooltip:   &tooltip,
				TS:        ts,
			})
		}

		g.Uptime = uptime
		g.Chart.Intervals = genIntervals(g.Chart.Points)
		g.Status = generateGroupStatus(&g.Hosts, nil)
		if len(g.Hosts) != 0 {
			sort.Slice(g.Hosts, func(i, j int) bool {
				return g.Hosts[i].Index < g.Hosts[j].Index
			})
			g.Index = g.Hosts[0].Index
		}

		for _, id := range hiddenHosts {
			for i, h := range g.Hosts {
				if h.ID == id {
					g.Hosts = append(g.Hosts[:i], g.Hosts[i+1:]...)
					break
				}
			}
		}

		s.Hosts = append(s.Hosts, g)
	}

	if len(s.Hosts) != 0 {
		s.Status = generateGroupStatus(&s.Hosts, nil)
	}
	sort.Slice(s.Hosts, func(i, j int) bool {
		return s.Hosts[i].Index < s.Hosts[j].Index
	})

	return s, nil
}

// StatsByID - returns the stats of a host by id
func (m *Monitor) StatsByID(ctx context.Context, id string, dayReport bool) (*types.Stats, error) {
	m.mu.RLock()
	w, ok := m.watchers[id]
	if !ok {
		m.mu.RUnlock()
		return nil, types.ErrHostNotFound
	}
	step := *w.host.Interval
	m.mu.RUnlock()

	history, err := m.Store.FindResponses(ctx, id)
	if err != nil {
		return nil, fmt.Errorf("failed to get history: %w", err)
	}

	incidents, err := m.Store.FindIncidents(ctx, id, 0, 30)
	if err != nil {
		return nil, fmt.Errorf("failed to get incidents: %w", err)
	}
	processIncidents(incidents)

	var details *types.Details
	if !dayReport {
		details = getDetails(history, incidents)
	}

	if !dayReport && len(history) > 90 {
		history = history[len(history)-90:]
	}
	chart, uptime, responseTime := genChart(history, step, dayReport)

	w.mu.RLock()
	status := w.status
	if w.status == "" {
		status = types.Unknown
	}
	s := &types.Stats{
		IsHost: true,
		Status: status,
		Hosts: []types.Stat{
			{
				ID:           w.host.ID,
				Name:         w.host.Name,
				Description:  w.host.Description,
				Host:         w.host.SecureURL(),
				Status:       status,
				Uptime:       uptime,
				ResponseTime: responseTime,
				Chart:        chart,
				Details:      details,
				Index:        w.host.Index,
			},
		},
		Incidents: incidents,
	}
	w.mu.RUnlock()

	return s, nil
}

func (m *Monitor) ResponseTime(ctx context.Context, id string) ([]time.Time, []float64, error) {
	history, err := m.Store.FindResponses(ctx, id)
	if err != nil {
		return nil, nil, fmt.Errorf("failed to get history: %w", err)
	}

	// aggregate data per day
	days := make(map[time.Time][]*types.HttpResponse)
	for _, r := range history {
		day := time.Date(r.Timestamp.Year(), r.Timestamp.Month(), r.Timestamp.Day(), 0, 0, 0, 0, r.Timestamp.Location())
		if _, ok := days[day]; !ok {
			days[day] = make([]*types.HttpResponse, 0)
		}
		days[day] = append(days[day], r)
	}

	// calculate average response time per day
	list := make(map[time.Time]float64)
	for hour, responses := range days {
		var sum float64
		for _, r := range responses {
			sum += float64(r.Time.Milliseconds())
		}
		list[hour] = sum / float64(len(responses))
	}

	keys := make([]time.Time, 0, len(list))
	for k := range list {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		return keys[i].Before(keys[j])
	})

	values := make([]float64, 0, len(list))
	for _, k := range keys {
		values = append(values, list[k])
	}

	return keys, values, nil
}

// genChart - generates the chart for the host.
// genIntervals - generates the intervals for the chart: 90d - 60d - 30d.
// getDetails - generates the details for the host.
// generateGroupStatus - generates the status for the group.
func genChart(history []*types.HttpResponse, interval time.Duration, dayReport bool) (types.Chart, int, string) {
	points := []*types.Point{}
	uptime := 0
	unknown := 0
	responseTime := time.Duration(0)
	format := "2006-01-02 15:04:05"
	historyPoints := 90

	if dayReport {
		now := time.Now()
		startOfDay := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())
		days := make([]*types.HttpResponse, 0)
		today := make([]*types.HttpResponse, 0)
		for _, r := range history {
			if r.Timestamp.After(startOfDay) {
				today = append(today, r)
			} else {
				days = append(days, r)
			}
		}
		if len(days) > historyPoints {
			history = days[len(days)-historyPoints:]
		} else {
			history = days
		}
		history = append(history, store.AggregateDay(startOfDay, today))

		format = "2006-01-02"
		interval = time.Hour * 24
		historyPoints += 1
	}

	start := time.Now().Add(-interval * time.Duration(90))
	for i := 0; i < historyPoints; i++ {
		ts := start.Add(interval * time.Duration(i))
		points = append(points, &types.Point{
			Timestamp: ts.Format(format),
			Status:    types.Unknown,
			TS:        ts,
		})
	}

	space := 0
	if len(history) < len(points) {
		space = len(points) - len(history)
	}
	for i, r := range history {
		pointFormat := "2006-01-02 15:04:05"
		if r.IsAggregated {
			pointFormat = "2006-01-02"
		}
		p := &types.Point{
			Timestamp: r.Timestamp.Format(pointFormat),
			Status:    r.StatusType,
			TS:        r.Timestamp,
		}

		tooltip := r.Timestamp.Format(pointFormat) + "\nStatus: " + string(r.StatusType)
		if r.IsAggregated {
			if r.Uptime > 0 {
				tooltip += fmt.Sprintf("\nUptime: %.2f%%", r.Uptime*100)
			}
		}
		if r.Time > 0 {
			tooltip += "\nResponse time: " + r.Time.Truncate(time.Millisecond).String()
		}
		p.Tooltip = &tooltip

		points[space+i] = p
		responseTime += r.Time
	}

	for _, c := range points {
		if c.Status == types.UP {
			uptime++
		} else if c.Status == types.Unknown {
			unknown++
		}
	}
	if len(points) != unknown {
		uptime = (uptime * 100) / (len(points) - unknown)
	}

	sort.Slice(points, func(i, j int) bool {
		return points[i].TS.Before(points[j].TS)
	})

	return types.Chart{
		Points:    points,
		Intervals: genIntervals(points),
	}, uptime, (responseTime / time.Duration(len(points))).Truncate(time.Millisecond).String()
}
func genIntervals(points []*types.Point) []string {
	if len(points) < 61 {
		return []string{"", "", ""}
	}
	p := []time.Time{points[0].TS, points[30].TS, points[60].TS}
	for i, ts := range p {
		if ts.IsZero() {
			p[i] = time.Now()
		}
	}
	intervals := make([]string, 3)
	for i, ts := range p {
		if ts.IsZero() {
			intervals[i] = ""
			continue
		}
		ago := time.Since(ts)
		if ago.Hours() < 24 {
			rounded := ago
			if rounded > time.Hour {
				rounded = rounded.Round(time.Hour)
			} else if rounded > time.Minute {
				rounded = rounded.Round(time.Minute)
			} else {
				rounded = rounded.Round(time.Millisecond)
			}

			val := rounded.String()
			if strings.Contains(rounded.String(), "m0s") {
				val = strings.ReplaceAll(val, "0s", "")
			}
			if strings.Contains(rounded.String(), "h0m") {
				val = strings.ReplaceAll(val, "0m", "")
			}
			intervals[i] = val
			continue
		}
		days := int(ago.Hours() / 24)
		intervals[i] = fmt.Sprintf("%dd", days)
	}
	return intervals
}
func getDetails(responses []*types.HttpResponse, incidents []*types.Incident) *types.Details {
	d := &types.Details{}

	last30DaysUp := 0
	last30DaysCount := 0
	uptime30Days := 0.0
	responseTime30Days := time.Duration(0)

	for _, r := range responses {
		if r.Timestamp.After(time.Now().Add(-time.Hour * 24 * 30)) {
			last30DaysCount++
			if r.StatusType != types.DOWN {
				last30DaysUp++
			}
			responseTime30Days += r.Time
		}
	}

	if last30DaysCount > 0 {
		uptime30Days = float64(last30DaysUp) * 100 / float64(last30DaysCount)
	}
	if last30DaysUp != 0 {
		responseTime30Days /= time.Duration(last30DaysUp)
	}
	if uptime30Days == math.Trunc(uptime30Days) {
		d.Uptime = fmt.Sprintf("%.0f", uptime30Days)
	} else {
		d.Uptime = fmt.Sprintf("%.2f", uptime30Days)
	}
	d.ResponseTime = formatDuration(responseTime30Days)

	if len(incidents) > 0 {
		lastIncident := incidents[0]
		ts := lastIncident.StartTS
		if lastIncident.EndTS != nil {
			ts = *lastIncident.EndTS
		}
		d.LastOutage = &types.LastOutageDetails{
			Since:    formatDuration(time.Since(ts)),
			TS:       ts.Format("2006-01-02 15:04:05"),
			Duration: lastIncident.Duration,
		}
	}

	if len(responses) > 0 && responses[len(responses)-1].SSLCertExpiry != nil {
		latest := responses[len(responses)-1]
		expireAt := latest.SSLCertExpiry
		d.SSL = &types.SSLDetails{
			ExpireInDays: int(expireAt.Sub(time.Now()).Hours() / 24),
			ExpireTS:     expireAt.Format("January 2, 2006"),
			Issuer:       latest.SSLIssuer,
			TLSVersion:   latest.TLSVersion,
		}
	}

	return d
}
func generateGroupStatus(hosts *[]types.Stat, i *int) types.StatusType {
	allHosts := len(*hosts)
	upHosts := 0
	downHosts := 0
	degradedHosts := 0
	unknownHosts := 0
	for _, stat := range *hosts {
		status := stat.Status
		if i != nil {
			status = stat.Chart.Points[*i].Status
		}
		if status == types.UP {
			upHosts++
		} else if status == types.DOWN {
			downHosts++
		} else if status == types.DEGRADED {
			degradedHosts++
		} else {
			unknownHosts++
		}
	}

	if upHosts == allHosts || (unknownHosts > 0 && upHosts > 0 && downHosts == 0 && degradedHosts == 0) {
		return types.UP
	} else if downHosts == allHosts {
		return types.DOWN
	} else if downHosts > 0 || degradedHosts > 0 {
		return types.DEGRADED
	} else {
		return types.Unknown
	}
}
func processIncidents(list []*types.Incident) {
	for i, e := range list {
		list[i].Start = e.StartTS.Format("2006-01-02 15:04:05")
		text := fmt.Sprintf("Host is down for %s!", formatDuration(time.Now().Sub(e.StartTS)))
		if e.EndTS != nil {
			duration := e.EndTS.Sub(e.StartTS)
			list[i].Duration = formatDuration(duration)
			format := "2006-01-02 15:04:05"
			if duration < time.Hour*24 {
				format = "15:04:05"
			}
			list[i].End = e.EndTS.Format(format)
			text = fmt.Sprintf("Host was down for %s", formatDuration(duration))
		}
		list[i].Text = text
		switch e.Details.StatusCode {
		case 521:
			list[i].Details.StatusText = "Web server is down"
		case 522:
			list[i].Details.StatusText = "Connection timed out"
		case 523:
			list[i].Details.StatusText = "Origin is unreachable"
		default:
			list[i].Details.StatusText = http.StatusText(e.Details.StatusCode)
		}
	}
}

func formatDuration(d time.Duration) string {
	rounded := d.Round(time.Millisecond)
	str := rounded.String()

	if rounded > time.Hour*24 {
		str = fmt.Sprintf("%dd", int(rounded.Hours()/24))
	} else if rounded > time.Hour {
		str = fmt.Sprintf("%dh", int(rounded.Hours()))
	} else if rounded > time.Minute {
		str = fmt.Sprintf("%dm", int(rounded.Minutes()))
	} else if rounded > time.Second {
		str = fmt.Sprintf("%ds", int(rounded.Seconds()))
	} else if rounded > time.Millisecond {
		str = fmt.Sprintf("%dms", rounded.Milliseconds())
	} else if rounded > time.Microsecond {
		str = fmt.Sprintf("%dµs", rounded.Microseconds())
	} else {
		str = fmt.Sprintf("%dns", rounded.Nanoseconds())
	}

	return str
}
