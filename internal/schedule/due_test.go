package schedule

import (
	"testing"
	"time"
)

func TestLatestDueJumpsToTheNewestMinute(t *testing.T) {
	from := time.Date(1990, 1, 1, 0, 0, 0, 0, time.UTC)
	now := time.Date(2026, 10, 14, 8, 17, 40, 0, time.UTC)
	raw := []byte(`{"every":"1m"}`)
	type outcome struct {
		at  time.Time
		err error
	}
	done := make(chan outcome, 1)
	go func() {
		at, err := latestDue(from, raw, now)
		done <- outcome{at, err}
	}()
	select {
	case got := <-done:
		if got.err != nil {
			t.Fatal(got.err)
		}
		want := now.Truncate(time.Minute)
		if !got.at.Equal(want) {
			t.Fatalf("latest due %v, want %v", got.at, want)
		}
	case <-time.After(200 * time.Millisecond):
		t.Fatal("latest due walked every missed minute")
	}
}

func TestLatestDueDoesNotReturnAnOlderSlot(t *testing.T) {
	from := time.Date(2026, 6, 15, 8, 0, 30, 0, time.UTC)
	now := time.Date(2026, 6, 15, 8, 0, 45, 0, time.UTC)
	got, err := latestDue(from, []byte(`{"every":"1m"}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(from) {
		t.Fatalf("latest due %v, want the armed slot %v", got, from)
	}
	future := now.Add(time.Minute)
	got, err = latestDue(future, []byte(`{"every":"1m"}`), now)
	if err != nil {
		t.Fatal(err)
	}
	if !got.Equal(future) {
		t.Fatalf("latest due %v, want the future slot %v", got, future)
	}
}

func TestLatestDueDailyUsesTheNewestClock(t *testing.T) {
	loc, err := time.LoadLocation("Asia/Dhaka")
	if err != nil {
		t.Fatal(err)
	}
	from := time.Date(2024, 1, 1, 9, 30, 0, 0, loc)
	now := time.Date(2026, 6, 15, 10, 0, 0, 0, loc)
	got, err := latestDue(from.UTC(), []byte(`{"daily":"09:30","timezone":"Asia/Dhaka"}`), now.UTC())
	if err != nil {
		t.Fatal(err)
	}
	want := time.Date(2026, 6, 15, 9, 30, 0, 0, loc).UTC()
	if !got.Equal(want) {
		t.Fatalf("latest due %v, want %v", got, want)
	}
}
