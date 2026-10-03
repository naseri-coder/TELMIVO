package policies

import (
	"math"
	"testing"
	"time"
)

func TestMergeStrategies(t *testing.T) {
	for _, x := range []struct {
		s    string
		want int64
	}{{"ADD", 70}, {"MAX", 50}, {"MIN", 20}, {"REPLACE", 50}} {
		n, e := Merge(20, 50, x.s)
		if e != nil || n != x.want {
			t.Fatalf("%s: %d %v", x.s, n, e)
		}
	}
	if _, e := Merge(math.MaxInt64, 1, "ADD"); e == nil {
		t.Fatal("overflow")
	}
	if _, e := Merge(1, 2, "MAGIC"); e == nil {
		t.Fatal("implicit merge")
	}
}
func TestCalendarWindows(t *testing.T) {
	now := time.Date(2026, 10, 4, 22, 0, 0, 0, time.UTC)
	d := Definition{Window: "CALENDAR_DAY", Timezone: "Asia/Tehran"}
	b, end, e := Bucket(d, now)
	if e != nil || end.Sub(now) != 22*time.Hour+30*time.Minute || b == "" {
		t.Fatalf("timezone boundary %s %v %v", b, end, e)
	}
	d.Window = "CALENDAR_WEEK"
	_, end, e = Bucket(d, now)
	if e != nil || end.In(time.FixedZone("Tehran", 12600)).Weekday() != time.Monday {
		t.Fatal("week")
	}
	d.Window = "CALENDAR_MONTH"
	_, end, e = Bucket(d, now)
	if e != nil || end.In(time.FixedZone("Tehran", 12600)).Day() != 1 {
		t.Fatal("month")
	}
	d.Window = "NONE"
	b, end, e = Bucket(d, now)
	if b != "NONE" || end != nil || e != nil {
		t.Fatal("capacity resets")
	}
}
func TestPolicyValidation(t *testing.T) {
	v := Version{Key: "quota", Kind: "QUOTA", Feature: "downloads", EffectiveFrom: time.Now(), Definition: Definition{Metric: "download.jobs", Unit: "JOBS", Window: "CALENDAR_DAY", Timezone: "UTC", Mode: "USAGE", Limit: 5, ReservationSeconds: 60}}
	if e := Validate(v); e != nil {
		t.Fatal(e)
	}
	v.Definition.Mode = "CAPACITY"
	if Validate(v) == nil {
		t.Fatal("capacity must not reset")
	}
	v.Definition.Mode = "USAGE"
	v.Definition.Timezone = "arbitrary"
	if Validate(v) == nil {
		t.Fatal("timezone")
	}
	v.Kind = "PRICE"
	v.Definition = Definition{Asset: "USD", AmountAtomic: "1.5", Scale: 2}
	if Validate(v) == nil {
		t.Fatal("float money")
	}
}
func TestReservationConfiguration(t *testing.T) {
	if reservable(nil, 1, nil) {
		t.Fatal("unconfigured job admitted")
	}
	if reservable(map[string]Resource{"file": {Mode: "ITEM"}}, 1, map[string]int64{"file": 100}) {
		t.Fatal("size check alone is not an allocation")
	}
	if !reservable(map[string]Resource{"jobs": {Mode: "USAGE", Primary: true}}, 1, nil) {
		t.Fatal("usage quota")
	}
	if !reservable(map[string]Resource{"jobs": {Mode: "CONCURRENT"}}, 1, nil) {
		t.Fatal("concurrency quota")
	}
	if !reservable(map[string]Resource{"storage": {Mode: "CAPACITY"}}, 0, map[string]int64{"storage": 100}) {
		t.Fatal("capacity allocation")
	}
}
