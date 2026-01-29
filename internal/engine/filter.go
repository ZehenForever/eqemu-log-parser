package engine

import "time"

type TimeFilter struct {
	Cutoff *time.Time // nil means no cutoff
}

func NewTimeFilterLastHours(lastHours float64, now time.Time) TimeFilter {
	if lastHours <= 0 {
		return TimeFilter{}
	}
	cutoff := now.Add(-time.Duration(lastHours * float64(time.Hour)))
	return TimeFilter{Cutoff: &cutoff}
}

func (f TimeFilter) Allow(ts time.Time) bool {
	if f.Cutoff == nil {
		return true
	}
	return !ts.Before(*f.Cutoff)
}
