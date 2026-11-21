package main

import (
	"database/sql"
	"errors"
	"fmt"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

// Store is a thin SQLite layer (pure Go driver, no CGO). One file in /data, WAL mode.
type Store struct{ db *sql.DB }

const schema = `
CREATE TABLE IF NOT EXISTS services (
  id INTEGER PRIMARY KEY,
  name_ru TEXT NOT NULL, name_en TEXT NOT NULL,
  duration_min INTEGER NOT NULL,
  price_rub INTEGER NOT NULL,      -- kopecks
  price_stars INTEGER NOT NULL,    -- XTR
  active INTEGER NOT NULL DEFAULT 1,
  sort INTEGER NOT NULL DEFAULT 0
);
CREATE TABLE IF NOT EXISTS masters (
  id INTEGER PRIMARY KEY,
  name_ru TEXT NOT NULL, name_en TEXT NOT NULL,
  role_ru TEXT NOT NULL DEFAULT '', role_en TEXT NOT NULL DEFAULT '',
  color TEXT NOT NULL DEFAULT '#7c5cff',
  active INTEGER NOT NULL DEFAULT 1
);
CREATE TABLE IF NOT EXISTS master_services (
  master_id INTEGER NOT NULL REFERENCES masters(id) ON DELETE CASCADE,
  service_id INTEGER NOT NULL REFERENCES services(id) ON DELETE CASCADE,
  PRIMARY KEY (master_id, service_id)
);
CREATE TABLE IF NOT EXISTS hours (
  master_id INTEGER NOT NULL REFERENCES masters(id) ON DELETE CASCADE,
  weekday INTEGER NOT NULL,        -- 0 = Sunday … 6 = Saturday
  start_min INTEGER NOT NULL, end_min INTEGER NOT NULL,
  PRIMARY KEY (master_id, weekday)
);
CREATE TABLE IF NOT EXISTS users (
  id INTEGER PRIMARY KEY,          -- Telegram user id
  first_name TEXT NOT NULL DEFAULT '', username TEXT NOT NULL DEFAULT '',
  lang TEXT NOT NULL DEFAULT 'en',
  created_at INTEGER NOT NULL
);
CREATE TABLE IF NOT EXISTS bookings (
  id INTEGER PRIMARY KEY,
  user_id INTEGER NOT NULL,
  client_name TEXT NOT NULL DEFAULT '',
  client_phone TEXT NOT NULL DEFAULT '',
  service_id INTEGER NOT NULL, master_id INTEGER NOT NULL,
  start_at INTEGER NOT NULL, end_at INTEGER NOT NULL,
  status TEXT NOT NULL,            -- pending | confirmed | cancelled | completed
  pay_method TEXT NOT NULL DEFAULT '',   -- yookassa | stars | ''
  pay_currency TEXT NOT NULL DEFAULT '',
  pay_amount INTEGER NOT NULL DEFAULT 0, -- minor units (kopecks) or stars
  pay_id TEXT NOT NULL DEFAULT '',       -- payments id or telegram_payment_charge_id
  paid_at INTEGER NOT NULL DEFAULT 0,
  refunded INTEGER NOT NULL DEFAULT 0,
  hold_until INTEGER NOT NULL DEFAULT 0, -- pending slot hold expiry
  reminded_24h INTEGER NOT NULL DEFAULT 0, reminded_2h INTEGER NOT NULL DEFAULT 0,
  lang TEXT NOT NULL DEFAULT 'en',
  created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS bookings_master_time ON bookings(master_id, start_at);
CREATE INDEX IF NOT EXISTS bookings_user ON bookings(user_id);
CREATE TABLE IF NOT EXISTS messages (       -- outgoing bot messages (shown in the emulator chat)
  id INTEGER PRIMARY KEY,
  chat_id INTEGER NOT NULL, text TEXT NOT NULL, button TEXT NOT NULL DEFAULT '',
  kind TEXT NOT NULL DEFAULT '', created_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS messages_chat ON messages(chat_id, id);
CREATE TABLE IF NOT EXISTS meta (k TEXT PRIMARY KEY, v TEXT NOT NULL);
`

func OpenStore(path string) (*Store, error) {
	dsn := path + "?_pragma=busy_timeout(5000)&_pragma=journal_mode(WAL)&_pragma=foreign_keys(1)"
	db, err := sql.Open("sqlite", dsn)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1) // SQLite: one writer; keeps memory tiny
	if _, err := db.Exec(schema); err != nil {
		return nil, fmt.Errorf("schema: %w", err)
	}
	return &Store{db: db}, nil
}

func (s *Store) Close() error { return s.db.Close() }

var ErrNotFound = errors.New("not found")

// ---- types ----

type Service struct {
	ID          int64  `json:"id"`
	NameRU      string `json:"name_ru"`
	NameEN      string `json:"name_en"`
	DurationMin int    `json:"duration_min"`
	PriceRub    int64  `json:"price_rub"` // kopecks
	PriceStars  int64  `json:"price_stars"`
	Active      bool   `json:"active"`
	Sort        int    `json:"sort"`
}

func (v Service) Name(lang string) string {
	if lang == "ru" {
		return v.NameRU
	}
	return v.NameEN
}

type Hours struct {
	Weekday  int `json:"weekday"`
	StartMin int `json:"start_min"`
	EndMin   int `json:"end_min"`
}

type Master struct {
	ID         int64   `json:"id"`
	NameRU     string  `json:"name_ru"`
	NameEN     string  `json:"name_en"`
	RoleRU     string  `json:"role_ru"`
	RoleEN     string  `json:"role_en"`
	Color      string  `json:"color"`
	Active     bool    `json:"active"`
	ServiceIDs []int64 `json:"service_ids"`
	Hours      []Hours `json:"hours"`
}

func (m Master) Name(lang string) string {
	if lang == "ru" {
		return m.NameRU
	}
	return m.NameEN
}

type Booking struct {
	ID          int64  `json:"id"`
	UserID      int64  `json:"user_id"`
	ClientName  string `json:"client_name"`
	ClientPhone string `json:"client_phone"`
	ServiceID   int64  `json:"service_id"`
	MasterID    int64  `json:"master_id"`
	StartAt     int64  `json:"start_at"`
	EndAt       int64  `json:"end_at"`
	Status      string `json:"status"`
	PayMethod   string `json:"pay_method"`
	PayCurrency string `json:"pay_currency"`
	PayAmount   int64  `json:"pay_amount"`
	PayID       string `json:"pay_id"`
	PaidAt      int64  `json:"paid_at"`
	Refunded    bool   `json:"refunded"`
	HoldUntil   int64  `json:"hold_until"`
	Reminded24h bool   `json:"reminded_24h"`
	Reminded2h  bool   `json:"reminded_2h"`
	Lang        string `json:"lang"`
	CreatedAt   int64  `json:"created_at"`
}

type Message struct {
	ID        int64  `json:"id"`
	ChatID    int64  `json:"chat_id"`
	Text      string `json:"text"`
	Button    string `json:"button,omitempty"`
	Kind      string `json:"kind,omitempty"`
	CreatedAt int64  `json:"created_at"`
}

// ---- services ----

func (s *Store) Services(onlyActive bool) ([]Service, error) {
	q := `SELECT id,name_ru,name_en,duration_min,price_rub,price_stars,active,sort FROM services`
	if onlyActive {
		q += ` WHERE active=1`
	}
	rows, err := s.db.Query(q + ` ORDER BY sort, id`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Service{}
	for rows.Next() {
		var v Service
		if err := rows.Scan(&v.ID, &v.NameRU, &v.NameEN, &v.DurationMin, &v.PriceRub, &v.PriceStars, &v.Active, &v.Sort); err != nil {
			return nil, err
		}
		out = append(out, v)
	}
	return out, rows.Err()
}

func (s *Store) Service(id int64) (Service, error) {
	var v Service
	err := s.db.QueryRow(`SELECT id,name_ru,name_en,duration_min,price_rub,price_stars,active,sort FROM services WHERE id=?`, id).
		Scan(&v.ID, &v.NameRU, &v.NameEN, &v.DurationMin, &v.PriceRub, &v.PriceStars, &v.Active, &v.Sort)
	if errors.Is(err, sql.ErrNoRows) {
		return v, ErrNotFound
	}
	return v, err
}

func (s *Store) SaveService(v *Service) error {
	if v.ID == 0 {
		res, err := s.db.Exec(`INSERT INTO services(name_ru,name_en,duration_min,price_rub,price_stars,active,sort) VALUES(?,?,?,?,?,?,?)`,
			v.NameRU, v.NameEN, v.DurationMin, v.PriceRub, v.PriceStars, v.Active, v.Sort)
		if err != nil {
			return err
		}
		v.ID, _ = res.LastInsertId()
		return nil
	}
	res, err := s.db.Exec(`UPDATE services SET name_ru=?,name_en=?,duration_min=?,price_rub=?,price_stars=?,active=?,sort=? WHERE id=?`,
		v.NameRU, v.NameEN, v.DurationMin, v.PriceRub, v.PriceStars, v.Active, v.Sort, v.ID)
	return mustAffect(res, err)
}

// ---- masters ----

func (s *Store) Masters(onlyActive bool) ([]Master, error) {
	q := `SELECT id,name_ru,name_en,role_ru,role_en,color,active FROM masters`
	if onlyActive {
		q += ` WHERE active=1`
	}
	rows, err := s.db.Query(q + ` ORDER BY id`)
	if err != nil {
		return nil, err
	}
	out := []Master{}
	for rows.Next() {
		var m Master
		if err := rows.Scan(&m.ID, &m.NameRU, &m.NameEN, &m.RoleRU, &m.RoleEN, &m.Color, &m.Active); err != nil {
			rows.Close()
			return nil, err
		}
		out = append(out, m)
	}
	rows.Close()
	for i := range out {
		if err := s.fillMaster(&out[i]); err != nil {
			return nil, err
		}
	}
	return out, nil
}

func (s *Store) fillMaster(m *Master) error {
	m.ServiceIDs = []int64{}
	m.Hours = []Hours{}
	rows, err := s.db.Query(`SELECT service_id FROM master_services WHERE master_id=? ORDER BY service_id`, m.ID)
	if err != nil {
		return err
	}
	for rows.Next() {
		var id int64
		rows.Scan(&id)
		m.ServiceIDs = append(m.ServiceIDs, id)
	}
	rows.Close()
	rows, err = s.db.Query(`SELECT weekday,start_min,end_min FROM hours WHERE master_id=? ORDER BY weekday`, m.ID)
	if err != nil {
		return err
	}
	defer rows.Close()
	for rows.Next() {
		var h Hours
		rows.Scan(&h.Weekday, &h.StartMin, &h.EndMin)
		m.Hours = append(m.Hours, h)
	}
	return rows.Err()
}

func (s *Store) Master(id int64) (Master, error) {
	var m Master
	err := s.db.QueryRow(`SELECT id,name_ru,name_en,role_ru,role_en,color,active FROM masters WHERE id=?`, id).
		Scan(&m.ID, &m.NameRU, &m.NameEN, &m.RoleRU, &m.RoleEN, &m.Color, &m.Active)
	if errors.Is(err, sql.ErrNoRows) {
		return m, ErrNotFound
	}
	if err != nil {
		return m, err
	}
	return m, s.fillMaster(&m)
}

func (s *Store) SaveMaster(m *Master) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	if m.ID == 0 {
		res, err := tx.Exec(`INSERT INTO masters(name_ru,name_en,role_ru,role_en,color,active) VALUES(?,?,?,?,?,?)`,
			m.NameRU, m.NameEN, m.RoleRU, m.RoleEN, m.Color, m.Active)
		if err != nil {
			return err
		}
		m.ID, _ = res.LastInsertId()
	} else {
		res, err := tx.Exec(`UPDATE masters SET name_ru=?,name_en=?,role_ru=?,role_en=?,color=?,active=? WHERE id=?`,
			m.NameRU, m.NameEN, m.RoleRU, m.RoleEN, m.Color, m.Active, m.ID)
		if err := mustAffect(res, err); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM master_services WHERE master_id=?`, m.ID); err != nil {
		return err
	}
	for _, sid := range m.ServiceIDs {
		if _, err := tx.Exec(`INSERT OR IGNORE INTO master_services(master_id,service_id) VALUES(?,?)`, m.ID, sid); err != nil {
			return err
		}
	}
	if _, err := tx.Exec(`DELETE FROM hours WHERE master_id=?`, m.ID); err != nil {
		return err
	}
	for _, h := range m.Hours {
		if _, err := tx.Exec(`INSERT OR REPLACE INTO hours(master_id,weekday,start_min,end_min) VALUES(?,?,?,?)`, m.ID, h.Weekday, h.StartMin, h.EndMin); err != nil {
			return err
		}
	}
	return tx.Commit()
}

// ---- users ----

func (s *Store) UpsertUser(id int64, firstName, username, lang string) error {
	_, err := s.db.Exec(`INSERT INTO users(id,first_name,username,lang,created_at) VALUES(?,?,?,?,?)
		ON CONFLICT(id) DO UPDATE SET first_name=excluded.first_name, username=excluded.username, lang=excluded.lang`,
		id, firstName, username, lang, time.Now().Unix())
	return err
}

func (s *Store) UserLang(id int64) string {
	var l string
	if s.db.QueryRow(`SELECT lang FROM users WHERE id=?`, id).Scan(&l) != nil || l == "" {
		return "en"
	}
	return l
}

func (s *Store) SetUserLang(id int64, lang string) error {
	_, err := s.db.Exec(`INSERT INTO users(id,lang,created_at) VALUES(?,?,?) ON CONFLICT(id) DO UPDATE SET lang=excluded.lang`, id, lang, time.Now().Unix())
	return err
}

// ---- bookings ----

const bookingCols = `id,user_id,client_name,client_phone,service_id,master_id,start_at,end_at,status,pay_method,pay_currency,pay_amount,pay_id,paid_at,refunded,hold_until,reminded_24h,reminded_2h,lang,created_at`

func scanBooking(sc interface{ Scan(...any) error }) (Booking, error) {
	var b Booking
	err := sc.Scan(&b.ID, &b.UserID, &b.ClientName, &b.ClientPhone, &b.ServiceID, &b.MasterID, &b.StartAt, &b.EndAt, &b.Status,
		&b.PayMethod, &b.PayCurrency, &b.PayAmount, &b.PayID, &b.PaidAt, &b.Refunded, &b.HoldUntil, &b.Reminded24h, &b.Reminded2h, &b.Lang, &b.CreatedAt)
	return b, err
}

func (s *Store) queryBookings(where string, args ...any) ([]Booking, error) {
	rows, err := s.db.Query(`SELECT `+bookingCols+` FROM bookings WHERE `+where, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Booking{}
	for rows.Next() {
		b, err := scanBooking(rows)
		if err != nil {
			return nil, err
		}
		out = append(out, b)
	}
	return out, rows.Err()
}

func (s *Store) Booking(id int64) (Booking, error) {
	b, err := scanBooking(s.db.QueryRow(`SELECT `+bookingCols+` FROM bookings WHERE id=?`, id))
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}

func (s *Store) BookingByPayID(payID string) (Booking, error) {
	b, err := scanBooking(s.db.QueryRow(`SELECT `+bookingCols+` FROM bookings WHERE pay_id=? AND pay_id<>''`, payID))
	if errors.Is(err, sql.ErrNoRows) {
		return b, ErrNotFound
	}
	return b, err
}

// BusyIntervals returns [start,end) of bookings that block a master's time in [from,to):
// confirmed/completed, plus pending ones whose hold has not expired.
func (s *Store) BusyIntervals(masterID, from, to, now int64) ([][2]int64, error) {
	rows, err := s.db.Query(`SELECT start_at,end_at FROM bookings WHERE master_id=? AND start_at<? AND end_at>?
		AND (status IN ('confirmed','completed') OR (status='pending' AND hold_until>?)) ORDER BY start_at`, masterID, to, from, now)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out [][2]int64
	for rows.Next() {
		var a, b int64
		rows.Scan(&a, &b)
		out = append(out, [2]int64{a, b})
	}
	return out, rows.Err()
}

var ErrSlotTaken = errors.New("slot taken")

// CreatePending atomically checks the slot and inserts a pending booking with a hold.
func (s *Store) CreatePending(b *Booking, now int64) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var n int
	if err := tx.QueryRow(`SELECT COUNT(*) FROM bookings WHERE master_id=? AND start_at<? AND end_at>?
		AND (status IN ('confirmed','completed') OR (status='pending' AND hold_until>?))`, b.MasterID, b.EndAt, b.StartAt, now).Scan(&n); err != nil {
		return err
	}
	if n > 0 {
		return ErrSlotTaken
	}
	b.Status, b.CreatedAt = "pending", now
	res, err := tx.Exec(`INSERT INTO bookings(user_id,client_name,client_phone,service_id,master_id,start_at,end_at,status,hold_until,lang,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?)`, b.UserID, b.ClientName, b.ClientPhone, b.ServiceID, b.MasterID, b.StartAt, b.EndAt, b.Status, b.HoldUntil, b.Lang, now)
	if err != nil {
		return err
	}
	b.ID, _ = res.LastInsertId()
	return tx.Commit()
}

// InsertBooking is used by the seeder (no checks).
func (s *Store) InsertBooking(b *Booking) error {
	res, err := s.db.Exec(`INSERT INTO bookings(user_id,client_name,client_phone,service_id,master_id,start_at,end_at,status,pay_method,pay_currency,pay_amount,pay_id,paid_at,lang,created_at)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,?)`, b.UserID, b.ClientName, b.ClientPhone, b.ServiceID, b.MasterID, b.StartAt, b.EndAt, b.Status,
		b.PayMethod, b.PayCurrency, b.PayAmount, b.PayID, b.PaidAt, b.Lang, b.CreatedAt)
	if err != nil {
		return err
	}
	b.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) SetPaymentStarted(id int64, method, currency string, amount int64, payID string, holdUntil int64) error {
	res, err := s.db.Exec(`UPDATE bookings SET pay_method=?,pay_currency=?,pay_amount=?,pay_id=?,hold_until=? WHERE id=? AND status='pending'`,
		method, currency, amount, payID, holdUntil, id)
	return mustAffect(res, err)
}

// Confirm marks a pending booking paid. Returns false if it was not pending (already confirmed → idempotent no-op).
func (s *Store) Confirm(id int64, payID string, paidAt int64) (bool, error) {
	res, err := s.db.Exec(`UPDATE bookings SET status='confirmed', pay_id=CASE WHEN ?<>'' THEN ? ELSE pay_id END, paid_at=? WHERE id=? AND status='pending'`,
		payID, payID, paidAt, id)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) SetStatus(id int64, from []string, to string) (bool, error) {
	q := `UPDATE bookings SET status=? WHERE id=? AND status IN (` + strings.TrimSuffix(strings.Repeat("?,", len(from)), ",") + `)`
	args := []any{to, id}
	for _, f := range from {
		args = append(args, f)
	}
	res, err := s.db.Exec(q, args...)
	if err != nil {
		return false, err
	}
	n, _ := res.RowsAffected()
	return n > 0, nil
}

func (s *Store) MarkRefunded(id int64) error {
	_, err := s.db.Exec(`UPDATE bookings SET refunded=1 WHERE id=?`, id)
	return err
}

func (s *Store) UserBookings(userID int64) ([]Booking, error) {
	return s.queryBookings(`user_id=? AND status IN ('pending','confirmed','completed','cancelled') ORDER BY start_at DESC LIMIT 50`, userID)
}

func (s *Store) BookingsBetween(from, to int64) ([]Booking, error) {
	return s.queryBookings(`start_at>=? AND start_at<? AND status<>'expired' ORDER BY start_at`, from, to)
}

// DueReminders: confirmed bookings starting within `ahead` seconds that were not reminded for that window yet.
func (s *Store) DueReminders(now int64, window string) ([]Booking, error) {
	switch window {
	case "24h":
		return s.queryBookings(`status='confirmed' AND reminded_24h=0 AND start_at>? AND start_at<=? AND start_at-created_at>?`, now+2*3600, now+24*3600, 24*3600)
	case "2h":
		return s.queryBookings(`status='confirmed' AND reminded_2h=0 AND start_at>? AND start_at<=?`, now, now+2*3600)
	}
	return nil, fmt.Errorf("bad window %q", window)
}

func (s *Store) MarkReminded(id int64, window string) error {
	col := map[string]string{"24h": "reminded_24h", "2h": "reminded_2h"}[window]
	if col == "" {
		return fmt.Errorf("bad window %q", window)
	}
	_, err := s.db.Exec(`UPDATE bookings SET `+col+`=1 WHERE id=?`, id)
	return err
}

// ExpireHolds turns pending bookings with an expired hold into "expired" (frees the slot, hidden from lists).
func (s *Store) ExpireHolds(now int64) (int64, error) {
	res, err := s.db.Exec(`UPDATE bookings SET status='expired' WHERE status='pending' AND hold_until<=?`, now)
	if err != nil {
		return 0, err
	}
	return res.RowsAffected()
}

// CompletePast marks confirmed bookings that ended as completed.
func (s *Store) CompletePast(now int64) error {
	_, err := s.db.Exec(`UPDATE bookings SET status='completed' WHERE status='confirmed' AND end_at<=?`, now)
	return err
}

// ---- messages (bot outbox, for the emulator chat) ----

func (s *Store) AddMessage(m *Message) error {
	m.CreatedAt = time.Now().Unix()
	res, err := s.db.Exec(`INSERT INTO messages(chat_id,text,button,kind,created_at) VALUES(?,?,?,?,?)`, m.ChatID, m.Text, m.Button, m.Kind, m.CreatedAt)
	if err != nil {
		return err
	}
	m.ID, _ = res.LastInsertId()
	return nil
}

func (s *Store) Messages(chatID, afterID int64) ([]Message, error) {
	rows, err := s.db.Query(`SELECT id,chat_id,text,button,kind,created_at FROM messages WHERE chat_id=? AND id>? ORDER BY id LIMIT 100`, chatID, afterID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := []Message{}
	for rows.Next() {
		var m Message
		rows.Scan(&m.ID, &m.ChatID, &m.Text, &m.Button, &m.Kind, &m.CreatedAt)
		out = append(out, m)
	}
	return out, rows.Err()
}

// ---- meta / reset ----

func (s *Store) Meta(k string) string {
	var v string
	s.db.QueryRow(`SELECT v FROM meta WHERE k=?`, k).Scan(&v)
	return v
}

func (s *Store) SetMeta(k, v string) error {
	_, err := s.db.Exec(`INSERT INTO meta(k,v) VALUES(?,?) ON CONFLICT(k) DO UPDATE SET v=excluded.v`, k, v)
	return err
}

func (s *Store) Wipe() error {
	_, err := s.db.Exec(`DELETE FROM bookings; DELETE FROM messages; DELETE FROM hours; DELETE FROM master_services; DELETE FROM masters; DELETE FROM services; DELETE FROM meta;`)
	return err
}

func mustAffect(res sql.Result, err error) error {
	if err != nil {
		return err
	}
	if n, _ := res.RowsAffected(); n == 0 {
		return ErrNotFound
	}
	return nil
}
