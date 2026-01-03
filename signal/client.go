package signal

import (
	"bufio"
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"sync/atomic"

	"tron"
)

type Client struct {
	baseURL    string
	botAccount string
	httpClient *http.Client
	reqID      atomic.Int64
}

type jsonRPCRequest struct {
	JSONRPC string      `json:"jsonrpc"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
	ID      int64       `json:"id"`
}

type jsonRPCResponse struct {
	JSONRPC string          `json:"jsonrpc"`
	Result  json.RawMessage `json:"result,omitempty"`
	Error   *jsonRPCError   `json:"error,omitempty"`
	ID      int64           `json:"id"`
}

type jsonRPCError struct {
	Code    int    `json:"code"`
	Message string `json:"message"`
}

type sendParams struct {
	Account   string   `json:"account"`
	Recipient []string `json:"recipient,omitempty"`
	GroupID   string   `json:"groupId,omitempty"`
	Message   string   `json:"message"`
}

type envelope struct {
	Envelope struct {
		Source       string `json:"source"`
		SourceUUID   string `json:"sourceUuid"`
		SourceNumber string `json:"sourceNumber"`
		SourceName   string `json:"sourceName"`
		Account      string `json:"account"`
		DataMessage  *struct {
			Message          string `json:"message"`
			Timestamp        int64  `json:"timestamp"`
			ExpiresInSeconds int    `json:"expiresInSeconds"`
			GroupInfo        *struct {
				GroupID string `json:"groupId"`
				Type    string `json:"type"`
			} `json:"groupInfo"`
		} `json:"dataMessage"`
	} `json:"envelope"`
}

func NewClient(baseURL, botAccount string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		botAccount: botAccount,
		httpClient: &http.Client{},
	}
}

func (c *Client) SendMessage(recipient, message string) error {
	params := sendParams{
		Account:   c.botAccount,
		Recipient: []string{recipient},
		Message:   message,
	}

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "send",
		Params:  params,
		ID:      c.reqID.Add(1),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/v1/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if rpcResp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return nil
}

func (c *Client) SendGroupMessage(groupID, message string) error {
	params := sendParams{
		Account: c.botAccount,
		GroupID: groupID,
		Message: message,
	}

	req := jsonRPCRequest{
		JSONRPC: "2.0",
		Method:  "send",
		Params:  params,
		ID:      c.reqID.Add(1),
	}

	body, err := json.Marshal(req)
	if err != nil {
		return fmt.Errorf("marshal request: %w", err)
	}

	resp, err := c.httpClient.Post(c.baseURL+"/api/v1/rpc", "application/json", bytes.NewReader(body))
	if err != nil {
		return fmt.Errorf("send request: %w", err)
	}
	defer resp.Body.Close()

	var rpcResp jsonRPCResponse
	if err := json.NewDecoder(resp.Body).Decode(&rpcResp); err != nil {
		return fmt.Errorf("decode response: %w", err)
	}

	if rpcResp.Error != nil {
		return fmt.Errorf("rpc error %d: %s", rpcResp.Error.Code, rpcResp.Error.Message)
	}

	return nil
}

func (c *Client) SubscribeMessages(ctx context.Context) <-chan tron.IncomingMessage {
	ch := make(chan tron.IncomingMessage, 10)

	go func() {
		defer close(ch)

		for {
			select {
			case <-ctx.Done():
				return
			default:
			}

			if err := c.streamEvents(ctx, ch); err != nil {
				select {
				case <-ctx.Done():
					return
				default:
				}
			}
		}
	}()

	return ch
}

func (c *Client) streamEvents(ctx context.Context, ch chan<- tron.IncomingMessage) error {
	req, err := http.NewRequestWithContext(ctx, "GET", c.baseURL+"/api/v1/events", nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "text/event-stream")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data:") {
			continue
		}

		data := strings.TrimPrefix(line, "data:")
		data = strings.TrimSpace(data)
		if data == "" {
			continue
		}

		var env envelope
		if err := json.Unmarshal([]byte(data), &env); err != nil {
			continue
		}

		if env.Envelope.DataMessage == nil || env.Envelope.DataMessage.Message == "" {
			continue
		}

		if c.isSelfMessage(env) {
			continue
		}

		msg := tron.IncomingMessage{
			Source:           env.Envelope.Source,
			SourceUUID:       env.Envelope.SourceUUID,
			SourceNumber:     env.Envelope.SourceNumber,
			SourceName:       env.Envelope.SourceName,
			Message:          env.Envelope.DataMessage.Message,
			Timestamp:        env.Envelope.DataMessage.Timestamp,
			ExpiresInSeconds: env.Envelope.DataMessage.ExpiresInSeconds,
		}

		if env.Envelope.DataMessage.GroupInfo != nil {
			msg.GroupID = env.Envelope.DataMessage.GroupInfo.GroupID
			msg.IsGroup = true
		}

		ch <- msg
	}

	return scanner.Err()
}

func (c *Client) isSelfMessage(env envelope) bool {
	botAccount := strings.TrimPrefix(c.botAccount, "+")
	botAccount = strings.TrimPrefix(botAccount, "u:")
	botAccount = strings.ToLower(botAccount)

	sources := []string{
		env.Envelope.Source,
		env.Envelope.SourceUUID,
		env.Envelope.SourceNumber,
	}

	for _, src := range sources {
		src = strings.TrimPrefix(src, "+")
		src = strings.TrimPrefix(src, "u:")
		if strings.ToLower(src) == botAccount {
			return true
		}
	}

	return false
}
