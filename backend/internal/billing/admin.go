package billing

import (
	"context"
	"crypto/sha256"
	"crypto/subtle"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
	"os"
	"site-assistant/internal/core"
	"time"
)

func AuthorizedAdmin() bool {
	expected, e := hex.DecodeString(os.Getenv("BILLING_ADMIN_TOKEN_HASH"))
	token := os.Getenv("BILLING_ADMIN_TOKEN")
	actual := sha256.Sum256([]byte(token))
	return e == nil && len(expected) == 32 && len(token) >= 32 && subtle.ConstantTimeCompare(expected, actual[:]) == 1
}

type AdminInput struct {
	OrganizationID, ExpectedName, Plan, Action, RequestID, Actor string
	Days                                                         int
}

// Administrative commands use a separate environment credential, never a cabinet HTTP endpoint.
func Admin(ctx context.Context, db *pgxpool.Pool, in AdminInput) (json.RawMessage, error) {
	if !AuthorizedAdmin() {
		return nil, fmt.Errorf("billing administrative authorization required")
	}
	if in.Action != "grant" && in.Action != "change-plan" && in.Action != "extend" && in.Action != "suspend" {
		return nil, fmt.Errorf("invalid subscription action")
	}
	if in.Days < 1 || in.Days > 366 || in.OrganizationID == "" || in.RequestID == "" || in.ExpectedName == "" || in.Actor == "" {
		return nil, fmt.Errorf("explicit organization, expected name, actor, request ID and days required")
	}
	tx, e := db.Begin(ctx)
	if e != nil {
		return nil, e
	}
	defer tx.Rollback(ctx)
	var name string
	if e = tx.QueryRow(ctx, "SELECT name FROM organizations WHERE id=$1", in.OrganizationID).Scan(&name); e != nil {
		return nil, e
	}
	if name != in.ExpectedName {
		return nil, fmt.Errorf("organization name mismatch")
	}
	payload, _ := json.Marshal(in)
	hash := sha256.Sum256(payload)
	hashText := hex.EncodeToString(hash[:])
	var before []byte
	if e = tx.QueryRow(ctx, "SELECT to_jsonb(s) FROM billing_subscriptions s WHERE organization_id=$1 FOR UPDATE", in.OrganizationID).Scan(&before); e != nil {
		return nil, e
	}
	var prior string
	e = tx.QueryRow(ctx, "SELECT request_hash FROM billing_audit WHERE organization_id=$1 AND request_id=$2", in.OrganizationID, in.RequestID).Scan(&prior)
	if e == nil {
		if prior != hashText {
			return nil, core.Fail("request_conflict", 409)
		}
		tx.Rollback(ctx)
		return Snapshot(ctx, db, in.OrganizationID)
	}
	if e != pgx.ErrNoRows {
		return nil, e
	}
	var state struct {
		PlanID     string    `json:"plan_id"`
		Status     string    `json:"status"`
		Region     string    `json:"region"`
		ValidUntil time.Time `json:"valid_until"`
	}
	if e = json.Unmarshal(before, &state); e != nil {
		return nil, e
	}
	var p struct {
		ID, Code   string
		PeriodDays int
	}
	var limits []byte
	if in.Action == "grant" || in.Action == "change-plan" {
		e = tx.QueryRow(ctx, "SELECT id,code,period_days,billing_plan_limits(p) FROM billing_plans p WHERE region=$1 AND code=$2 AND status='active' ORDER BY version DESC LIMIT 1", state.Region, in.Plan).Scan(&p.ID, &p.Code, &p.PeriodDays, &limits)
	} else {
		e = tx.QueryRow(ctx, "SELECT id,code,period_days,billing_plan_limits(p) FROM billing_plans p WHERE id=$1", state.PlanID).Scan(&p.ID, &p.Code, &p.PeriodDays, &limits)
	}
	if e != nil {
		return nil, e
	}
	var period string
	var end time.Time
	if e = tx.QueryRow(ctx, "SELECT id,ends_at FROM billing_periods WHERE organization_id=$1 ORDER BY starts_at DESC LIMIT 1", in.OrganizationID).Scan(&period, &end); e != nil {
		return nil, e
	}
	now := time.Now().UTC()
	until := state.ValidUntil
	status := "active"
	if p.Code == "trial" {
		status = "trialing"
	}
	if in.Action == "suspend" {
		status = "suspended"
	} else if in.Action == "extend" {
		if until.Before(now) {
			until = now
		}
		until = until.Add(time.Duration(in.Days) * 24 * time.Hour)
	} else {
		until = now.Add(time.Duration(in.Days) * 24 * time.Hour)
	}
	if in.Action != "suspend" {
		if end.After(now) {
			newEnd := end
			if p.Code == "trial" {
				newEnd = until
			} else if newEnd.After(until) {
				newEnd = until
			}
			_, e = tx.Exec(ctx, "UPDATE billing_periods SET plan_id=$2,limits=$3,ends_at=$4 WHERE id=$1", period, p.ID, limits, newEnd)
		} else {
			newEnd := now.Add(time.Duration(p.PeriodDays) * 24 * time.Hour)
			if newEnd.After(until) || p.Code == "trial" {
				newEnd = until
			}
			_, e = tx.Exec(ctx, "INSERT INTO billing_periods(organization_id,plan_id,starts_at,ends_at,limits) VALUES($1,$2,$3,$4,$5)", in.OrganizationID, p.ID, now, newEnd, limits)
		}
		if e != nil {
			return nil, e
		}
	}
	var after []byte
	e = tx.QueryRow(ctx, "UPDATE billing_subscriptions SET plan_id=$2,status=$3,valid_until=$4,trial_end=CASE WHEN $5='trial' THEN $4 ELSE trial_end END,activation_source='cli',updated_at=now() WHERE organization_id=$1 RETURNING to_jsonb(billing_subscriptions)", in.OrganizationID, p.ID, status, until, p.Code).Scan(&after)
	if e != nil {
		return nil, e
	}
	if _, e = tx.Exec(ctx, "INSERT INTO billing_audit(organization_id,request_id,actor,action,request_hash,before_state,after_state) VALUES($1,$2,$3,$4,$5,$6,$7)", in.OrganizationID, in.RequestID, in.Actor, in.Action, hashText, before, after); e != nil {
		return nil, e
	}
	if e = tx.Commit(ctx); e != nil {
		return nil, e
	}
	return Snapshot(ctx, db, in.OrganizationID)
}
