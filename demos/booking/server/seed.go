package main

import (
	"fmt"
	"hash/fnv"
	"math/rand"
	"time"
)

// Sample salon (fictional) — the demo starts populated so the admin schedule looks alive.
var seedServices = []Service{
	{NameRU: "Женская стрижка и укладка", NameEN: "Women's haircut & styling", DurationMin: 60, PriceRub: 250000, PriceStars: 1500, PriceUSD: 4500, Sort: 1},
	{NameRU: "Мужская стрижка", NameEN: "Men's haircut", DurationMin: 45, PriceRub: 150000, PriceStars: 900, PriceUSD: 3000, Sort: 2},
	{NameRU: "Маникюр с покрытием", NameEN: "Gel manicure", DurationMin: 90, PriceRub: 220000, PriceStars: 1300, PriceUSD: 4000, Sort: 3},
	{NameRU: "Педикюр", NameEN: "Pedicure", DurationMin: 90, PriceRub: 260000, PriceStars: 1550, PriceUSD: 4500, Sort: 4},
	{NameRU: "Оформление и окрашивание бровей", NameEN: "Brow shaping & tint", DurationMin: 45, PriceRub: 120000, PriceStars: 700, PriceUSD: 2500, Sort: 5},
	{NameRU: "Окрашивание волос", NameEN: "Hair colouring", DurationMin: 150, PriceRub: 650000, PriceStars: 3900, PriceUSD: 12000, Sort: 6},
}

func workWeek(days []int, from, to int) []Hours {
	out := []Hours{}
	for _, d := range days {
		out = append(out, Hours{Weekday: d, StartMin: from * 60, EndMin: to * 60})
	}
	return out
}

var seedMasters = []Master{
	{NameRU: "Анна", NameEN: "Anna", RoleRU: "Стилист-колорист", RoleEN: "Stylist & colourist", Color: "#e86a92", ServiceIDs: []int64{1, 2, 6}, Hours: workWeek([]int{1, 2, 3, 4, 5}, 10, 19)},
	{NameRU: "Мария", NameEN: "Maria", RoleRU: "Мастер ногтевого сервиса", RoleEN: "Nail artist", Color: "#7c5cff", ServiceIDs: []int64{3, 4}, Hours: workWeek([]int{2, 3, 4, 5, 6}, 10, 20)},
	{NameRU: "Елена", NameEN: "Elena", RoleRU: "Бровист, стилист", RoleEN: "Brow artist & stylist", Color: "#f4a340", ServiceIDs: []int64{1, 5}, Hours: workWeek([]int{1, 3, 5, 6, 0}, 11, 20)},
	{NameRU: "Дмитрий", NameEN: "Dmitry", RoleRU: "Барбер", RoleEN: "Barber", Color: "#2bb0a6", ServiceIDs: []int64{2}, Hours: workWeek([]int{0, 1, 2, 4, 5, 6}, 12, 21)},
}

// fictional clients, per edition
var seedClients = map[string][]string{
	"ru":   {"Ольга К.", "Ирина П.", "Наталья В.", "Екатерина Л.", "Алексей Н.", "Юлия С.", "Мария Д.", "Анна Г."},
	"intl": {"Sofia M.", "James W.", "Ana R.", "Lena T.", "Oliver B.", "Emma S.", "Noah K.", "Mia L."},
}

// ResetDemo wipes data and seeds the sample salon with bookings ±7 days around today.
func (a *App) ResetDemo() error {
	if err := a.store.Wipe(); err != nil {
		return err
	}
	if err := a.Seed(); err != nil {
		return err
	}
	return a.store.SetMeta("reset_day", a.now().In(a.cfg.TZ).Format("2006-01-02"))
}

func (a *App) Seed() error {
	for i := range seedServices {
		s := seedServices[i]
		s.ID, s.Active = int64(i+1), true
		if _, err := a.store.db.Exec(`INSERT INTO services(id,name_ru,name_en,duration_min,price_rub,price_stars,price_usd,active,sort) VALUES(?,?,?,?,?,?,?,1,?)`,
			s.ID, s.NameRU, s.NameEN, s.DurationMin, s.PriceRub, s.PriceStars, s.PriceUSD, s.Sort); err != nil {
			return err
		}
	}
	for i := range seedMasters {
		m := seedMasters[i]
		m.Active = true
		if err := a.store.SaveMaster(&m); err != nil {
			return err
		}
	}
	// deterministic per day so screenshots are reproducible
	today := a.now().In(a.cfg.TZ)
	h := fnv.New64a()
	fmt.Fprint(h, today.Format("2006-01-02"))
	rng := rand.New(rand.NewSource(int64(h.Sum64())))
	ms, err := a.store.Masters(true)
	if err != nil {
		return err
	}
	now := a.now().Unix()
	clients := seedClients[a.cfg.Edition]
	if len(clients) == 0 {
		clients = seedClients["intl"]
	}
	for d := -7; d <= 7; d++ {
		day := today.AddDate(0, 0, d)
		for _, m := range ms {
			// 1–3 bookings per master per day: the schedule looks alive and clients still find free time
			n := 1 + rng.Intn(3)
			from, to := DayRange(day)
			busy, _ := a.store.BusyIntervals(m.ID, from, to, now)
			for k := 0; k < n; k++ {
				sid := m.ServiceIDs[rng.Intn(len(m.ServiceIDs))]
				svc, _ := a.store.Service(sid)
				slots := FreeSlots(SlotParams{Day: day, Hours: m.Hours, DurationMin: svc.DurationMin, StepMin: 30, Busy: busy})
				if len(slots) == 0 {
					break
				}
				sl := slots[rng.Intn(len(slots))]
				b := &Booking{UserID: EmulatorUserBase + 500 + int64(rng.Intn(1000)), ClientName: clients[rng.Intn(len(clients))],
					ServiceID: sid, MasterID: m.ID, StartAt: sl.Start, EndAt: sl.Start + int64(svc.DurationMin)*60,
					Status: "confirmed", Lang: a.cfg.Lang, CreatedAt: sl.Start - int64(2+rng.Intn(10))*86400}
				if a.cfg.Edition == "intl" && rng.Intn(2) == 0 {
					b.PayMethod, b.PayCurrency, b.PayAmount = "stars", "XTR", a.deposit(svc.PriceStars, 1)
				} else {
					b.PayMethod, b.PayCurrency, b.PayAmount = "card", a.cardCurrency(), a.deposit(a.cardPrice(svc), 100)
				}
				b.PayID, b.PaidAt = fmt.Sprintf("seed_%d_%d", d+7, rng.Int63()), b.CreatedAt
				if b.EndAt <= now {
					b.Status = "completed"
				}
				if err := a.store.InsertBooking(b); err != nil {
					return err
				}
				busy = append(busy, [2]int64{b.StartAt, b.EndAt})
			}
		}
	}
	_, err = a.store.db.Exec(`UPDATE bookings SET reminded_24h=1, reminded_2h=1`)
	return err
}

var _ = time.Now
