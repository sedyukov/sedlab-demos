package main

import (
	_ "embed"
	"encoding/json"
	"fmt"
	"os"
	"sort"
)

//go:embed limits.json
var defaultLimits []byte

type Resource struct {
	Kind         string `json:"kind"`
	Unit         string `json:"unit"`
	PerUserDay   int64  `json:"per_user_day"`
	PerDayGlobal int64  `json:"per_day_global"`
	Monthly      int64  `json:"monthly"`
	MaxPerCall   int64  `json:"max_per_call,omitempty"`
	LabelRU      string `json:"label_ru"` // provider + country, shown in the widget hint (legal/ui-texts.md §4)
	LabelEN      string `json:"label_en"`
}

type Limits struct {
	Resources map[string]Resource `json:"resources"`
	Demos     map[string][]string `json:"demos"`
}

// LoadLimits: embedded limits.json, or AUTH_LIMITS_FILE when set.
func LoadLimits(path string) (*Limits, error) {
	b := defaultLimits
	if path != "" {
		var err error
		if b, err = os.ReadFile(path); err != nil {
			return nil, err
		}
	}
	var l Limits
	if err := json.Unmarshal(b, &l); err != nil {
		return nil, fmt.Errorf("limits: %w", err)
	}
	for d, rs := range l.Demos {
		for _, r := range rs {
			if _, ok := l.Resources[r]; !ok {
				return nil, fmt.Errorf("limits: demo %s uses unknown resource %s", d, r)
			}
		}
	}
	return &l, nil
}

// For returns the limit of `resource` for `demo`; ok=false when the demo doesn't use it.
func (l *Limits) For(demo, resource string) (Resource, bool) {
	for _, r := range l.Demos[demo] {
		if r == resource {
			return l.Resources[resource], true
		}
	}
	return Resource{}, false
}

func (l *Limits) DemoResources(demo string) []string {
	rs := append([]string(nil), l.Demos[demo]...)
	sort.Strings(rs)
	return rs
}

func (r Resource) Limit() Limit {
	return Limit{PerUserDay: r.PerUserDay, PerDayGlobal: r.PerDayGlobal, Monthly: r.Monthly}
}
