package crypto

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"time"

	"katsironni-hub/internal/config"
)

type StellarVerifier struct {
	client *http.Client
}

func NewStellarVerifier() *StellarVerifier {
	return &StellarVerifier{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (v *StellarVerifier) VerifyTx(ctx context.Context, chain *config.ChainConfig, txHash, expectedAmountNative string, minConfirmations int) (*TxResult, error) {
	url := fmt.Sprintf("%s/transactions/%s", chain.RPCURL, txHash)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	
	resp, err := v.client.Do(req)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()
	
	var tx StellarTx
	if err := json.NewDecoder(resp.Body).Decode(&tx); err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	
	// Check recipient and amount from operations
	recipientMatch := false
	var actualAmount int64
	
	for _, op := range tx.Operations {
		if op.Type == "payment" && op.To == chain.Address {
			recipientMatch = true
			// Parse amount - in stroops (1 XLM = 10^7 stroops)
			if op.Amount != "" {
				amountFloat := 0.0
				fmt.Sscanf(op.Amount, "%f", &amountFloat)
				actualAmount = int64(amountFloat * 10000000)
			}
		}
	}
	
	// Check amount
	amountOK := true
	if expectedAmountNative != "" && actualAmount > 0 {
		expected := new(big.Int)
		expected.SetString(expectedAmountNative, 10)
		actual := big.NewInt(actualAmount)
		amountOK = actual.Cmp(expected) >= 0
	}
	
	// Stellar has instant finality - 1 confirmation when in ledger
	confirmed := tx.Successful
	confirmations := int64(1)
	if !confirmed {
		confirmations = 0
	}
	
	verified := recipientMatch && amountOK && confirmed
	
	return &TxResult{
		Verified:       verified,
		ActualAmount:   fmt.Sprintf("%d", actualAmount),
		Confirmations:  int(confirmations),
		RecipientMatch: recipientMatch,
		Timestamp:      parseStellarTime(tx.CreatedAt),
		BlockNumber:    int64(tx.Ledger),
	}, nil
}

func (v *StellarVerifier) GetRate(ctx context.Context, rateID string) (float64, error) {
	url := fmt.Sprintf("https://api.coingecko.com/api/v3/simple/price?ids=%s&vs_currencies=cad", rateID)
	req, _ := http.NewRequestWithContext(ctx, "GET", url, nil)
	resp, err := v.client.Do(req)
	if err != nil {
		return 0, err
	}
	defer resp.Body.Close()
	
	var result map[string]map[string]float64
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return 0, err
	}
	return result[rateID]["cad"], nil
}

type StellarTx struct {
	Hash        string `json:"hash"`
	Ledger      int    `json:"ledger"`
	CreatedAt   string `json:"created_at"`
	Successful  bool   `json:"successful"`
	Operations  []StellarOperation `json:"operations"`
}

type StellarOperation struct {
	Type   string `json:"type"`
	To     string `json:"to"`
	From   string `json:"from"`
	Amount string `json:"amount"`
	Asset  string `json:"asset_type"`
}

func parseStellarTime(s string) time.Time {
	// Stellar uses ISO 8601 format like "2024-01-15T12:30:45Z"
	t, _ := time.Parse(time.RFC3339, s)
	return t
}