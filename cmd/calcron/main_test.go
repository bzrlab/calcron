package main

import (
	"testing"
	"time"
)

func TestDurationConfiguration(t *testing.T) {
	for _, tc := range []struct {
		value   string
		want    time.Duration
		invalid bool
	}{
		{"", time.Hour, false}, {"720h", 720 * time.Hour, false}, {"30days", 0, true}, {"0s", 0, true}, {"-1h", 0, true},
	} {
		t.Run(tc.value, func(t *testing.T) {
			t.Setenv("CALCRON_RETENTION", tc.value)
			got, err := duration("CALCRON_RETENTION", time.Hour)
			if (err != nil) != tc.invalid || got != tc.want {
				t.Fatalf("got %v, %v", got, err)
			}
		})
	}
}
