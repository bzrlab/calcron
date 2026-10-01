package schedule

import (
	"testing"
	"time"

	"github.com/bzrlab/calcron/internal/calendar"
)

func TestNextErrPermanent(t *testing.T) {
	if !nextErrPermanent(calendar.ErrNoEligibleDate) {
		t.Error("no eligible date should retire the schedule")
	}
	if !nextErrPermanent(calendar.ErrTimezone) {
		t.Error("an unloadable timezone should retire the schedule")
	}
	if nextErrPermanent(calendar.ErrLocalTimeGap) {
		t.Error("a DST gap clears on its own and must not retire the schedule")
	}
}

// A timezone that cannot be loaded is reachable: Load reads the column directly and
// never runs the calendar's own validation, so a definition written before validation
// existed would otherwise defer forever with no operator signal.
func TestUnloadableTimezoneRetiresSchedule(t *testing.T) {
	_, err := calendar.NextTime(calendar.Definition{Timezone: "Not/AZone", Weekdays: []int{1}}, time.Now().UTC(), "10:00")
	if err == nil {
		t.Fatal("NextTime accepted a timezone that cannot be loaded")
	}
	if !nextErrPermanent(err) {
		t.Errorf("nextErrPermanent(%v) = false, want true", err)
	}
}
