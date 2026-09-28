// Package calendar owns business-calendar validation and occurrence lookup.
package calendar

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"

	"github.com/bzrlab/calcron/internal/protocol"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Definition struct {
	Timezone  string          `json:"timezone"`
	Weekdays  []int           `json:"weekdays"`
	Overrides map[string]bool `json:"overrides"`
}
type Module struct{ db *pgxpool.Pool }

func New(db *pgxpool.Pool) *Module { return &Module{db: db} }

func (m *Module) Set(ctx context.Context, f protocol.Frame) protocol.Reply {
	if f.ApplicationID == "" || f.Name == "" {
		return fail("applicationId and name required")
	}
	if _, err := Parse(f.Data); err != nil {
		return fail(err.Error())
	}
	if _, err := m.db.Exec(ctx, `insert into calendars(application_id,name,definition) values($1,$2,$3) on conflict(application_id,name) do update set definition=excluded.definition,updated_at=now()`, f.ApplicationID, f.Name, f.Data); err != nil {
		return fail(err.Error())
	}
	return ok(nil)
}

func (m *Module) Next(ctx context.Context, f protocol.Frame) protocol.Reply {
	if f.ApplicationID == "" || f.Calendar == "" || f.LocalTime == "" {
		return fail("applicationId, calendar and localTime required")
	}
	definition, err := m.Load(ctx, f.ApplicationID, f.Calendar)
	if err != nil {
		return fail(err.Error())
	}
	from := time.Now().UTC()
	if f.At != "" {
		from, err = time.Parse(time.RFC3339, f.At)
		if err != nil {
			return fail("invalid at")
		}
	}
	next, err := NextTime(definition, from, f.LocalTime)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]any{"nextAt": next.Format(time.RFC3339)})
}

func (m *Module) Occurrences(ctx context.Context, f protocol.Frame) protocol.Reply {
	if f.LocalTime == "" || f.At == "" || f.Until == "" {
		return fail("localTime, at and until required")
	}
	var definition Definition
	var err error
	switch {
	case f.Calendar != "" && f.ApplicationID == "":
		return fail("applicationId required with calendar")
	case f.Calendar != "":
		definition, err = m.Load(ctx, f.ApplicationID, f.Calendar)
	case len(f.Data) > 0:
		definition, err = Parse(f.Data)
	default:
		return fail("calendar or data required")
	}
	if err != nil {
		return fail(err.Error())
	}
	loc, _ := time.LoadLocation(definition.Timezone)
	from, errFrom := time.Parse(time.RFC3339, f.At)
	until, errUntil := time.Parse(time.RFC3339, f.Until)
	if errFrom != nil || errUntil != nil {
		return fail("invalid at or until")
	}
	if !until.After(from) || until.Sub(from) > 366*24*time.Hour {
		return fail("until must be after at and within 366 days")
	}
	occurrences := []string{}
	for from = from.Add(-time.Nanosecond); ; {
		next, nextErr := NextTime(definition, from, f.LocalTime)
		if errors.Is(nextErr, ErrLocalTimeGap) {
			local := from.In(loc)
			from = time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc)
			if !from.Before(until) {
				break
			}
			continue
		}
		if errors.Is(nextErr, ErrNoEligibleDate) {
			break
		}
		if nextErr != nil {
			return fail(nextErr.Error())
		}
		if !next.Before(until) {
			break
		}
		occurrences = append(occurrences, next.Format(time.RFC3339))
		from = next
	}
	return ok(map[string]any{"occurrences": occurrences})
}

func Parse(raw []byte) (Definition, error) {
	var definition Definition
	if json.Unmarshal(raw, &definition) != nil || definition.Timezone == "" {
		return definition, errors.New("invalid calendar")
	}
	if _, err := time.LoadLocation(definition.Timezone); err != nil {
		return definition, errors.New("invalid timezone")
	}
	if len(definition.Weekdays) == 0 {
		return definition, errors.New("calendar needs weekdays")
	}
	return definition, nil
}
func (m *Module) Load(ctx context.Context, app, name string) (Definition, error) {
	var raw []byte
	var definition Definition
	if m.db.QueryRow(ctx, `select definition from calendars where application_id=$1 and name=$2`, app, name).Scan(&raw) != nil {
		return definition, errors.New("calendar not found")
	}
	return definition, json.Unmarshal(raw, &definition)
}

var (
	ErrLocalTimeGap   = errors.New("localTime does not exist on calendar date")
	ErrNoEligibleDate = errors.New("no eligible calendar date")
)

func NextTime(definition Definition, from time.Time, clock string) (time.Time, error) {
	loc, err := time.LoadLocation(definition.Timezone)
	if err != nil {
		return time.Time{}, err
	}
	parts := strings.Split(clock, ":")
	if len(parts) != 2 {
		return time.Time{}, errors.New("localTime must be HH:MM")
	}
	hour, err := strconv.Atoi(parts[0])
	if err != nil {
		return time.Time{}, err
	}
	minute, err := strconv.Atoi(parts[1])
	if err != nil || hour > 23 || minute > 59 {
		return time.Time{}, errors.New("invalid localTime")
	}
	allowed := map[int]bool{}
	for _, day := range definition.Weekdays {
		allowed[day] = true
	}
	local := from.In(loc)
	for offset := 0; offset < 370; offset++ {
		day := local.AddDate(0, 0, offset)
		eligible := allowed[int(day.Weekday())]
		if override, exists := definition.Overrides[day.Format("2006-01-02")]; exists {
			eligible = override
		}
		at := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
		if eligible && (at.Year() != day.Year() || at.Month() != day.Month() || at.Day() != day.Day() || at.Hour() != hour || at.Minute() != minute) {
			return time.Time{}, ErrLocalTimeGap
		}
		if eligible && at.After(from) {
			return at.UTC(), nil
		}
	}
	return time.Time{}, ErrNoEligibleDate
}
func fail(err string) protocol.Reply { return protocol.Reply{Error: err} }
func ok(data any) protocol.Reply     { return protocol.Reply{OK: true, Data: data} }
