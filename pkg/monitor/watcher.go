package monitor

import (
	"context"
	"fmt"
	"log"
	"sync"
	"time"

	"github.com/exelban/EndPoll/pkg/dialer"
	"github.com/exelban/EndPoll/pkg/notify"
	"github.com/exelban/EndPoll/store"
	"github.com/exelban/EndPoll/types"
)

type watcher struct {
	dialer *dialer.Dialer
	notify *notify.Notify
	store  store.Interface
	host   *types.Host

	status    types.StatusType
	lastCheck time.Time

	successCount int
	failureCount int

	ctx    context.Context
	cancel context.CancelFunc

	incident *types.Incident

	mu sync.RWMutex
}

// run - runs check loop for host
func (w *watcher) run(ctx context.Context) {
	ctx, cancel := context.WithCancel(ctx)
	w.ctx = ctx
	w.cancel = cancel

	incidents, err := w.store.FindIncidents(ctx, w.host.ID, 0, 1)
	if err != nil {
		log.Printf("[ERROR] get incidents for %s: %s", w.host.String(), err)
	}
	if len(incidents) > 0 && incidents[0].EndTS == nil {
		w.incident = incidents[0]
	}
	if lastResponse, err := w.store.LastResponse(ctx, w.host.ID); err == nil && lastResponse != nil {
		w.status = lastResponse.StatusType
	}

	log.Printf("[INFO] %s: new watcher", w.host.String())

	if w.host.InitialDelay != nil {
		time.Sleep(*w.host.InitialDelay)
	}
	w.check()

	ticker := time.NewTicker(*w.host.Interval)
	for {
		select {
		case <-ticker.C:
			w.check()
		case <-ctx.Done():
			log.Printf("[DEBUG] %s: stopped", w.host.String())
			ticker.Stop()
			return
		}
	}
}

// check - call the host and check host status
func (w *watcher) check() {
	resp := w.dialer.Dial(w.ctx, w.host)

	w.mu.Lock()
	resp.Status = w.host.Status(resp.Code, resp.Bytes)
	w.lastCheck = time.Now()
	w.validate(&resp)
	resp.StatusType = w.status
	if err := w.store.AddResponse(w.ctx, w.host.ID, &resp); err != nil {
		log.Printf("[ERROR] save response to db %s: %s", w.host.String(), err)
	}
	w.mu.Unlock()

	debug := fmt.Sprintf("[DEBUG] %s (%s): %s status", w.host.String(), w.host.ID, w.status)
	if w.status != types.UP {
		debug += fmt.Sprintf(" (%d - %s)", resp.Code, resp.Body)
	}
	log.Println(debug)
}

// validate - set status based on response status and thresholds
func (w *watcher) validate(resp *types.HttpResponse) {
	if resp.Status { // host is up
		w.successCount++
		w.failureCount = 0
		if w.successCount >= w.host.SuccessThreshold {
			newStatus := types.UP
			if w.status != types.Unknown && w.status != types.UP {
				if err := w.notify.Send(w.host, newStatus); err != nil {
					log.Print(err)
				}

				if w.incident != nil {
					incidentDuration := time.Since(w.incident.StartTS)
					if incidentDuration > time.Second {
						if err := w.store.EndIncident(w.ctx, w.host.ID, w.incident.ID, time.Now()); err != nil {
							log.Printf("[ERROR] end incident in db %s: %s", w.host.String(), err)
						}
					} else {
						if err := w.store.DeleteIncident(w.ctx, w.host.ID, w.incident.ID); err != nil {
							log.Printf("[ERROR] delete incident in db %s: %s", w.host.String(), err)
						}
					}
					w.incident = nil
				}
			}
			w.status = newStatus
		}
	} else { // host is down
		w.failureCount++
		w.successCount = 0
		if w.failureCount >= w.host.FailureThreshold {
			newStatus := types.DOWN
			if w.status != types.Unknown && w.status != types.DOWN {
				if err := w.notify.Send(w.host, newStatus); err != nil {
					log.Print(err)
				}

				if w.incident == nil {
					w.incident = &types.Incident{
						Details: types.IncidentDetails{
							StatusCode: resp.Code,
							Response:   resp.Body,
							TS:         resp.Timestamp,
						},
						StartTS: time.Now(),
					}
					if err := w.store.AddIncident(w.ctx, w.host.ID, w.incident); err != nil {
						log.Printf("[ERROR] save incident to db %s: %s", w.host.String(), err)
					}
				}
			}
			w.status = newStatus
		}
	}

	if w.status == "" {
		w.status = types.Unknown
	}
}
