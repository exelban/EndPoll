package pkg

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestWatchFile(t *testing.T) {
	for _, initial := range []bool{false, true} {
		name := "changes only"
		if initial {
			name = "initial and changes"
		}
		t.Run(name, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "file")
			require.NoError(t, os.WriteFile(path, []byte("content"), 0600))
			before := time.Now().Add(-time.Hour).Truncate(time.Second)
			require.NoError(t, os.Chtimes(path, before, before))

			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			ch, err := WatchFile(ctx, path, 10*time.Millisecond, initial)
			require.NoError(t, err)

			if initial {
				select {
				case ts := <-ch:
					require.True(t, ts.Equal(before))
				case <-time.After(time.Second):
					t.Fatal("initial notification was not delivered")
				}
			}
			select {
			case <-ch:
				t.Fatal("unchanged file produced a notification")
			case <-time.After(30 * time.Millisecond):
			}

			after := before.Add(time.Minute)
			require.NoError(t, os.Chtimes(path, after, after))
			select {
			case ts := <-ch:
				require.True(t, ts.Equal(after))
			case <-time.After(time.Second):
				t.Fatal("file change was not delivered")
			}
		})
	}
}

func TestWatchFile_CancelPendingChange(t *testing.T) {
	path := filepath.Join(t.TempDir(), "page.html")
	require.NoError(t, os.WriteFile(path, []byte("page"), 0600))

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	ch, err := WatchFile(ctx, path, 10*time.Millisecond, false)
	require.NoError(t, err)

	future := time.Now().Add(time.Hour)
	require.NoError(t, os.Chtimes(path, future, future))
	time.Sleep(30 * time.Millisecond)

	cancel()
	done := make(chan struct{})
	go func() {
		for range ch {
		}
		close(done)
	}()
	select {
	case <-done:
	case <-time.After(time.Second):
		t.Fatal("file watcher did not stop")
	}
}

func TestWatchFile_InvalidInput(t *testing.T) {
	path := filepath.Join(t.TempDir(), "missing")
	_, err := WatchFile(context.Background(), path, time.Second, false)
	require.ErrorIs(t, err, os.ErrNotExist)

	for _, interval := range []time.Duration{0, -time.Second} {
		_, err := WatchFile(context.Background(), path, interval, false)
		require.EqualError(t, err, "watch interval must be positive")
	}
}
