package db

import (
	"database/sql"
	"fmt"
	"log"
	"strings"
	"time"

	_ "modernc.org/sqlite"
)

type DB struct {
	*sql.DB
}

func Open(path string) (*DB, error) {
	db, err := sql.Open("sqlite", path+"?_pragma=journal_mode(WAL)&_pragma=busy_timeout(5000)&_pragma=foreign_keys(ON)")
	if err != nil {
		return nil, fmt.Errorf("open db: %w", err)
	}

	// Test connection
	if err := db.Ping(); err != nil {
		return nil, fmt.Errorf("ping db: %w", err)
	}

	// Run migrations
	if err := migrate(db); err != nil {
		return nil, fmt.Errorf("migrate: %w", err)
	}

	return &DB{db}, nil
}

func migrate(db *sql.DB) error {
	migrations := []string{
		`CREATE TABLE IF NOT EXISTS users (
			id TEXT PRIMARY KEY,
			email TEXT UNIQUE NOT NULL,
			password_hash TEXT NOT NULL,
			email_verified_at DATETIME,
			is_admin INTEGER DEFAULT 0,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_users_email ON users(email)`,
		`CREATE TABLE IF NOT EXISTS sessions (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token_hash TEXT UNIQUE NOT NULL,
			expires_at DATETIME NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_token_hash ON sessions(token_hash)`,
		`CREATE INDEX IF NOT EXISTS idx_sessions_user_id ON sessions(user_id)`,
		`CREATE TABLE IF NOT EXISTS email_tokens (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			token TEXT UNIQUE NOT NULL,
			purpose TEXT NOT NULL,
			expires_at DATETIME NOT NULL
		)`,
		`CREATE INDEX IF NOT EXISTS idx_email_tokens_token ON email_tokens(token)`,
		`CREATE TABLE IF NOT EXISTS entitlements (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			service_key TEXT NOT NULL,
			source TEXT NOT NULL CHECK(source IN ('stripe','crypto','bundle')),
			status TEXT NOT NULL CHECK(status IN ('active','past_due','canceled','expired')),
			interval TEXT NOT NULL CHECK(interval IN ('month','year')),
			period_start DATETIME NOT NULL,
			period_end DATETIME NOT NULL,
			stripe_customer_id TEXT,
			stripe_subscription_id TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, service_key, source)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_entitlements_user_id ON entitlements(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_entitlements_service_key ON entitlements(service_key)`,
		`CREATE TABLE IF NOT EXISTS payments_crypto (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			service_key TEXT NOT NULL,
			interval TEXT NOT NULL CHECK(interval IN ('month','year')),
			chain TEXT NOT NULL,
			txhash TEXT NOT NULL,
			expected_cad_cents INTEGER NOT NULL,
			expected_amount_native TEXT NOT NULL,
			actual_amount_native TEXT,
			status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','verified','rejected')),
			quoted_rate TEXT,
			submitted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			verified_at DATETIME,
			UNIQUE(chain, txhash)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_payments_crypto_user_id ON payments_crypto(user_id)`,
		`CREATE INDEX IF NOT EXISTS idx_payments_crypto_status ON payments_crypto(status)`,
		`CREATE TABLE IF NOT EXISTS stripe_events (
			id TEXT PRIMARY KEY,
			event_id TEXT UNIQUE NOT NULL,
			type TEXT NOT NULL,
			processed_at DATETIME DEFAULT CURRENT_TIMESTAMP
		)`,
		`CREATE TABLE IF NOT EXISTS audit_log (
			id TEXT PRIMARY KEY,
			at DATETIME DEFAULT CURRENT_TIMESTAMP,
			actor TEXT NOT NULL,
			action TEXT NOT NULL,
			detail_json TEXT
		)`,
		`CREATE TABLE IF NOT EXISTS oauth_accounts (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			provider TEXT NOT NULL CHECK(provider IN ('google','x','github','siwe')),
			provider_account_id TEXT NOT NULL,
			email TEXT,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(provider, provider_account_id)
		)`,
		`CREATE INDEX IF NOT EXISTS idx_oauth_accounts_user ON oauth_accounts(user_id)`,
		`CREATE TABLE IF NOT EXISTS siwe_nonces (
			nonce TEXT PRIMARY KEY,
			address TEXT NOT NULL,
			message TEXT NOT NULL,
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			expires_at DATETIME NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS api_keys (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			service_key TEXT NOT NULL,
			key_hash TEXT NOT NULL UNIQUE,
			status TEXT NOT NULL CHECK(status IN ('active','revoked')),
			created_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			last_used_at DATETIME
		)`,
		`CREATE INDEX IF NOT EXISTS idx_api_keys_user ON api_keys(user_id)`,
		`CREATE TABLE IF NOT EXISTS chat_usage (
			user_id TEXT NOT NULL,
			service_key TEXT NOT NULL,
			month TEXT NOT NULL,
			message_count INTEGER NOT NULL DEFAULT 0,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			PRIMARY KEY (user_id, service_key, month)
		)`,
	}

	for _, stmt := range migrations {
		if _, err := db.Exec(stmt); err != nil {
			return fmt.Errorf("migration failed: %w", err)
		}
	}

	// Extend interval CHECK constraints (SQLite cannot ALTER a CHECK): rebuild the table
	// when the old (month|year-only) constraint is present AND the table is empty —
	// the one-time-purchase tiers need 'lifetime' (entitlements) / 'onetime' (payments).
	if err := rebuildIntervalChecks(db); err != nil {
		return err
	}

	log.Println("Database migrations completed")
	return nil
}

func rebuildIntervalChecks(db *sql.DB) error {
	rebuild := func(table, newDDL, oldMarker string) error {
		var ddl string
		err := db.QueryRow(`SELECT sql FROM sqlite_master WHERE type='table' AND name=?`, table).Scan(&ddl)
		if err != nil {
			return nil // table absent — created above with the new DDL already
		}
		if !strings.Contains(ddl, oldMarker) {
			return nil // already the new schema
		}
		var count int
		if err := db.QueryRow("SELECT COUNT(*) FROM " + table).Scan(&count); err != nil {
			return nil
		}
		if count > 0 {
			log.Printf("WARNING: %s has the pre-onetime CHECK but %d rows — skipping rebuild", table, count)
			return nil
		}
		if _, err := db.Exec("DROP TABLE " + table); err != nil {
			return err
		}
		if _, err := db.Exec(newDDL); err != nil {
			return err
		}
		log.Printf("rebuilt %s with extended interval CHECK", table)
		return nil
	}
	if err := rebuild("entitlements",
		`CREATE TABLE entitlements (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			service_key TEXT NOT NULL,
			source TEXT NOT NULL CHECK(source IN ('stripe','crypto','bundle')),
			status TEXT NOT NULL CHECK(status IN ('active','past_due','canceled','expired')),
			interval TEXT NOT NULL CHECK(interval IN ('month','year','lifetime')),
			period_start DATETIME NOT NULL,
			period_end DATETIME NOT NULL,
			stripe_customer_id TEXT,
			stripe_subscription_id TEXT,
			updated_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			UNIQUE(user_id, service_key, source)
		)`, "IN ('month','year')"); err != nil {
		return err
	}
	return rebuild("payments_crypto",
		`CREATE TABLE payments_crypto (
			id TEXT PRIMARY KEY,
			user_id TEXT NOT NULL REFERENCES users(id) ON DELETE CASCADE,
			service_key TEXT NOT NULL,
			interval TEXT NOT NULL CHECK(interval IN ('month','year','onetime')),
			chain TEXT NOT NULL,
			txhash TEXT NOT NULL,
			expected_cad_cents INTEGER NOT NULL,
			expected_amount_native TEXT NOT NULL,
			actual_amount_native TEXT,
			status TEXT NOT NULL DEFAULT 'pending' CHECK(status IN ('pending','verified','rejected')),
			quoted_rate TEXT,
			submitted_at DATETIME DEFAULT CURRENT_TIMESTAMP,
			verified_at DATETIME,
			UNIQUE(chain, txhash)
		)`, "IN ('month','year')")
}

func GenerateID() string {
	return fmt.Sprintf("%d-%s", time.Now().UnixNano(), randomString(8))
}

func randomString(n int) string {
	const letters = "abcdefghijklmnopqrstuvwxyz0123456789"
	b := make([]byte, n)
	for i := range b {
		b[i] = letters[time.Now().UnixNano()%int64(len(letters))]
		time.Sleep(1) // ensure different values
	}
	return string(b)
}