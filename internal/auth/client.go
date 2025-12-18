package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"time"
)

// ValidateRequest is the request body for kube-federated-auth /validate endpoint
type ValidateRequest struct {
	Token   string `json:"token"`
	Cluster string `json:"cluster"`
}

// ValidateResponse is the response from kube-federated-auth /validate endpoint
type ValidateResponse struct {
	Valid          bool   `json:"valid"`
	Namespace      string `json:"namespace"`
	ServiceAccount string `json:"serviceAccount"`
	UID            string `json:"uid,omitempty"`
	Error          string `json:"error,omitempty"`
}

// CallerIdentity represents the authenticated caller information
type CallerIdentity struct {
	Namespace      string
	ServiceAccount string
	UID            string
	Cluster        string
}

// Client handles authentication with kube-federated-auth service
type Client struct {
	baseURL    string
	cluster    string
	httpClient *http.Client
	logger     *slog.Logger
}

// NewClient creates a new auth client
func NewClient(baseURL, cluster string, timeout time.Duration, logger *slog.Logger) *Client {
	return &Client{
		baseURL: baseURL,
		cluster: cluster,
		httpClient: &http.Client{
			Timeout: timeout,
		},
		logger: logger,
	}
}

// Validate validates a token against kube-federated-auth
func (c *Client) Validate(ctx context.Context, token string) (*CallerIdentity, error) {
	reqBody := ValidateRequest{
		Token:   token,
		Cluster: c.cluster,
	}

	bodyBytes, err := json.Marshal(reqBody)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+"/validate", bytes.NewReader(bodyBytes))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("failed to call auth service: %w", err)
	}
	defer resp.Body.Close()

	switch resp.StatusCode {
	case http.StatusOK:
		var validateResp ValidateResponse
		if err := json.NewDecoder(resp.Body).Decode(&validateResp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		if !validateResp.Valid {
			return nil, fmt.Errorf("token validation failed: %s", validateResp.Error)
		}
		return &CallerIdentity{
			Namespace:      validateResp.Namespace,
			ServiceAccount: validateResp.ServiceAccount,
			UID:            validateResp.UID,
			Cluster:        c.cluster,
		}, nil
	case http.StatusUnauthorized:
		return nil, fmt.Errorf("invalid token")
	case http.StatusBadRequest:
		return nil, fmt.Errorf("unknown cluster: %s", c.cluster)
	default:
		return nil, fmt.Errorf("auth service error: status %d", resp.StatusCode)
	}
}
