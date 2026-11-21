package main

import "time"

// Slot is a bookable start time.
type Slot struct {
	Start int64  `json:"start"` // unix seconds
	Label string `json:"label"` // "14:30" in salon time zone
}

// SlotParams — pure input for slot calculation (easy to unit-test).
type SlotParams struct {
	Day         time.Time // any time inside the requested day, in the salon location
	Hours       []Hours   // master's weekly hours
	DurationMin int
	StepMin     int        // grid step, e.g. 30
	Busy        [][2]int64 // [start,end) unix intervals
	NotBefore   int64      // earliest allowed start (now + lead time)
}

// FreeSlots returns starts on a StepMin grid within working hours where [start, start+duration)
// fits entirely and does not overlap any busy interval.
func FreeSlots(p SlotParams) []Slot {
	out := []Slot{}
	if p.StepMin <= 0 {
		p.StepMin = 30
	}
	loc := p.Day.Location()
	y, m, d := p.Day.Date()
	midnight := time.Date(y, m, d, 0, 0, 0, 0, loc)
	wd := int(midnight.Weekday())
	var h *Hours
	for i := range p.Hours {
		if p.Hours[i].Weekday == wd {
			h = &p.Hours[i]
		}
	}
	if h == nil || p.DurationMin <= 0 {
		return out
	}
	for min := h.StartMin; min+p.DurationMin <= h.EndMin; min += p.StepMin {
		st := midnight.Add(time.Duration(min) * time.Minute) // DST-safe enough for salon hours
		start := st.Unix()
		end := start + int64(p.DurationMin)*60
		if start < p.NotBefore {
			continue
		}
		free := true
		for _, b := range p.Busy {
			if start < b[1] && end > b[0] {
				free = false
				break
			}
		}
		if free {
			out = append(out, Slot{Start: start, Label: st.Format("15:04")})
		}
	}
	return out
}

// DayRange returns [00:00, next 00:00) of the day containing t, as unix seconds.
func DayRange(t time.Time) (int64, int64) {
	y, m, d := t.Date()
	a := time.Date(y, m, d, 0, 0, 0, 0, t.Location())
	return a.Unix(), a.AddDate(0, 0, 1).Unix()
}
