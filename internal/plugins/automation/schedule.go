package automation

import (
	"errors"
	"strconv"
	"strings"
	"time"
	_ "time/tzdata"
)

const maxCronSearchMinutes = 5 * 366 * 24 * 60

type cronField struct {
	allowed map[int]bool
	any     bool
}

func parseCronNumber(raw string, min, max int, weekday bool) (int, error) {
	n, err := strconv.Atoi(raw)
	if err != nil || n < min || n > max {
		return 0, errors.New("invalid cron field")
	}
	if weekday && n == 7 {
		return 0, nil
	}
	return n, nil
}

func parseCronField(raw string, min, max int, weekday bool) (cronField, error) {
	f := cronField{allowed: map[int]bool{}, any: raw == "*"}
	if raw == "" {
		return f, errors.New("invalid cron field")
	}
	for _, part := range strings.Split(raw, ",") {
		if part == "" {
			return f, errors.New("invalid cron field")
		}
		base, step := part, 1
		hasStep := strings.Contains(part, "/")
		if hasStep {
			pieces := strings.Split(part, "/")
			if len(pieces) != 2 {
				return f, errors.New("invalid cron step")
			}
			base = pieces[0]
			var err error
			step, err = strconv.Atoi(pieces[1])
			if err != nil || step < 1 || step > max-min+1 {
				return f, errors.New("invalid cron step")
			}
		}
		start, end := min, max
		switch {
		case base == "*":
		case strings.Contains(base, "-"):
			pieces := strings.Split(base, "-")
			if len(pieces) != 2 {
				return f, errors.New("invalid cron range")
			}
			var err error
			start, err = parseCronNumber(pieces[0], min, max, weekday)
			if err != nil {
				return f, err
			}
			endRaw, err := strconv.Atoi(pieces[1])
			if err != nil || endRaw < min || endRaw > max {
				return f, errors.New("invalid cron range")
			}
			end = endRaw
			if weekday && start == 0 && pieces[0] == "7" {
				start = 7
			}
			if start > end {
				return f, errors.New("invalid cron range")
			}
		default:
			n, err := parseCronNumber(base, min, max, weekday)
			if err != nil {
				return f, err
			}
			if !hasStep {
				f.allowed[n] = true
				continue
			}
			start = n
			if weekday && n == 0 && base == "7" {
				start = 7
			}
		}
		for n := start; n <= end; n += step {
			v := n
			if weekday && v == 7 {
				v = 0
			}
			f.allowed[v] = true
		}
	}
	if len(f.allowed) == 0 {
		return f, errors.New("empty cron field")
	}
	return f, nil
}

type cronSpec struct {
	minute   cronField
	hour     cronField
	day      cronField
	month    cronField
	weekday  cronField
	location *time.Location
}

func parseCron(expr, timezone string) (cronSpec, error) {
	var out cronSpec
	parts := strings.Fields(expr)
	if len(parts) != 5 || len(expr) > 128 {
		return out, errors.New("cron must have five fields")
	}
	if timezone == "" {
		timezone = "UTC"
	}
	if len(timezone) > 64 {
		return out, errors.New("timezone too long")
	}
	loc, err := time.LoadLocation(timezone)
	if err != nil {
		return out, errors.New("invalid timezone")
	}
	out.location = loc
	if out.minute, err = parseCronField(parts[0], 0, 59, false); err != nil {
		return out, err
	}
	if out.hour, err = parseCronField(parts[1], 0, 23, false); err != nil {
		return out, err
	}
	if out.day, err = parseCronField(parts[2], 1, 31, false); err != nil {
		return out, err
	}
	if out.month, err = parseCronField(parts[3], 1, 12, false); err != nil {
		return out, err
	}
	if out.weekday, err = parseCronField(parts[4], 0, 7, true); err != nil {
		return out, err
	}
	return out, nil
}

func (s cronSpec) match(t time.Time) bool {
	local := t.In(s.location)
	if !s.minute.allowed[local.Minute()] || !s.hour.allowed[local.Hour()] || !s.month.allowed[int(local.Month())] {
		return false
	}
	dayMatch := s.day.allowed[local.Day()]
	weekdayMatch := s.weekday.allowed[int(local.Weekday())]
	switch {
	case !s.day.any && !s.weekday.any:
		return dayMatch || weekdayMatch
	case !s.day.any:
		return dayMatch
	case !s.weekday.any:
		return weekdayMatch
	default:
		return true
	}
}

func ValidCron(expr, timezone string) bool {
	_, err := parseCron(expr, timezone)
	return err == nil
}

func NextCron(expr, timezone string, after time.Time) (time.Time, error) {
	spec, err := parseCron(expr, timezone)
	if err != nil {
		return time.Time{}, err
	}
	candidate := after.UTC().Truncate(time.Minute)
	for i := 0; i < maxCronSearchMinutes; i++ {
		candidate = candidate.Add(time.Minute)
		if spec.match(candidate) {
			return candidate.UTC(), nil
		}
	}
	return time.Time{}, errors.New("cron has no occurrence within search horizon")
}
