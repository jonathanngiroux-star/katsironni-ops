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

type BTCVerifier struct {
	client *http.Client
}

func NewBTCVerifier() *BTCVerifier {
	return &BTCVerifier{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (v *BTCVerifier) VerifyTx(ctx context.Context, chain *config.ChainConfig, txHash, expectedAmountNative string, minConfirmations int) (*TxResult, error) {
	url := fmt.Sprintf("%s/tx/%s", chain.RPCURL, txHash)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	
	resp, err := v.client.Do(req)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()
	
	var tx BTCTransaction
	if err := json.NewDecoder(resp.Body).Decode(&tx); err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	
	// Check recipient
	recipientMatch := false
	for _, out := range tx.Vout {
		if out.ScriptPubKey.Address == chain.Address {
			recipientMatch = true
			break
		}
	}
	
	// Check amount
	amountOK := true
	if expectedAmountNative != "" {
		expected := new(big.Int)
		expected.SetString(expectedAmountNative, 10)
		// Sum all outputs to our address
		var total int64
		for _, out := range tx.Vout {
			if out.ScriptPubKey.Address == chain.Address {
				total += int64(out.Value * 100000000) // Convert BTC to satoshi
			}
		}
		actual := big.NewInt(total)
		amountOK = actual.Cmp(expected) >= 0
	}
	
	// Get confirmations from status endpoint
	statusURL := fmt.Sprintf("%s/tx/%s/status", chain.RPCURL, txHash)
	statusReq, _ := http.NewRequestWithContext(ctx, "GET", statusURL, nil)
	statusResp, err := v.client.Do(statusReq)
	var confirmations int64
	if err == nil && statusResp != nil {
		defer statusResp.Body.Close()
		var status BTCStatus
		if err := json.NewDecoder(statusResp.Body).Decode(&status); err == nil {
			confirmations = int64(status.Confirmations)
		}
	}
	
	confirmed := confirmations >= int64(minConfirmations)
	verified := recipientMatch && amountOK && confirmed
	
	return &TxResult{
		Verified:       verified,
		ActualAmount:   fmt.Sprintf("%d", totalSatoshi(tx)),
		Confirmations:  int(confirmations),
		RecipientMatch: recipientMatch,
		Timestamp:      time.Unix(tx.Status.BlockTime, 0),
		BlockNumber:    int64(tx.Status.BlockHeight),
	}, nil
}

func totalSatoshi(tx BTCTransaction) int64 {
	var total int64
	for _, out := range tx.Vout {
		total += int64(out.Value * 100000000)
	}
	return total
}

type BTCTransaction struct {
	Txid     string `json:"txid"`
	Vout     []BTCVout `json:"vout"`
	Status   BTCStatus `json:"status"`
}

type BTCVout struct {
	Value        float64 `json:"value"`
	ScriptPubKey BTCScriptPubKey `json:"scriptpubkey"`
}

type BTCScriptPubKey struct {
	Address string `json:"address"`
}

type BTCStatus struct {
	Confirmed   bool `json:"confirmed"`
	BlockHeight int  `json:"block_height"`
	Confirmations int `json:"confirmations"`
	BlockTime   int64 `json:"block_time"`
}

func (v *BTCVerifier) GetRate(ctx context.Context, rateID string) (float64, error) {
	// Use CoinGecko
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