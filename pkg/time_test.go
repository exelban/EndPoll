package pkg

import (
	"testing"
	"time"

	"github.com/stretchr/testify/require"
)

func TestStartOfDay(t *testing.T) {
	location, err := time.LoadLocation("Europe/Warsaw")
	require.NoError(t, err)

	cases := []struct {
		name  string
		input time.Time
		want  string
	}{
		{name: "zero", input: time.Time{}, want: "0001-01-01T00:00:00Z"},
		{name: "utc", input: time.Date(2026, 10, 12, 23, 59, 59, 123, time.UTC), want: "2026-10-12T00:00:00Z"},
		{name: "offset", input: time.Date(2026, 10, 12, 1, 30, 0, 0, time.FixedZone("offset", 5*3600+45*60)), want: "2026-10-12T00:00:00+05:45"},
		{name: "daylight saving starts", input: time.Date(2026, 3, 29, 12, 0, 0, 0, location), want: "2026-03-29T00:00:00+01:00"},
		{name: "daylight saving ends", input: time.Date(2026, 10, 25, 12, 0, 0, 0, location), want: "2026-10-25T00:00:00+02:00"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			day := StartOfDay(tc.input)
			require.Equal(t, tc.want, day.Format(time.RFC3339))
			require.Same(t, tc.input.Location(), day.Location())
		})
	}
}
