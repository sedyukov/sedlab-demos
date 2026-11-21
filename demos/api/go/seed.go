package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// Sample data for a new workspace: ../contract/seed.json (shared with the Python backend, synthetic companies).
type seedData struct {
	Customers []struct {
		Name map[string]string  `json:"name"`
		City map[string]*string `json:"city"`
	} `json:"customers"`
	Orders []struct {
		Customer int               `json:"customer"`
		Title    map[string]string `json:"title"`
		Total    map[string]int64  `json:"total"`
		Status   string            `json:"status"`
		Tags     []string          `json:"tags"`
		DueDays  *int              `json:"due_in_days"`
		AgeHours float64           `json:"age_hours"`
	} `json:"orders"`
}

var seed *seedData

func loadSeed() (*seedData, error) {
	if seed != nil {
		return seed, nil
	}
	b, err := os.ReadFile(filepath.Join(env("CONTRACT_DIR", "../contract"), "seed.json"))
	if err != nil {
		return nil, err
	}
	var s seedData
	if err := json.Unmarshal(b, &s); err != nil {
		return nil, err
	}
	seed = &s
	return seed, nil
}

func seedWorkspace(ctx context.Context, tx *sql.Tx, ws, edition string, now time.Time) error {
	s, err := loadSeed()
	if err != nil {
		return err
	}
	cur := "USD"
	if edition == "ru" {
		cur = "RUB"
	}
	n := len(s.Customers)
	ids := make([]string, n)
	for i, c := range s.Customers {
		ids[i] = newID("cus_")
		created := ts(now.Add(-time.Duration(30*24-i*24) * time.Hour)) // a month ago, a day apart
		city := c.City[edition]
		if _, err := tx.ExecContext(ctx, `INSERT INTO customers (id, workspace_id, name, city, search, created_at, updated_at) VALUES (?, ?, ?, ?, ?, ?, ?)`,
			ids[i], ws, c.Name[edition], city, customerSearch(c.Name[edition], city), created, created); err != nil {
			return err
		}
	}
	for i, o := range s.Orders {
		created := ts(now.Add(-time.Duration(o.AgeHours * float64(time.Hour))))
		ord := Order{ID: newID("ord_"), Number: int64(i + 1), CustomerID: ids[o.Customer], Title: o.Title[edition], Status: o.Status,
			Total: o.Total[edition], Currency: cur, Tags: o.Tags, CreatedAt: created, UpdatedAt: created}
		if o.DueDays != nil {
			d := now.AddDate(0, 0, *o.DueDays).Format("2006-01-02")
			ord.DueDate = &d
		}
		if err := insertOrder(ctx, tx, ws, ord); err != nil {
			return err
		}
	}
	_, err = tx.ExecContext(ctx, `UPDATE workspaces SET order_seq = ? WHERE id = ?`, len(s.Orders), ws)
	return err
}
