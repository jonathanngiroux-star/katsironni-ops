package crypto

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"strings"
	"time"

	"katsironni-hub/internal/config"
)

type SolanaVerifier struct {
	client *http.Client
}

func NewSolanaVerifier() *SolanaVerifier {
	return &SolanaVerifier{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (v *SolanaVerifier) VerifyTx(ctx context.Context, chain *config.ChainConfig, txHash, expectedAmountNative string, minConfirmations int) (*TxResult, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "getTransaction",
		"params":  []interface{}{txHash, map[string]string{"encoding": "jsonParsed", "commitment": "finalized"}},
		"id":      1,
	}

	data, err := json.Marshal(reqBody)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}

	req, err := http.NewRequestWithContext(ctx, "POST", chain.RPCURL, strings.NewReader(string(data)))
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	defer resp.Body.Close()

	var result SolanaTxResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}

	if result.Error != nil {
		return &TxResult{Verified: false, Error: result.Error.Message}, nil
	}
	if result.Result == nil {
		return &TxResult{Verified: false, Error: "transaction not found"}, nil
	}

	tx := result.Result

	// Check recipient - look for transfer to our address in parsed instructions
	recipientMatch := false
	var actualAmount int64

	if tx.Meta != nil && tx.Meta.PostTokenBalances != nil {
		for _, balance := range tx.Meta.PostTokenBalances {
			if balance.Owner == chain.Address {
				recipientMatch = true
				// Parse amount from uiAmount
				if balance.UiTokenAmount != nil && balance.UiTokenAmount.UiAmount != nil {
					actualAmount = int64(*balance.UiTokenAmount.UiAmount * 1e9) // SOL to lamports
				}
			}
		}
	}

	// Also check native SOL transfers
	if tx.Transaction != nil && tx.Transaction.Message != nil {
		for _, inst := range tx.Transaction.Message.Instructions {
			if parsed, ok := inst.(map[string]interface{}); ok {
				if program, _ := parsed["program"].(string); program == "system" {
					if parsedData, ok := parsed["parsed"].(map[string]interface{}); ok {
						if info, ok := parsedData["info"].(map[string]interface{}); ok {
							if dest, ok := info["destination"].(string); ok && dest == chain.Address {
								recipientMatch = true
								if lamports, ok := info["lamports"].(float64); ok {
									actualAmount = int64(lamports)
								}
							}
						}
					}
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

	// Check confirmations - finalized means confirmed
	confirmed := tx.Meta != nil && tx.Meta.Err == nil
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
		Timestamp:      time.Unix(tx.BlockTime, 0),
	}, nil
}

func (v *SolanaVerifier) GetRate(ctx context.Context, rateID string) (float64, error) {
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

type SolanaTxResponse struct {
	Jsonrpc string        `json:"jsonrpc"`
	Result  *SolanaTx     `json:"result"`
	Error   *RPCError     `json:"error"`
	ID      int           `json:"id"`
}

type SolanaTx struct {
	BlockTime int64 `json:"blockTime"`
	Meta      *SolanaMeta `json:"meta"`
	Transaction *SolanaTransaction `json:"transaction"`
}

type SolanaMeta struct {
	Err              interface{} `json:"err"`
	PostTokenBalances []SolanaTokenBalance `json:"postTokenBalances"`
}

type SolanaTokenBalance struct {
	Owner         string `json:"owner"`
	UiTokenAmount *SolanaUiTokenAmount `json:"uiTokenAmount"`
}

type SolanaUiTokenAmount struct {
	UiAmount *float64 `json:"uiAmount"`
}

type SolanaTransaction struct {
	Message *SolanaMessage `json:"message"`
}

type SolanaMessage struct {
	Instructions []interface{} `json:"instructions"`
}