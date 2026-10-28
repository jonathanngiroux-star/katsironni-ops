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

type ChainVerifier interface {
	VerifyTx(ctx context.Context, chain *config.ChainConfig, txHash, expectedAmountNative string, minConfirmations int) (*TxResult, error)
	GetRate(ctx context.Context, rateID string) (float64, error)
}

type TxResult struct {
	Verified        bool
	ActualAmount    string
	Confirmations   int
	RecipientMatch  bool
	Timestamp       time.Time
	BlockNumber     int64
	Error           string
}

type EVMVerifier struct {
	client *http.Client
}

func NewEVMVerifier() *EVMVerifier {
	return &EVMVerifier{
		client: &http.Client{Timeout: 15 * time.Second},
	}
}

func (v *EVMVerifier) VerifyTx(ctx context.Context, chain *config.ChainConfig, txHash, expectedAmountNative string, minConfirmations int) (*TxResult, error) {
	// Get transaction
	tx, err := v.getTransaction(ctx, chain.RPCURL, txHash)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}

	// Get receipt for confirmations
	receipt, err := v.getTransactionReceipt(ctx, chain.RPCURL, txHash)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}

	// Check recipient (to address)
	recipientMatch := strings.EqualFold(tx.To, chain.Address)

	// Check amount
	amountOK := true
	if expectedAmountNative != "" {
		expected := new(big.Int)
		expected.SetString(expectedAmountNative, 10)
		actual := new(big.Int)
		actual.SetString(tx.Value, 10)
		amountOK = actual.Cmp(expected) >= 0
	}

	// Check confirmations
	currentBlock, err := v.getBlockNumber(ctx, chain.RPCURL)
	if err != nil {
		return &TxResult{Verified: false, Error: err.Error()}, nil
	}
	confirmations := currentBlock - receipt.BlockNumber
	confirmed := confirmations >= int64(minConfirmations)

	verified := recipientMatch && amountOK && confirmed

	return &TxResult{
		Verified:       verified,
		ActualAmount:   tx.Value,
		Confirmations:  int(confirmations),
		RecipientMatch: recipientMatch,
		Timestamp:      time.Unix(tx.Timestamp, 0),
		BlockNumber:    receipt.BlockNumber,
	}, nil
}

func (v *EVMVerifier) getTransaction(ctx context.Context, rpcURL, txHash string) (*EVMTransaction, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_getTransactionByHash",
		"params":  []string{txHash},
		"id":      1,
	}
	result, err := v.rpcCall(ctx, rpcURL, reqBody, func(data []byte) (interface{}, error) {
		var resp struct {
			Result *EVMTransaction `json:"result"`
			Error  *RPCError       `json:"error"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, err
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("RPC error: %s", resp.Error.Message)
		}
		if resp.Result == nil {
			return nil, fmt.Errorf("transaction not found")
		}
		return resp.Result, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*EVMTransaction), nil
}

func (v *EVMVerifier) getTransactionReceipt(ctx context.Context, rpcURL, txHash string) (*EVMReceipt, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_getTransactionReceipt",
		"params":  []string{txHash},
		"id":      1,
	}
	result, err := v.rpcCall(ctx, rpcURL, reqBody, func(data []byte) (interface{}, error) {
		var resp struct {
			Result *EVMReceipt `json:"result"`
			Error  *RPCError   `json:"error"`
		}
		if err := json.Unmarshal(data, &resp); err != nil {
			return nil, err
		}
		if resp.Error != nil {
			return nil, fmt.Errorf("RPC error: %s", resp.Error.Message)
		}
		if resp.Result == nil {
			return nil, fmt.Errorf("receipt not found")
		}
		return resp.Result, nil
	})
	if err != nil {
		return nil, err
	}
	return result.(*EVMReceipt), nil
}

func (v *EVMVerifier) getBlockNumber(ctx context.Context, rpcURL string) (int64, error) {
	reqBody := map[string]interface{}{
		"jsonrpc": "2.0",
		"method":  "eth_blockNumber",
		"params":  []interface{}{},
		"id":      1,
	}
	var resp struct {
		Result string `json:"result"`
		Error  *RPCError `json:"error"`
	}
	if err := v.rpcCallRaw(ctx, rpcURL, reqBody, &resp); err != nil {
		return 0, err
	}
	if resp.Error != nil {
		return 0, fmt.Errorf("RPC error: %s", resp.Error.Message)
	}
	blockNum := new(big.Int)
	blockNum.SetString(resp.Result[2:], 16) // Remove 0x prefix
	return blockNum.Int64(), nil
}

func (v *EVMVerifier) GetRate(ctx context.Context, rateID string) (float64, error) {
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

func (v *EVMVerifier) rpcCall(ctx context.Context, rpcURL string, reqBody interface{}, parse func([]byte) (interface{}, error)) (interface{}, error) {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return nil, err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, strings.NewReader(string(data)))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := v.client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()

	body := make([]byte, 0, 4096)
	buf := make([]byte, 1024)
	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			body = append(body, buf[:n]...)
		}
		if err != nil {
			break
		}
	}

	return parse(body)
}

func (v *EVMVerifier) rpcCallRaw(ctx context.Context, rpcURL string, reqBody interface{}, resp interface{}) error {
	data, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, "POST", rpcURL, strings.NewReader(string(data)))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	httpResp, err := v.client.Do(req)
	if err != nil {
		return err
	}
	defer httpResp.Body.Close()

	return json.NewDecoder(httpResp.Body).Decode(resp)
}

type EVMTransaction struct {
	Hash        string `json:"hash"`
	From        string `json:"from"`
	To          string `json:"to"`
	Value       string `json:"value"`
	BlockNumber string `json:"blockNumber"`
	Timestamp   int64  `json:"timestamp"`
}

type EVMReceipt struct {
	BlockNumber       int64  `json:"blockNumber,string"`
	TransactionHash   string `json:"transactionHash"`
	Status            string `json:"status"`
}

type RPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}