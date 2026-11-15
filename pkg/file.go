package pkg

import (
	"context"
	"fmt"
	"os"
	"time"
)

// WatchFile - polls a file and reports its modification time when it changes.
// When initial is true, the first successful poll also reports the current value.
// The channel is closed when the context is canceled.
func WatchFile(ctx context.Context, path string, interval time.Duration, initial bool) (<-chan time.Time, error) {
	if interval <= 0 {
		return nil, fmt.Errorf("watch interval must be positive")
	}
	fi, err := os.Stat(path)
	if err != nil {
		return nil, fmt.Errorf("stat file %s: %w", path, err)
	}

	modTimestamp := fi.ModTime()
	if initial {
		modTimestamp = time.Time{}
	}
	ch := make(chan time.Time)

	go func() {
		defer close(ch)
		tk := time.NewTicker(interval)
		defer tk.Stop()

		for {
			select {
			case <-tk.C:
				fi, err := os.Stat(path)
				if err != nil {
					continue
				}
				if fi.ModTime() != modTimestamp {
					modTimestamp = fi.ModTime()
					select {
					case ch <- modTimestamp:
					case <-ctx.Done():
						return
					}
				}
			case <-ctx.Done():
				return
			}
		}
	}()

	return ch, nil
}
