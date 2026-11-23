package client

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"entradasya/ms-checkin/internal/model"

	"github.com/google/uuid"
)

// TicketsClient defines the contract for communicating with ms-tickets
type TicketsClient interface {
	ValidateTicket(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error)
}

type HTTPTicketsClient struct {
	baseURL       string
	internalToken string
	httpClient    *http.Client
}

func NewHTTPTicketsClient(baseURL, internalToken string) *HTTPTicketsClient {
	return &HTTPTicketsClient{
		baseURL:       baseURL,
		internalToken: internalToken,
		httpClient: &http.Client{
			Timeout: 10 * time.Second,
		},
	}
}

func (c *HTTPTicketsClient) ValidateTicket(ctx context.Context, req model.ValidateTicketRequest, authToken string) (*model.ValidateTicketResponse, int, error) {
	url := fmt.Sprintf("%s/api/v1/tickets/validate", c.baseURL)

	bodyBytes, err := json.Marshal(req)
	if err != nil {
		return nil, 0, fmt.Errorf("error serializing validate ticket request: %w", err)
	}

	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewBuffer(bodyBytes))
	if err != nil {
		return nil, 0, fmt.Errorf("error creating http request: %w", err)
	}

	httpReq.Header.Set("Content-Type", "application/json; charset=utf-8")
	if c.internalToken != "" {
		httpReq.Header.Set("X-Internal-Token", c.internalToken)
	}
	if authToken != "" {
		httpReq.Header.Set("Authorization", authToken)
	}
	httpReq.Header.Set("X-Correlation-ID", uuid.New().String())

	resp, err := c.httpClient.Do(httpReq)
	if err != nil {
		return nil, 0, fmt.Errorf("error contacting ms-tickets: %w", err)
	}
	defer resp.Body.Close()

	respBytes, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, resp.StatusCode, fmt.Errorf("error reading ms-tickets response body: %w", err)
	}

	var valResp model.ValidateTicketResponse
	if len(respBytes) > 0 {
		if err := json.Unmarshal(respBytes, &valResp); err != nil {
			return nil, resp.StatusCode, fmt.Errorf("error unmarshaling ms-tickets response: %w", err)
		}
	}

	return &valResp, resp.StatusCode, nil
}
