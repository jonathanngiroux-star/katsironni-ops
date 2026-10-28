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

type TronVerifier struct {
	client *http.Client
}

func NewTronVerifier() *TronVerifier {
	return &TronVerifier{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (v *TronVerifier) VerifyTx(ctx context.Context, chain *config.ChainConfig, txHash, expectedAmountNative string, minConfirmations int) (*TxResult, error) {
	url := fmt.Sprintf("%s/transaction-info?hash=%s", chain.RPCURL, txHash)
	
	req, err := http.NewRequestWithContext(ctx, "GET", url, nil)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	
	resp, err := v.client.Do(req)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()
	
	var result TronTxResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	
	if !result.Success || result.Data == nil {
		return &TxResult{Verified: false, Error: "transaction not found"}, nil
	}
	
	tx := result.Data
	
	// Check recipient - for TRX transfers, check contract calls to our address
	recipientMatch := false
	var actualAmount int64
	
	if tx.ContractData != nil {
		for _, contract := range tx.ContractData {
			if contract.Type == "TransferContract" {
				if contract.Parameter.ToAddress == chain.Address {
					recipientMatch = true
					actualAmount = contract.Parameter.Amount
				}
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
	
	// Check confirmations
	confirmations := int64(tx.Confirmations)
	confirmed := confirmations >= int64(minConfirmations)
	
	verified := recipientMatch && amountOK && confirmed
	
	return &TxResult{
		Verified:       verified,
		ActualAmount:   fmt.Sprintf("%d", actualAmount),
		Confirmations:  int(confirmations),
		RecipientMatch: recipientMatch,
		Timestamp:      time.Unix(tx.Timestamp/1000, 0),
		BlockNumber:    int64(tx.Block),
	}, nil
}

func (v *TronVerifier) GetRate(ctx context.Context, rateID string) (float64, error) {
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

type TronTxResponse struct {
	Success bool `json:"success"`
	Data    *TronTxData `json:"data"`
}

type TronTxData struct {
	TxID          string `json:"txID"`
	Block         int    `json:"block"`
	Timestamp     int64  `json:"timestamp"`
	Confirmations int    `json:"confirmations"`
	ContractData  []TronContract `json:"contractData"`
}

type TronContract struct {
	Type       string `json:"type"`
	Parameter  TronTransferParam `json:"parameter"`
}

type TronTransferParam struct {
	ToAddress string `json:"to_address"`
	Amount    int64  `json:"amount"`
}