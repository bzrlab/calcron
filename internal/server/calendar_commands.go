package server

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"
	"time"
)

type calendarDefinition struct {
	Timezone  string          `json:"timezone"`
	Weekdays  []int           `json:"weekdays"`
	Overrides map[string]bool `json:"overrides"`
}

func (s *Server) setCalendar(ctx context.Context, f frame) reply {
	if f.ApplicationID == "" || f.Name == "" {
		return fail("applicationId and name required")
	}
	if _, err := parseCalendar(f.Data); err != nil {
		return fail(err.Error())
	}
	if _, err := s.db.Exec(ctx, `insert into calendars(application_id,name,definition) values($1,$2,$3) on conflict(application_id,name) do update set definition=excluded.definition,updated_at=now()`, f.ApplicationID, f.Name, f.Data); err != nil {
		return fail(err.Error())
	}
	return ok(nil)
}

func parseCalendar(raw []byte) (calendarDefinition, error) {
	var definition calendarDefinition
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

func (s *Server) loadCalendar(ctx context.Context, app, name string) (calendarDefinition, error) {
	var raw []byte
	var definition calendarDefinition
	if s.db.QueryRow(ctx, `select definition from calendars where application_id=$1 and name=$2`, app, name).Scan(&raw) != nil {
		return definition, errors.New("calendar not found")
	}
	return definition, json.Unmarshal(raw, &definition)
}

func (s *Server) calendarOccurrences(ctx context.Context, f frame) reply {
	if f.LocalTime == "" || f.At == "" || f.Until == "" {
		return fail("localTime, at and until required")
	}
	var definition calendarDefinition
	var err error
	switch {
	case f.Calendar != "" && f.ApplicationID == "":
		return fail("applicationId required with calendar")
	case f.Calendar != "":
		definition, err = s.loadCalendar(ctx, f.ApplicationID, f.Calendar)
	case len(f.Data) > 0:
		definition, err = parseCalendar(f.Data)
	default:
		return fail("calendar or data required")
	}
	if err != nil {
		return fail(err.Error())
	}
	loc, _ := time.LoadLocation(definition.Timezone)
	from, fromErr := time.Parse(time.RFC3339, f.At)
	until, untilErr := time.Parse(time.RFC3339, f.Until)
	if fromErr != nil || untilErr != nil {
		return fail("invalid at or until")
	}
	if !until.After(from) || until.Sub(from) > 366*24*time.Hour {
		return fail("until must be after at and within 366 days")
	}
	occurrences := []string{}
	for from = from.Add(-time.Nanosecond); ; {
		next, nextErr := nextCalendar(definition, from, f.LocalTime)
		if errors.Is(nextErr, errLocalTimeGap) {
			local := from.In(loc)
			from = time.Date(local.Year(), local.Month(), local.Day()+1, 0, 0, 0, 0, loc)
			if !from.Before(until) {
				break
			}
			continue
		}
		if errors.Is(nextErr, errNoEligibleDate) {
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

var (
	errLocalTimeGap   = errors.New("localTime does not exist on calendar date")
	errNoEligibleDate = errors.New("no eligible calendar date")
)

func nextCalendar(definition calendarDefinition, from time.Time, clock string) (time.Time, error) {
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
		date := day.Format("2006-01-02")
		eligible := allowed[int(day.Weekday())]
		if override, exists := definition.Overrides[date]; exists {
			eligible = override
		}
		at := time.Date(day.Year(), day.Month(), day.Day(), hour, minute, 0, 0, loc)
		if eligible && (at.Year() != day.Year() || at.Month() != day.Month() || at.Day() != day.Day() || at.Hour() != hour || at.Minute() != minute) {
			return time.Time{}, errLocalTimeGap
		}
		if eligible && at.After(from) {
			return at.UTC(), nil
		}
	}
	return time.Time{}, errNoEligibleDate
}

func (s *Server) nextCalendar(ctx context.Context, f frame) reply {
	if f.ApplicationID == "" || f.Calendar == "" || f.LocalTime == "" {
		return fail("applicationId, calendar and localTime required")
	}
	definition, err := s.loadCalendar(ctx, f.ApplicationID, f.Calendar)
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
	next, err := nextCalendar(definition, from, f.LocalTime)
	if err != nil {
		return fail(err.Error())
	}
	return ok(map[string]any{"nextAt": next.Format(time.RFC3339)})
}
