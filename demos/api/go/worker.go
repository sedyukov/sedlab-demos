package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"strings"
	"time"
)

const (
	simulatedError = "upstream returned 503 (simulated)"
	lockFor        = 30 * time.Second
	workDuration   = 400 * time.Millisecond
)

// runWorker polls the shared queue. The Python backend runs the same loop against the same table:
// whoever claims a job first (BEGIN IMMEDIATE + status flip) processes it.
func (a *App) runWorker(ctx context.Context) {
	for ctx.Err() == nil {
		worked, err := a.workOnce(ctx)
		if err != nil && ctx.Err() == nil {
			log.Printf("worker: %v", err)
		}
		if !worked {
			select {
			case <-ctx.Done():
			case <-time.After(250 * time.Millisecond):
			}
		}
	}
}

func (a *App) claim(ctx context.Context, now time.Time) (string, error) {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return "", err
	}
	defer tx.Rollback()
	var id string
	err = tx.QueryRowContext(ctx, `SELECT id FROM jobs
		WHERE (status IN ('queued', 'retrying') AND run_at <= ?) OR (status = 'running' AND locked_until < ?)
		ORDER BY run_at, id LIMIT 1`, ts(now), ts(now)).Scan(&id)
	if errors.Is(err, sql.ErrNoRows) {
		return "", nil
	}
	if err != nil {
		return "", err
	}
	if _, err := tx.ExecContext(ctx, `UPDATE jobs SET status = 'running', locked_until = ?, updated_at = ? WHERE id = ?`,
		ts(now.Add(lockFor)), ts(now), id); err != nil {
		return "", err
	}
	return id, tx.Commit()
}

func (a *App) workOnce(ctx context.Context) (bool, error) {
	id, err := a.claim(ctx, time.Now())
	if err != nil || id == "" {
		return false, err
	}
	select { // the "work": an API call to an external system
	case <-ctx.Done():
		return true, nil
	case <-time.After(workDuration):
	}
	return true, a.finish(ctx, id, time.Now())
}

func jobResult(typ string, number, total int64, currency, status string) map[string]any {
	switch typ {
	case "send_invoice":
		return map[string]any{"invoice_number": fmt.Sprintf("INV-%05d", number), "amount": total, "currency": currency}
	case "sync_crm":
		return map[string]any{"crm_deal_id": fmt.Sprintf("deal-%d", number), "stage": status}
	default:
		return map[string]any{"channel": "email", "template": "order_" + status, "delivered": true}
	}
}

func (a *App) finish(ctx context.Context, id string, now time.Time) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	var ws, orderID, typ, hist string
	var attempts, maxAtt, sim int
	err = tx.QueryRowContext(ctx, `SELECT workspace_id, order_id, type, attempts, max_attempts, simulate_failures, history
		FROM jobs WHERE id = ? AND status = 'running'`, id).Scan(&ws, &orderID, &typ, &attempts, &maxAtt, &sim, &hist)
	if errors.Is(err, sql.ErrNoRows) {
		return nil // deleted with its order meanwhile
	}
	if err != nil {
		return err
	}
	var history []histEntry
	_ = json.Unmarshal([]byte(hist), &history)
	attempt := attempts + 1
	entry := histEntry{Attempt: attempt, At: ts(now), Worker: "go", Outcome: "ok"}

	var errMsg string
	var number, total int64
	var currency, status string
	if attempt <= sim {
		errMsg = simulatedError
	} else {
		err := tx.QueryRowContext(ctx, `SELECT number, total, currency, status FROM orders WHERE id = ?`, orderID).Scan(&number, &total, &currency, &status)
		if errors.Is(err, sql.ErrNoRows) {
			errMsg = "order not found"
		} else if err != nil {
			return err
		}
	}
	if errMsg != "" {
		entry.Outcome, entry.Error = "error", &errMsg
		history = append(history, entry)
		hb, _ := json.Marshal(history)
		if attempt >= maxAtt || errMsg == "order not found" {
			_, err = tx.ExecContext(ctx, `UPDATE jobs SET status = 'dead', attempts = ?, last_error = ?, history = ?, locked_until = NULL, updated_at = ? WHERE id = ?`,
				attempt, errMsg, string(hb), ts(now), id)
		} else {
			next := now.Add(time.Duration(1<<(attempt-1)) * time.Second)
			_, err = tx.ExecContext(ctx, `UPDATE jobs SET status = 'retrying', attempts = ?, last_error = ?, history = ?, run_at = ?, locked_until = NULL, updated_at = ? WHERE id = ?`,
				attempt, errMsg, string(hb), ts(next), ts(now), id)
		}
	} else {
		history = append(history, entry)
		hb, _ := json.Marshal(history)
		rb, _ := json.Marshal(jobResult(typ, number, total, currency, status))
		_, err = tx.ExecContext(ctx, `UPDATE jobs SET status = 'succeeded', attempts = ?, last_error = NULL, result = ?, processed_by = 'go', history = ?, locked_until = NULL, updated_at = ? WHERE id = ?`,
			attempt, string(rb), string(hb), ts(now), id)
	}
	if err != nil {
		return err
	}
	return tx.Commit()
}

// ---- housekeeping: idle sandboxes, the daily wipe, old idempotency keys ----

func (a *App) runHousekeeping(ctx context.Context) {
	for {
		if err := a.housekeep(ctx, time.Now().UTC()); err != nil && ctx.Err() == nil {
			log.Printf("housekeeping: %v", err)
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(10 * time.Minute):
		}
	}
}

func (a *App) housekeep(ctx context.Context, now time.Time) error {
	tx, err := a.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	defer tx.Rollback()
	conds := []string{
		`(auth_method = 'sandbox' AND last_seen_at < '` + ts(now.Add(-12*time.Hour)) + `')`,
		`(auth_method = 'accounts' AND last_seen_at < '` + ts(now.Add(-30*24*time.Hour)) + `')`,
	}
	today := now.Format("2006-01-02")
	if now.Hour() >= 3 {
		var last string
		_ = tx.QueryRowContext(ctx, `SELECT value FROM settings WHERE key = 'last_wipe'`).Scan(&last)
		if last != today {
			conds = append(conds, `(auth_method = 'sandbox' AND created_at < '`+today+`T03:00:00.000Z')`)
			if _, err := tx.ExecContext(ctx, `INSERT INTO settings (key, value) VALUES ('last_wipe', ?) ON CONFLICT (key) DO UPDATE SET value = excluded.value`, today); err != nil {
				return err
			}
		}
	}
	sel := `SELECT id FROM workspaces WHERE ` + strings.Join(conds, " OR ")
	for _, t := range []string{"jobs", "orders", "customers", "idempotency_keys"} {
		if _, err := tx.ExecContext(ctx, `DELETE FROM `+t+` WHERE workspace_id IN (`+sel+`)`); err != nil {
			return err
		}
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM workspaces WHERE id IN (`+sel+`)`); err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM idempotency_keys WHERE created_at < ?`, ts(now.Add(-24*time.Hour))); err != nil {
		return err
	}
	return tx.Commit()
}
