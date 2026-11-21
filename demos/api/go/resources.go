package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"math"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"
)

// ---- pagination ----

func encodeCursor(sort, value, id string) string {
	return b64.EncodeToString([]byte(sort + "|" + value + "|" + id))
}

func decodeCursor(c, sort string, numeric bool) (string, string, bool) {
	b, err := b64.DecodeString(c)
	if err != nil || !utf8.Valid(b) {
		return "", "", false
	}
	s := string(b)
	if !strings.HasPrefix(s, sort+"|") {
		return "", "", false
	}
	rest := s[len(sort)+1:]
	i := strings.LastIndex(rest, "|")
	if i < 0 || i == len(rest)-1 {
		return "", "", false
	}
	value, id := rest[:i], rest[i+1:]
	if numeric && !intRe.MatchString(value) {
		return "", "", false
	}
	return value, id, true
}

type page struct {
	sort    string
	col     string
	desc    bool
	numeric bool
	limit   int64
	after   *[2]string // value, id
}

// parsePage reads sort, limit and cursor (in that order) after the resource-specific filters.
func parsePage(qv *queryV, sorts map[string]bool, defSort string, numericCols map[string]bool) page {
	var allowed []string
	for s := range sorts {
		allowed = append(allowed, s)
	}
	p := page{sort: defSort}
	if sorts != nil {
		p.sort = qv.enum("sort", defSort, allowed...)
	}
	p.limit = qv.intParam("limit", 1, 100, 20)
	p.col = strings.TrimPrefix(p.sort, "-")
	p.desc = strings.HasPrefix(p.sort, "-")
	p.numeric = numericCols[p.col]
	if c, ok := qv.str("cursor"); ok {
		v, id, good := decodeCursor(c, p.sort, p.numeric)
		if !good {
			qv.add("cursor", "invalid")
		} else {
			p.after = &[2]string{v, id}
		}
	}
	return p
}

func (p page) keyset(where []string, args []any) ([]string, []any) {
	if p.after == nil {
		return where, args
	}
	op := ">"
	if p.desc {
		op = "<"
	}
	var v any = p.after[0]
	if p.numeric {
		n, _ := strconv.ParseInt(p.after[0], 10, 64)
		v = n
	}
	where = append(where, "("+p.col+" "+op+" ? OR ("+p.col+" = ? AND id "+op+" ?))")
	return where, append(args, v, v, p.after[1])
}

func (p page) order() string {
	dir := "ASC"
	if p.desc {
		dir = "DESC"
	}
	return " ORDER BY " + p.col + " " + dir + ", id " + dir + " LIMIT " + strconv.FormatInt(p.limit+1, 10)
}

func listResponse[T any](items []T, p page, total int, key func(T) (string, string)) map[string]any {
	var next any
	if int64(len(items)) > p.limit {
		items = items[:p.limit]
		v, id := key(items[len(items)-1])
		next = encodeCursor(p.sort, v, id)
	}
	if items == nil {
		items = []T{}
	}
	return map[string]any{"items": items, "next_cursor": next, "total_count": total}
}

func itoa(n int64) string { return strconv.FormatInt(n, 10) }

// ---- customers ----

type Customer struct {
	ID        string  `json:"id"`
	Name      string  `json:"name"`
	City      *string `json:"city"`
	CreatedAt string  `json:"created_at"`
	UpdatedAt string  `json:"updated_at"`
}

const customerCols = `id, name, city, created_at, updated_at`

func scanCustomer(s interface{ Scan(...any) error }) (Customer, error) {
	var c Customer
	var city sql.NullString
	err := s.Scan(&c.ID, &c.Name, &city, &c.CreatedAt, &c.UpdatedAt)
	if city.Valid {
		c.City = &city.String
	}
	return c, err
}

func customerSearch(name string, city *string) string {
	s := name
	if city != nil {
		s += " " + *city
	}
	return strings.ToLower(s)
}

func (a *App) listCustomers(ctx context.Context, q *req) (result, *apiError) {
	qv := &queryV{q: q.r.URL.Query()}
	search := qv.search("q")
	p := parsePage(qv, map[string]bool{"name": true, "-name": true, "created_at": true, "-created_at": true}, "-created_at", nil)
	if len(qv.details) > 0 {
		return result{}, validationErr(qv.details)
	}
	where, args := []string{"workspace_id = ?"}, []any{q.p.WS}
	if search != "" {
		where, args = append(where, "instr(search, ?) > 0"), append(args, search)
	}
	var total int
	if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM customers WHERE `+strings.Join(where, " AND "), args...).Scan(&total); err != nil {
		return result{}, dbErr(err)
	}
	where, args = p.keyset(where, args)
	rows, err := a.db.QueryContext(ctx, `SELECT `+customerCols+` FROM customers WHERE `+strings.Join(where, " AND ")+p.order(), args...)
	if err != nil {
		return result{}, dbErr(err)
	}
	defer rows.Close()
	var items []Customer
	for rows.Next() {
		c, err := scanCustomer(rows)
		if err != nil {
			return result{}, dbErr(err)
		}
		items = append(items, c)
	}
	return result{200, listResponse(items, p, total, func(c Customer) (string, string) {
		if p.col == "name" {
			return c.Name, c.ID
		}
		return c.CreatedAt, c.ID
	})}, nil
}

func (a *App) getCustomerRow(ctx context.Context, db queryer, ws, id string) (Customer, *apiError) {
	c, err := scanCustomer(db.QueryRowContext(ctx, `SELECT `+customerCols+` FROM customers WHERE workspace_id = ? AND id = ?`, ws, id))
	if errors.Is(err, sql.ErrNoRows) {
		return c, notFound("customer")
	}
	if err != nil {
		return c, dbErr(err)
	}
	return c, nil
}

func (a *App) getCustomer(ctx context.Context, q *req) (result, *apiError) {
	c, e := a.getCustomerRow(ctx, a.db, q.p.WS, q.params["id"])
	if e != nil {
		return result{}, e
	}
	return result{200, c}, nil
}

func customerFields(v *validator) {
	v.str("name", true, false, 1, 100)
	v.str("city", false, true, 0, 60)
	v.unknown()
}

func optStr(v any) *string {
	if s, ok := v.(string); ok {
		return &s
	}
	return nil
}

func (a *App) createCustomer(ctx context.Context, q *req) (result, *apiError) {
	obj, e := parseObject(q.body)
	if e != nil {
		return result{}, e
	}
	v := newValidator(obj, false)
	customerFields(v)
	if len(v.details) > 0 {
		return result{}, validationErr(v.details)
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return result{}, dbErr(err)
	}
	defer tx.Rollback()
	if e := a.checkQuota(ctx, tx, q.p.WS, "customers", quotaCustomers); e != nil {
		return result{}, e
	}
	now := ts(time.Now())
	c := Customer{ID: newID("cus_"), Name: v.out["name"].(string), City: optStr(v.out["city"]), CreatedAt: now, UpdatedAt: now}
	if _, err := tx.ExecContext(ctx, `INSERT INTO customers (id, workspace_id, name, city, search, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
		c.ID, q.p.WS, c.Name, c.City, customerSearch(c.Name, c.City), now, now); err != nil {
		return result{}, dbErr(err)
	}
	if err := tx.Commit(); err != nil {
		return result{}, dbErr(err)
	}
	return result{201, c}, nil
}

func (a *App) checkQuota(ctx context.Context, tx *sql.Tx, ws, table string, max int) *apiError {
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM `+table+` WHERE workspace_id = ?`, ws).Scan(&n); err != nil {
		return dbErr(err)
	}
	if n >= max {
		return errf(409, "quota_exceeded", "workspace quota exceeded: "+table)
	}
	return nil
}

func (a *App) updateCustomer(ctx context.Context, q *req) (result, *apiError) {
	obj, e := parseObject(q.body)
	if e != nil {
		return result{}, e
	}
	if len(obj) == 0 {
		return result{}, validationErr([]detail{{"body", "empty"}})
	}
	v := newValidator(obj, true)
	customerFields(v)
	if len(v.details) > 0 {
		return result{}, validationErr(v.details)
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return result{}, dbErr(err)
	}
	defer tx.Rollback()
	c, e := a.getCustomerRow(ctx, tx, q.p.WS, q.params["id"])
	if e != nil {
		return result{}, e
	}
	if s, ok := v.out["name"].(string); ok {
		c.Name = s
	}
	if val, ok := v.out["city"]; ok {
		c.City = optStr(val)
	}
	c.UpdatedAt = ts(time.Now())
	if _, err := tx.ExecContext(ctx, `UPDATE customers SET name = ?, city = ?, search = ?, updated_at = ? WHERE id = ?`,
		c.Name, c.City, customerSearch(c.Name, c.City), c.UpdatedAt, c.ID); err != nil {
		return result{}, dbErr(err)
	}
	if err := tx.Commit(); err != nil {
		return result{}, dbErr(err)
	}
	return result{200, c}, nil
}

func (a *App) deleteCustomer(ctx context.Context, q *req) (result, *apiError) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return result{}, dbErr(err)
	}
	defer tx.Rollback()
	c, e := a.getCustomerRow(ctx, tx, q.p.WS, q.params["id"])
	if e != nil {
		return result{}, e
	}
	var n int
	if err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM orders WHERE workspace_id = ? AND customer_id = ?`, q.p.WS, c.ID).Scan(&n); err != nil {
		return result{}, dbErr(err)
	}
	if n > 0 {
		return result{}, errf(409, "conflict", "customer has orders")
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM customers WHERE id = ?`, c.ID); err != nil {
		return result{}, dbErr(err)
	}
	if err := tx.Commit(); err != nil {
		return result{}, dbErr(err)
	}
	return result{204, nil}, nil
}

// ---- orders ----

type Order struct {
	ID         string   `json:"id"`
	Number     int64    `json:"number"`
	CustomerID string   `json:"customer_id"`
	Title      string   `json:"title"`
	Status     string   `json:"status"`
	Total      int64    `json:"total"`
	Currency   string   `json:"currency"`
	Tags       []string `json:"tags"`
	DueDate    *string  `json:"due_date"`
	CreatedAt  string   `json:"created_at"`
	UpdatedAt  string   `json:"updated_at"`
}

const orderCols = `id, number, customer_id, title, status, total, currency, tags, due_date, created_at, updated_at`

var orderStatuses = []string{"new", "confirmed", "in_progress", "done", "cancelled"}

var transitions = map[string][]string{
	"new":         {"confirmed", "cancelled"},
	"confirmed":   {"in_progress", "cancelled"},
	"in_progress": {"done", "cancelled"},
}

func encodeTags(t []string) string {
	if len(t) == 0 {
		return ""
	}
	return "|" + strings.Join(t, "|") + "|"
}

func decodeTags(s string) []string {
	if s == "" {
		return []string{}
	}
	return strings.Split(strings.Trim(s, "|"), "|")
}

func scanOrder(s interface{ Scan(...any) error }) (Order, error) {
	var o Order
	var tags string
	var due sql.NullString
	err := s.Scan(&o.ID, &o.Number, &o.CustomerID, &o.Title, &o.Status, &o.Total, &o.Currency, &tags, &due, &o.CreatedAt, &o.UpdatedAt)
	o.Tags = decodeTags(tags)
	if due.Valid {
		o.DueDate = &due.String
	}
	return o, err
}

func contains(list []string, s string) bool {
	for _, x := range list {
		if x == s {
			return true
		}
	}
	return false
}

func (a *App) listOrders(ctx context.Context, q *req) (result, *apiError) {
	qv := &queryV{q: q.r.URL.Query()}
	where, args := []string{"workspace_id = ?"}, []any{q.p.WS}
	if s, ok := qv.str("status"); ok {
		parts := strings.Split(s, ",")
		good := true
		for _, st := range parts {
			if !contains(orderStatuses, st) {
				good = false
			}
		}
		if !good {
			qv.add("status", "invalid")
		} else {
			ph := strings.TrimSuffix(strings.Repeat("?, ", len(parts)), ", ")
			where = append(where, "status IN ("+ph+")")
			for _, st := range parts {
				args = append(args, st)
			}
		}
	}
	if s, ok := qv.str("customer_id"); ok {
		where, args = append(where, "customer_id = ?"), append(args, s)
	}
	if s, ok := qv.str("tag"); ok {
		where, args = append(where, "instr(tags, ?) > 0"), append(args, "|"+s+"|")
	}
	if s := qv.search("q"); s != "" {
		where, args = append(where, "instr(search, ?) > 0"), append(args, s)
	}
	if d := qv.date("created_from"); d != "" {
		where, args = append(where, "created_at >= ?"), append(args, d+"T00:00:00.000Z")
	}
	if d := qv.date("created_to"); d != "" {
		where, args = append(where, "created_at <= ?"), append(args, d+"T23:59:59.999Z")
	}
	if n := qv.intParam("min_total", 0, math.MaxInt64, -1); n >= 0 {
		where, args = append(where, "total >= ?"), append(args, n)
	}
	if n := qv.intParam("max_total", 0, math.MaxInt64, -1); n >= 0 {
		where, args = append(where, "total <= ?"), append(args, n)
	}
	p := parsePage(qv, map[string]bool{"created_at": true, "-created_at": true, "total": true, "-total": true, "number": true, "-number": true},
		"-created_at", map[string]bool{"total": true, "number": true})
	if len(qv.details) > 0 {
		return result{}, validationErr(qv.details)
	}
	var total int
	if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM orders WHERE `+strings.Join(where, " AND "), args...).Scan(&total); err != nil {
		return result{}, dbErr(err)
	}
	where, args = p.keyset(where, args)
	rows, err := a.db.QueryContext(ctx, `SELECT `+orderCols+` FROM orders WHERE `+strings.Join(where, " AND ")+p.order(), args...)
	if err != nil {
		return result{}, dbErr(err)
	}
	defer rows.Close()
	var items []Order
	for rows.Next() {
		o, err := scanOrder(rows)
		if err != nil {
			return result{}, dbErr(err)
		}
		items = append(items, o)
	}
	return result{200, listResponse(items, p, total, func(o Order) (string, string) {
		switch p.col {
		case "total":
			return itoa(o.Total), o.ID
		case "number":
			return itoa(o.Number), o.ID
		}
		return o.CreatedAt, o.ID
	})}, nil
}

func (a *App) getOrderRow(ctx context.Context, db queryer, ws, id string) (Order, *apiError) {
	o, err := scanOrder(db.QueryRowContext(ctx, `SELECT `+orderCols+` FROM orders WHERE workspace_id = ? AND id = ?`, ws, id))
	if errors.Is(err, sql.ErrNoRows) {
		return o, notFound("order")
	}
	if err != nil {
		return o, dbErr(err)
	}
	return o, nil
}

func (a *App) getOrder(ctx context.Context, q *req) (result, *apiError) {
	o, e := a.getOrderRow(ctx, a.db, q.p.WS, q.params["id"])
	if e != nil {
		return result{}, e
	}
	return result{200, o}, nil
}

func orderFields(v *validator) {
	v.str("customer_id", true, false, 1, 64)
	v.str("title", true, false, 1, 120)
	if v.patch {
		v.enum("status", false, orderStatuses...)
	}
	v.integer("total", true, 0, 100_000_000)
	v.enum("currency", false, "RUB", "USD", "EUR")
	v.tags("tags")
	v.date("due_date", true)
	v.unknown()
}

func customerExists(ctx context.Context, tx *sql.Tx, ws, id string) (bool, error) {
	var n int
	err := tx.QueryRowContext(ctx, `SELECT COUNT(*) FROM customers WHERE workspace_id = ? AND id = ?`, ws, id).Scan(&n)
	return n > 0, err
}

func (a *App) defaultCurrency() string {
	if a.cfg.Edition == "ru" {
		return "RUB"
	}
	return "USD"
}

func (a *App) createOrder(ctx context.Context, q *req) (result, *apiError) {
	obj, e := parseObject(q.body)
	if e != nil {
		return result{}, e
	}
	v := newValidator(obj, false)
	orderFields(v)
	if len(v.details) > 0 {
		return result{}, validationErr(v.details)
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return result{}, dbErr(err)
	}
	defer tx.Rollback()
	o := Order{CustomerID: v.out["customer_id"].(string), Title: v.out["title"].(string), Status: "new",
		Total: v.out["total"].(int64), Currency: a.defaultCurrency(), Tags: []string{}}
	if ok, err := customerExists(ctx, tx, q.p.WS, o.CustomerID); err != nil {
		return result{}, dbErr(err)
	} else if !ok {
		return result{}, validationErr([]detail{{"customer_id", "not_found"}})
	}
	if e := a.checkQuota(ctx, tx, q.p.WS, "orders", quotaOrders); e != nil {
		return result{}, e
	}
	if s, ok := v.out["currency"].(string); ok {
		o.Currency = s
	}
	if t, ok := v.out["tags"].([]string); ok {
		o.Tags = t
	}
	o.DueDate = optStr(v.out["due_date"])
	if err := tx.QueryRowContext(ctx, `UPDATE workspaces SET order_seq = order_seq + 1 WHERE id = ? RETURNING order_seq`, q.p.WS).Scan(&o.Number); err != nil {
		return result{}, dbErr(err)
	}
	now := ts(time.Now())
	o.ID, o.CreatedAt, o.UpdatedAt = newID("ord_"), now, now
	if err := insertOrder(ctx, tx, q.p.WS, o); err != nil {
		return result{}, dbErr(err)
	}
	if err := tx.Commit(); err != nil {
		return result{}, dbErr(err)
	}
	return result{201, o}, nil
}

func insertOrder(ctx context.Context, tx *sql.Tx, ws string, o Order) error {
	_, err := tx.ExecContext(ctx, `INSERT INTO orders (id, workspace_id, number, customer_id, title, search, status, total, currency, tags, due_date, created_at, updated_at)
		VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)`,
		o.ID, ws, o.Number, o.CustomerID, o.Title, strings.ToLower(o.Title), o.Status, o.Total, o.Currency, encodeTags(o.Tags), o.DueDate, o.CreatedAt, o.UpdatedAt)
	return err
}

func (a *App) updateOrder(ctx context.Context, q *req) (result, *apiError) {
	obj, e := parseObject(q.body)
	if e != nil {
		return result{}, e
	}
	if len(obj) == 0 {
		return result{}, validationErr([]detail{{"body", "empty"}})
	}
	v := newValidator(obj, true)
	orderFields(v)
	if len(v.details) > 0 {
		return result{}, validationErr(v.details)
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return result{}, dbErr(err)
	}
	defer tx.Rollback()
	o, e := a.getOrderRow(ctx, tx, q.p.WS, q.params["id"])
	if e != nil {
		return result{}, e
	}
	if s, ok := v.out["customer_id"].(string); ok {
		if ok, err := customerExists(ctx, tx, q.p.WS, s); err != nil {
			return result{}, dbErr(err)
		} else if !ok {
			return result{}, validationErr([]detail{{"customer_id", "not_found"}})
		}
		o.CustomerID = s
	}
	if s, ok := v.out["status"].(string); ok && s != o.Status {
		if !contains(transitions[o.Status], s) {
			return result{}, &apiError{Status: 409, Code: "invalid_transition",
				Message: "cannot change status from " + o.Status + " to " + s, Details: []detail{{"status", "invalid"}}}
		}
		o.Status = s
	}
	if s, ok := v.out["title"].(string); ok {
		o.Title = s
	}
	if n, ok := v.out["total"].(int64); ok {
		o.Total = n
	}
	if s, ok := v.out["currency"].(string); ok {
		o.Currency = s
	}
	if t, ok := v.out["tags"].([]string); ok {
		o.Tags = t
	}
	if val, ok := v.out["due_date"]; ok {
		o.DueDate = optStr(val)
	}
	o.UpdatedAt = ts(time.Now())
	if _, err := tx.ExecContext(ctx, `UPDATE orders SET customer_id = ?, title = ?, search = ?, status = ?, total = ?, currency = ?, tags = ?, due_date = ?, updated_at = ? WHERE id = ?`,
		o.CustomerID, o.Title, strings.ToLower(o.Title), o.Status, o.Total, o.Currency, encodeTags(o.Tags), o.DueDate, o.UpdatedAt, o.ID); err != nil {
		return result{}, dbErr(err)
	}
	if err := tx.Commit(); err != nil {
		return result{}, dbErr(err)
	}
	return result{200, o}, nil
}

func (a *App) deleteOrder(ctx context.Context, q *req) (result, *apiError) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return result{}, dbErr(err)
	}
	defer tx.Rollback()
	o, e := a.getOrderRow(ctx, tx, q.p.WS, q.params["id"])
	if e != nil {
		return result{}, e
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM jobs WHERE workspace_id = ? AND order_id = ?`, q.p.WS, o.ID); err != nil {
		return result{}, dbErr(err)
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM orders WHERE id = ?`, o.ID); err != nil {
		return result{}, dbErr(err)
	}
	if err := tx.Commit(); err != nil {
		return result{}, dbErr(err)
	}
	return result{204, nil}, nil
}

// ---- jobs (HTTP side; the worker is in worker.go) ----

type histEntry struct {
	Attempt int     `json:"attempt"`
	At      string  `json:"at"`
	Worker  string  `json:"worker"`
	Outcome string  `json:"outcome"`
	Error   *string `json:"error"`
}

type Job struct {
	ID               string          `json:"id"`
	OrderID          string          `json:"order_id"`
	Type             string          `json:"type"`
	Status           string          `json:"status"`
	Attempts         int             `json:"attempts"`
	MaxAttempts      int             `json:"max_attempts"`
	SimulateFailures int             `json:"simulate_failures"`
	NextRunAt        *string         `json:"next_run_at"`
	LastError        *string         `json:"last_error"`
	Result           json.RawMessage `json:"result"`
	ProcessedBy      *string         `json:"processed_by"`
	History          []histEntry     `json:"history"`
	CreatedAt        string          `json:"created_at"`
	UpdatedAt        string          `json:"updated_at"`
}

const jobCols = `id, order_id, type, status, attempts, max_attempts, simulate_failures, run_at, last_error, result, processed_by, history, created_at, updated_at`

const maxAttempts = 4

var jobTypes = []string{"send_invoice", "sync_crm", "notify_customer"}
var jobStatuses = []string{"queued", "running", "retrying", "succeeded", "dead"}

func scanJob(s interface{ Scan(...any) error }) (Job, error) {
	var j Job
	var runAt, hist string
	var lastErr, res, by sql.NullString
	err := s.Scan(&j.ID, &j.OrderID, &j.Type, &j.Status, &j.Attempts, &j.MaxAttempts, &j.SimulateFailures, &runAt,
		&lastErr, &res, &by, &hist, &j.CreatedAt, &j.UpdatedAt)
	if err != nil {
		return j, err
	}
	if j.Status == "queued" || j.Status == "retrying" {
		j.NextRunAt = &runAt
	}
	if lastErr.Valid {
		j.LastError = &lastErr.String
	}
	j.Result = json.RawMessage("null")
	if res.Valid {
		j.Result = json.RawMessage(res.String)
	}
	if by.Valid {
		j.ProcessedBy = &by.String
	}
	j.History = []histEntry{}
	_ = json.Unmarshal([]byte(hist), &j.History)
	return j, nil
}

func (a *App) enqueueJob(ctx context.Context, q *req) (result, *apiError) {
	obj, e := parseObject(q.body)
	if e != nil {
		return result{}, e
	}
	v := newValidator(obj, false)
	v.enum("type", true, jobTypes...)
	v.integer("simulate_failures", false, 0, 5)
	v.unknown()
	if len(v.details) > 0 {
		return result{}, validationErr(v.details)
	}
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return result{}, dbErr(err)
	}
	defer tx.Rollback()
	o, e := a.getOrderRow(ctx, tx, q.p.WS, q.params["id"])
	if e != nil {
		return result{}, e
	}
	if e := a.checkQuota(ctx, tx, q.p.WS, "jobs", quotaJobs); e != nil {
		return result{}, e
	}
	sim, _ := v.out["simulate_failures"].(int64)
	now := ts(time.Now())
	id := newID("job_")
	if _, err := tx.ExecContext(ctx, `INSERT INTO jobs (id, workspace_id, order_id, type, status, attempts, max_attempts, simulate_failures, run_at, history, created_at, updated_at)
		VALUES (?, ?, ?, ?, 'queued', 0, ?, ?, ?, '[]', ?, ?)`, id, q.p.WS, o.ID, v.out["type"], maxAttempts, sim, now, now, now); err != nil {
		return result{}, dbErr(err)
	}
	j, err := scanJob(tx.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE id = ?`, id))
	if err != nil {
		return result{}, dbErr(err)
	}
	if err := tx.Commit(); err != nil {
		return result{}, dbErr(err)
	}
	return result{202, j}, nil
}

func (a *App) listJobs(ctx context.Context, q *req) (result, *apiError) {
	qv := &queryV{q: q.r.URL.Query()}
	where, args := []string{"workspace_id = ?"}, []any{q.p.WS}
	if s, ok := qv.str("order_id"); ok {
		where, args = append(where, "order_id = ?"), append(args, s)
	}
	if qv.has("status") {
		if s := qv.enum("status", "", jobStatuses...); s != "" {
			where, args = append(where, "status = ?"), append(args, s)
		}
	}
	p := parsePage(qv, nil, "-created_at", nil)
	if len(qv.details) > 0 {
		return result{}, validationErr(qv.details)
	}
	var total int
	if err := a.db.QueryRowContext(ctx, `SELECT COUNT(*) FROM jobs WHERE `+strings.Join(where, " AND "), args...).Scan(&total); err != nil {
		return result{}, dbErr(err)
	}
	where, args = p.keyset(where, args)
	rows, err := a.db.QueryContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE `+strings.Join(where, " AND ")+p.order(), args...)
	if err != nil {
		return result{}, dbErr(err)
	}
	defer rows.Close()
	var items []Job
	for rows.Next() {
		j, err := scanJob(rows)
		if err != nil {
			return result{}, dbErr(err)
		}
		items = append(items, j)
	}
	return result{200, listResponse(items, p, total, func(j Job) (string, string) { return j.CreatedAt, j.ID })}, nil
}

func (a *App) getJob(ctx context.Context, q *req) (result, *apiError) {
	j, err := scanJob(a.db.QueryRowContext(ctx, `SELECT `+jobCols+` FROM jobs WHERE workspace_id = ? AND id = ?`, q.p.WS, q.params["id"]))
	if errors.Is(err, sql.ErrNoRows) {
		return result{}, notFound("job")
	}
	if err != nil {
		return result{}, dbErr(err)
	}
	return result{200, j}, nil
}
