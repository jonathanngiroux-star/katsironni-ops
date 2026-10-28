package crypto

import (
	"context"
	"database/sql"
	"fmt"
	"sync"
	"time"

	"katsironni-hub/internal/config"
	"katsironni-hub/internal/db"
)

type VerificationService struct {
	db           *db.DB
	chainsConfig *config.ChainsConfig
	evmVerifier  *EVMVerifier
	btcVerifier  *BTCVerifier
	solVerifier  *SolanaVerifier
	tronVerifier *TronVerifier
	xlmVerifier  *StellarVerifier
	rateCache    map[string]cachedRate
	rateMutex    sync.RWMutex
}

type cachedRate struct {
	Rate      float64
	ExpiresAt time.Time
}

func NewVerificationService(database *db.DB, chainsConfig *config.ChainsConfig) *VerificationService {
	return &VerificationService{
		db:           database,
		chainsConfig: chainsConfig,
		evmVerifier:  NewEVMVerifier(),
		btcVerifier:  NewBTCVerifier(),
		solVerifier:  NewSolanaVerifier(),
		tronVerifier: NewTronVerifier(),
		xlmVerifier:  NewStellarVerifier(),
		rateCache:    make(map[string]cachedRate),
	}
}

func (s *VerificationService) GetVerifier(chain *config.ChainConfig) ChainVerifier {
	switch chain.VerifyMethod {
	case "evm":
		return s.evmVerifier
	case "btc":
		return s.btcVerifier
	case "solana":
		return s.solVerifier
	case "tron":
		return s.tronVerifier
	case "stellar":
		return s.xlmVerifier
	default:
		return nil
	}
}

func (s *VerificationService) GetRate(ctx context.Context, rateID string) (float64, error) {
	s.rateMutex.RLock()
	if cached, ok := s.rateCache[rateID]; ok && time.Now().Before(cached.ExpiresAt) {
		s.rateMutex.RUnlock()
		return cached.Rate, nil
	}
	s.rateMutex.RUnlock()

	var rate float64
	var err error
	if rateID != "" {
		rate, err = s.evmVerifier.GetRate(ctx, rateID)
	}

	s.rateMutex.Lock()
	s.rateCache[rateID] = cachedRate{
		Rate:      rate,
		ExpiresAt: time.Now().Add(time.Duration(s.chainsConfig.RateCacheTTLSeconds) * time.Second),
	}
	s.rateMutex.Unlock()

	return rate, err
}

func (s *VerificationService) CalculateExpectedAmount(cadCents int, rate float64) string {
	cadDollars := float64(cadCents) / 100.0
	cryptoAmount := cadDollars / rate
	return fmt.Sprintf("%.18f", cryptoAmount)
}

func (s *VerificationService) SubmitCryptoPayment(ctx context.Context, userID, serviceKey, interval, chainKey, txHash string) (*PaymentResult, error) {
	product, ok := config.GetProduct(serviceKey)
	if !ok {
		return nil, fmt.Errorf("unknown product: %s", serviceKey)
	}
	if !product.Live {
		return nil, fmt.Errorf("service not yet live: %s", serviceKey)
	}

	var expectedCADCents int
	switch interval {
	case "month":
		expectedCADCents = product.MonthlyCents
	case "year":
		expectedCADCents = product.AnnualCents
	case "onetime":
		expectedCADCents = product.OnetimeCents
		if expectedCADCents == 0 {
			return nil, fmt.Errorf("no one-time tier for %s", serviceKey)
		}
	default:
		return nil, fmt.Errorf("invalid interval: %s", interval)
	}

	chain := s.chainsConfig.GetChain(chainKey)
	if chain == nil {
		return nil, fmt.Errorf("unknown chain: %s", chainKey)
	}
	if !chain.Enabled {
		return nil, fmt.Errorf("chain not enabled: %s", chainKey)
	}

	rate, _ := s.GetRate(ctx, chain.RateID)

	var expectedNative string
	if rate > 0 {
		expectedNative = s.CalculateExpectedAmount(expectedCADCents, rate)
	} else {
		expectedNative = "0"
	}

	paymentID := db.GenerateID()
	_, err := s.db.Exec(
		`INSERT INTO payments_crypto (id, user_id, service_key, interval, chain, txhash, expected_cad_cents, expected_amount_native, status, quoted_rate, submitted_at)
		 VALUES (?, ?, ?, ?, ?, ?, ?, ?, 'pending', ?, ?)`,
		paymentID, userID, serviceKey, interval, chainKey, txHash,
		expectedCADCents, expectedNative, fmt.Sprintf("%.8f", rate), time.Now().Format(time.RFC3339),
	)
	if err != nil {
		return nil, fmt.Errorf("create payment: %w", err)
	}

	go s.verifyPaymentAsync(paymentID, chain, txHash, expectedNative, s.chainsConfig.AmountTolerance, s.chainsConfig.SubmissionWindowHours)

	return &PaymentResult{
		PaymentID:        paymentID,
		Status:           "pending",
		ExpectedCADCents: expectedCADCents,
		ExpectedNative:   expectedNative,
		QuotedRate:       rate,
	}, nil
}

type PaymentResult struct {
	PaymentID        string
	Status           string
	ExpectedCADCents int
	ExpectedNative   string
	QuotedRate       float64
}

func (s *VerificationService) verifyPaymentAsync(paymentID string, chain *config.ChainConfig, txHash, expectedNative string, tolerance float64, windowHours int) {
	ctx := context.Background()
	verifier := s.GetVerifier(chain)
	if verifier == nil {
		s.UpdatePaymentStatus(paymentID, "rejected", "no verifier for chain")
		return
	}

	time.Sleep(5 * time.Second)

	maxRetries := (windowHours * 60) / s.chainsConfig.RetryIntervalMinutes
	for i := 0; i < maxRetries; i++ {
		result, err := verifier.VerifyTx(ctx, chain, txHash, expectedNative, chain.Confirmations)
		if err != nil {
			time.Sleep(time.Duration(s.chainsConfig.RetryIntervalMinutes) * time.Minute)
			continue
		}

		if result.Verified {
			s.UpdatePaymentStatus(paymentID, "verified", "")
			s.ActivateEntitlement(paymentID)
			return
		}

		if time.Since(result.Timestamp) > time.Duration(windowHours)*time.Hour {
			s.UpdatePaymentStatus(paymentID, "rejected", "transaction too old")
			return
		}

		if result.Confirmations < chain.Confirmations {
			time.Sleep(time.Duration(s.chainsConfig.RetryIntervalMinutes) * time.Minute)
			continue
		}

		s.UpdatePaymentStatus(paymentID, "rejected", "verification failed")
		return
	}

	s.UpdatePaymentStatus(paymentID, "rejected", "verification timeout")
}

func (s *VerificationService) UpdatePaymentStatus(paymentID, status, errorMsg string) {
	now := time.Now().Format(time.RFC3339)
	if status == "verified" {
		s.db.Exec(`UPDATE payments_crypto SET status = ?, verified_at = ? WHERE id = ?`, status, now, paymentID)
	} else {
		s.db.Exec(`UPDATE payments_crypto SET status = ? WHERE id = ?`, status, paymentID)
	}
}

func (s *VerificationService) ActivateEntitlement(paymentID string) {
	var userID, serviceKey, interval string
	err := s.db.QueryRow(
		`SELECT user_id, service_key, interval FROM payments_crypto WHERE id = ?`,
		paymentID,
	).Scan(&userID, &serviceKey, &interval)
	if err != nil {
		return
	}

	now := time.Now()
	var periodEnd time.Time
	entInterval := interval
	switch interval {
	case "month":
		periodEnd = now.AddDate(0, 1, 0)
	case "onetime":
		// One-time purchase: permanent access, no expiry.
		entInterval = "lifetime"
		periodEnd = now.AddDate(100, 0, 0)
	default:
		periodEnd = now.AddDate(1, 0, 0)
	}

	entitlementID := db.GenerateID()
	s.db.Exec(
		`INSERT INTO entitlements (id, user_id, service_key, source, status, interval, period_start, period_end)
		 VALUES (?, ?, ?, 'crypto', 'active', ?, ?, ?)
		 ON CONFLICT(user_id, service_key, source) DO UPDATE SET
		 status = 'active', period_start = excluded.period_start, period_end = excluded.period_end, updated_at = ?`,
		entitlementID, userID, serviceKey, entInterval,
		now.Format(time.RFC3339), periodEnd.Format(time.RFC3339), now.Format(time.RFC3339),
	)

	// Bundles activate entitlements for every member service (config.BundleMembers).
	for _, svc := range config.BundleMembers[serviceKey] {
		s.db.Exec(
			`INSERT INTO entitlements (id, user_id, service_key, source, status, interval, period_start, period_end)
			 VALUES (?, ?, ?, 'bundle', 'active', ?, ?, ?)
			 ON CONFLICT(user_id, service_key, source) DO UPDATE SET
			 status = 'active', period_start = excluded.period_start, period_end = excluded.period_end, updated_at = ?`,
			db.GenerateID(), userID, svc, entInterval,
			now.Format(time.RFC3339), periodEnd.Format(time.RFC3339), now.Format(time.RFC3339),
		)
	}
}

type PaymentCrypto struct {
	ID                  string
	UserID              string
	ServiceKey          string
	Interval            string
	Chain               string
	TxHash              string
	ExpectedCADCents    int
	ExpectedAmountNative string
	ActualAmountNative  string
	Status              string
	QuotedRate          string
	SubmittedAt         time.Time
	VerifiedAt          time.Time
}

func (s *VerificationService) GetPaymentStatus(paymentID string) (*PaymentCrypto, error) {
	var p PaymentCrypto
	var submittedAtStr, verifiedAtStr sql.NullString
	err := s.db.QueryRow(
		`SELECT id, user_id, service_key, interval, chain, txhash, expected_cad_cents, expected_amount_native, actual_amount_native, status, quoted_rate, submitted_at, verified_at
		 FROM payments_crypto WHERE id = ?`,
		paymentID,
	).Scan(&p.ID, &p.UserID, &p.ServiceKey, &p.Interval, &p.Chain, &p.TxHash,
		&p.ExpectedCADCents, &p.ExpectedAmountNative, &p.ActualAmountNative,
		&p.Status, &p.QuotedRate, &submittedAtStr, &verifiedAtStr)
	if err != nil {
		return nil, err
	}
	if submittedAtStr.Valid {
		p.SubmittedAt, _ = time.Parse(time.RFC3339, submittedAtStr.String)
	}
	if verifiedAtStr.Valid {
		p.VerifiedAt, _ = time.Parse(time.RFC3339, verifiedAtStr.String)
	}
	return &p, nil
}