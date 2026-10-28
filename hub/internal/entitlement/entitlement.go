package entitlement

import (
	"database/sql"
	"time"

	"katsironni-hub/internal/db"
)

type Entitlement struct {
	ID                     string
	UserID                 string
	ServiceKey             string
	Source                 string // stripe, crypto, bundle
	Status                 string // active, past_due, canceled, expired
	Interval               string // month, year
	PeriodStart            time.Time
	PeriodEnd              time.Time
	StripeCustomerID       string
	StripeSubscriptionID   string
	UpdatedAt              time.Time
}

func GetUserEntitlements(database *db.DB, userID string) ([]Entitlement, error) {
	rows, err := database.Query(
		`SELECT id, user_id, service_key, source, status, interval, period_start, period_end, stripe_customer_id, stripe_subscription_id, updated_at
		 FROM entitlements WHERE user_id = ?`, userID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var entitlements []Entitlement
	for rows.Next() {
		var e Entitlement
		var periodStartStr, periodEndStr, updatedAtStr string
		if err := rows.Scan(&e.ID, &e.UserID, &e.ServiceKey, &e.Source, &e.Status, &e.Interval,
			&periodStartStr, &periodEndStr, &e.StripeCustomerID, &e.StripeSubscriptionID, &updatedAtStr); err != nil {
			return nil, err
		}
		e.PeriodStart, _ = time.Parse(time.RFC3339, periodStartStr)
		e.PeriodEnd, _ = time.Parse(time.RFC3339, periodEndStr)
		e.UpdatedAt, _ = time.Parse(time.RFC3339, updatedAtStr)
		entitlements = append(entitlements, e)
	}
	return entitlements, nil
}

func HasActiveEntitlement(database *db.DB, userID, serviceKey string) (bool, error) {
	// Check direct entitlement
	var count int
	err := database.QueryRow(
		`SELECT COUNT(*) FROM entitlements
		 WHERE user_id = ? AND service_key = ? AND status = 'active' AND period_end > ?`,
		userID, serviceKey, time.Now().Format(time.RFC3339),
	).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	if count > 0 {
		return true, nil
	}

	// Check bundle entitlement
	err = database.QueryRow(
		`SELECT COUNT(*) FROM entitlements
		 WHERE user_id = ? AND service_key = 'bundle' AND status = 'active' AND period_end > ?`,
		userID, time.Now().Format(time.RFC3339),
	).Scan(&count)
	if err != nil && err != sql.ErrNoRows {
		return false, err
	}
	return count > 0, nil
}

func GetActiveServices(database *db.DB, userID string) ([]string, error) {
	rows, err := database.Query(
		`SELECT DISTINCT service_key FROM entitlements
		 WHERE user_id = ? AND status = 'active' AND period_end > ?`,
		userID, time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var services []string
	for rows.Next() {
		var serviceKey string
		if err := rows.Scan(&serviceKey); err != nil {
			return nil, err
		}
		services = append(services, serviceKey)
	}
	return services, nil
}

func CreateOrUpdateEntitlement(database *db.DB, e *Entitlement) error {
	e.UpdatedAt = time.Now()

	// Try to insert, if conflict update
	_, err := database.Exec(
		`INSERT INTO entitlements (id, user_id, service_key, source, status, interval, period_start, period_end, stripe_customer_id, stripe_subscription_id, updated_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, ?, ?, ?)
		 ON CONFLICT(user_id, service_key, source) DO UPDATE SET
		 status = excluded.status,
		 interval = excluded.interval,
		 period_start = excluded.period_start,
		 period_end = excluded.period_end,
		 stripe_customer_id = excluded.stripe_customer_id,
		 stripe_subscription_id = excluded.stripe_subscription_id,
		 updated_at = excluded.updated_at`,
		e.ID, e.UserID, e.ServiceKey, e.Source, e.Status, e.Interval,
		e.PeriodStart.Format(time.RFC3339), e.PeriodEnd.Format(time.RFC3339),
		e.StripeCustomerID, e.StripeSubscriptionID, e.UpdatedAt.Format(time.RFC3339),
	)
	return err
}

func CancelEntitlement(database *db.DB, userID, serviceKey, source string) error {
	_, err := database.Exec(
		`UPDATE entitlements SET status = 'canceled', updated_at = ? WHERE user_id = ? AND service_key = ? AND source = ?`,
		time.Now().Format(time.RFC3339), userID, serviceKey, source,
	)
	return err
}

func ExpireOldEntitlements(database *db.DB) error {
	_, err := database.Exec(
		`UPDATE entitlements SET status = 'expired', updated_at = ? WHERE status = 'active' AND period_end < ?`,
		time.Now().Format(time.RFC3339), time.Now().Format(time.RFC3339),
	)
	return err
}