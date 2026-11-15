package pkg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestCache_Expires(t *testing.T) {
	c := &Cache{}
	c.Set("k", 1, 10*time.Millisecond)
	_, ok := c.Get("k")
	require.True(t, ok)
	time.Sleep(20 * time.Millisecond)
	_, ok = c.Get("k")
	require.False(t, ok)
}
