package monitor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/exelban/EndPoll/internal/connectivity"
	"github.com/exelban/EndPoll/internal/dialer"
	"github.com/exelban/EndPoll/internal/notify"
	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
)

type watcher struct {
	dialer       *dialer.Dialer
	notify       *notify.Notify
	connectivity *connectivity.Checker
	store        store.Interface
	host         *types.Host

	status       types.StatusType
	lastCheck    time.Time
	lastResponse *types.HttpResponse

	// onChange - called after every recorded check (invalidates the stats cache)
	onChange func()

	successCount int
	failureCount int

	ctx    context.Context
	cancel context.CancelFunc
	done   chan struct{}

	incident         *types.Incident
	pendingIncidents []*types.Incident

	mu sync.RWMutex
}

// start - starts the check loop in a separate goroutine.
func (w *watcher) start(parent context.Context, delay bool) {
	if parent == nil {
		parent = context.Background()
	}
	ctx, cancel := context.WithCancel(parent)
	done := make(chan struct{})

	w.mu.Lock()
	w.ctx = ctx
	w.cancel = cancel
	w.done = done
	w.mu.Unlock()

	go func() {
		defer close(done)
		w.loop(ctx, delay)
	}()
}

// stop - stops the check loop and waits for it to finish
func (w *watcher) stop() {
	w.mu.RLock()
	cancel, done := w.cancel, w.done
	w.mu.RUnlock()

	if cancel != nil {
		cancel()
	}
	if done != nil {
		<-done
	}
}

// run - starts the check loop and blocks until it finishes
func (w *watcher) run(ctx context.Context) {
	w.start(ctx, true)
	w.mu.RLock()
	done := w.done
	w.mu.RUnlock()
	<-done
}

// update - replaces the host definition and the dependencies. Reports whether
// the host configuration changed (and the loop must be restarted).
func (w *watcher) update(host *types.Host, d *dialer.Dialer, n *notify.Notify, c *connectivity.Checker) bool {
	w.mu.Lock()
	defer w.mu.Unlock()

	changed := w.host.Changed(host)
	w.host = host
	w.dialer = d
	w.notify = n
	w.connectivity = c

	return changed
}

// snapshot - returns the host and the status under the lock
func (w *watcher) snapshot() (*types.Host, types.StatusType) {
	s := w.snap()
	return s.Host, s.Status
}

// snap - returns the full state of the watcher under the lock
func (w *watcher) snap() Snapshot {
	w.mu.RLock()
	defer w.mu.RUnlock()

	status := w.status
	if status == "" {
		status = types.Unknown
	}
	return Snapshot{
		Host:         w.host,
		Status:       status,
		LastCheck:    w.lastCheck,
		LastResponse: w.lastResponse,
	}
}

// loop - the check loop of the host
func (w *watcher) loop(ctx context.Context, delay bool) {
	w.mu.RLock()
	host := w.host
	s := w.store
	w.mu.RUnlock()

	incidents, err := s.FindIncidents(ctx, host.ID, 0, 1)
	if err != nil {
		log.Printf("[ERROR] get incidents for %s: %s", host.String(), err)
	}
	lastResponse, err := s.LastResponse(ctx, host.ID)
	if err != nil {
		log.Printf("[ERROR] get last response for %s: %s", host.String(), err)
	}

	w.mu.Lock()
	if w.incident == nil && len(w.pendingIncidents) == 0 && len(incidents) > 0 && incidents[0].EndTS == nil {
		w.incident = incidents[0]
	}
	if w.status == "" && lastResponse != nil {
		w.status = lastResponse.StatusType
	}
	w.mu.Unlock()

	log.Printf("[INFO] %s: new watcher", host.String())

	if delay && host.InitialDelay != nil && *host.InitialDelay > 0 {
		select {
		case <-time.After(*host.InitialDelay):
		case <-ctx.Done():
			return
		}
	}
	w.check()

	ticker := time.NewTicker(*host.Interval)
	defer ticker.Stop()
	for {
		select {
		case <-ticker.C:
			w.check()
		case <-ctx.Done():
			log.Printf("[DEBUG] %s: stopped", host.String())
			return
		}
	}
}

// check - call the host and check host status
func (w *watcher) check() {
	w.mu.RLock()
	ctx := w.ctx
	host := w.host
	d := w.dialer
	c := w.connectivity
	w.mu.RUnlock()
	if ctx == nil {
		ctx = context.Background()
	}

	resp := d.Dial(ctx, host)
	if ctx.Err() != nil {
		// the watcher was stopped while dialing: the result must not be recorded
		return
	}

	resp.Status = resp.OK && host.Status(resp.Code, resp.Bytes)

	// if the check failed but the monitor itself has no connectivity, skip the
	// evaluation entirely: don't count it as a failure, change status or alert.
	// This prevents false-positive DOWN statuses during a local internet outage.
	if !resp.Status && c != nil && !c.Online() {
		log.Printf("[WARN] %s: check failed but monitor has no connectivity, skipping", host.String())
		return
	}

	w.mu.Lock()
	if ctx.Err() != nil || w.host != host {
		w.mu.Unlock()
		return
	}
	w.lastCheck = time.Now()
	notification := w.validate(&resp)
	resp.StatusType = w.status
	if err := w.store.AddResponse(ctx, host.ID, &resp); err != nil {
		log.Printf("[ERROR] save response to db %s: %s", host.String(), err)
	}
	last := resp
	last.Bytes = nil
	w.lastResponse = &last
	status := w.status
	n := w.notify
	onChange := w.onChange
	w.mu.Unlock()

	if onChange != nil {
		onChange()
	}

	// the notification is sent outside the lock: it is a network call and must not
	// block the readers (stats) for the duration of the request.
	if notification != "" && n != nil {
		if err := n.Send(ctx, host, notification); err != nil {
			log.Printf("[ERROR] %s: send notification: %s", host.String(), err)
		}
	}

	debug := fmt.Sprintf("[DEBUG] %s (%s): %s status", host.String(), host.ID, status)
	if status != types.UP {
		debug += fmt.Sprintf(" (%d - %s)", resp.Code, resp.Body)
	}
	log.Println(debug)
}

// validate - set status based on response status and thresholds. Returns the status
// that must be notified (empty when the status did not change). Must be called with
// the lock held.
func (w *watcher) validate(resp *types.HttpResponse) (notification types.StatusType) {
	ctx := w.ctx
	if ctx == nil {
		ctx = context.Background()
	}
	if w.status == "" {
		w.status = types.Unknown
	}

	if resp.Status { // host is up
		w.successCount++
		w.failureCount = 0
		if w.successCount >= w.host.SuccessThreshold {
			newStatus := types.UP
			if w.status != types.Unknown && w.status != types.UP {
				notification = newStatus
			}
			if w.incident != nil && w.incident.EndTS == nil {
				ts := time.Now()
				w.incident.EndTS = &ts
				w.pendingIncidents = append(w.pendingIncidents, w.incident)
				w.incident = nil
			}
			w.status = newStatus
		}
	} else { // host is down
		w.failureCount++
		w.successCount = 0
		if w.failureCount >= w.host.FailureThreshold {
			newStatus := types.DOWN
			if w.status != types.DOWN {
				notification = newStatus
			}
			w.status = newStatus
		}
	}

	for len(w.pendingIncidents) > 0 {
		if err := w.saveIncident(ctx, w.pendingIncidents[0]); err != nil {
			log.Printf("[ERROR] end incident in db %s: %s", w.host.String(), err)
			break
		}
		w.pendingIncidents[0] = nil
		w.pendingIncidents = w.pendingIncidents[1:]
	}

	if w.status == types.DOWN && w.incident == nil {
		w.incident = &types.Incident{
			Details: types.IncidentDetails{
				StatusCode: resp.Code,
				Response:   resp.Body,
				TS:         resp.Timestamp,
			},
			StartTS: time.Now(),
		}
	}
	if w.incident != nil && w.incident.ID == 0 {
		if err := w.saveIncident(ctx, w.incident); err != nil {
			log.Printf("[ERROR] save incident to db %s: %s", w.host.String(), err)
		}
	}

	return notification
}

func (w *watcher) saveIncident(ctx context.Context, e *types.Incident) error {
	if e.EndTS != nil && e.EndTS.Sub(e.StartTS) <= time.Second {
		if e.ID == 0 {
			return nil
		}
		return w.store.DeleteIncident(ctx, w.host.ID, e.ID)
	}
	if e.ID == 0 {
		copy := *e
		if err := w.store.AddIncident(ctx, w.host.ID, &copy); err != nil {
			return err
		}
		e.ID = copy.ID
		return nil
	}
	return w.store.EndIncident(ctx, w.host.ID, e.ID, *e.EndTS)
}
