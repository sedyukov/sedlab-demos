package main

import (
	"testing"
	"time"
)

func TestFreeSlots(t *testing.T) {
	loc, _ := time.LoadLocation("Europe/Moscow")
	day := time.Date(2026, 11, 23, 0, 0, 0, 0, loc) // Monday
	at := func(h, m int) int64 { return time.Date(2026, 11, 23, h, m, 0, 0, loc).Unix() }
	hours := []Hours{{Weekday: 1, StartMin: 10 * 60, EndMin: 13 * 60}}

	got := FreeSlots(SlotParams{Day: day, Hours: hours, DurationMin: 60, StepMin: 30})
	want := []string{"10:00", "10:30", "11:00", "11:30", "12:00"}
	if len(got) != len(want) {
		t.Fatalf("got %v", got)
	}
	for i := range want {
		if got[i].Label != want[i] {
			t.Fatalf("slot %d = %s, want %s", i, got[i].Label, want[i])
		}
	}
	// busy 11:00–12:00 blocks starts 10:30, 11:00, 11:30
	got = FreeSlots(SlotParams{Day: day, Hours: hours, DurationMin: 60, StepMin: 30, Busy: [][2]int64{{at(11, 0), at(12, 0)}}})
	if len(got) != 2 || got[0].Label != "10:00" || got[1].Label != "12:00" {
		t.Fatalf("with busy: %v", got)
	}
	// lead time
	got = FreeSlots(SlotParams{Day: day, Hours: hours, DurationMin: 60, StepMin: 30, NotBefore: at(11, 15)})
	if len(got) != 2 || got[0].Label != "11:30" {
		t.Fatalf("not before: %v", got)
	}
	// day off
	if got := FreeSlots(SlotParams{Day: day.AddDate(0, 0, 1), Hours: hours, DurationMin: 60}); len(got) != 0 {
		t.Fatalf("day off: %v", got)
	}
	// service longer than the shift
	if got := FreeSlots(SlotParams{Day: day, Hours: hours, DurationMin: 240}); len(got) != 0 {
		t.Fatalf("too long: %v", got)
	}
}

func TestDeposit(t *testing.T) {
	a := &App{cfg: Config{DepositPercent: 30}}
	if d := a.deposit(250000, 100); d != 75000 {
		t.Fatal(d)
	}
	if d := a.deposit(150050, 100); d != 45100 { // rounds up to whole rubles
		t.Fatal(d)
	}
	if d := a.deposit(1550, 1); d != 465 {
		t.Fatal(d)
	}
}
