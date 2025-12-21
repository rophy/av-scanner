package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"time"
)

// ValidateRequest is the request body for kube-federated-auth /validate endpoint
type ValidateRequest struct {
	Token   string `json:"token"`
	Cluster string `json:"cluster"`
}

// ValidateResponse is the success response from kube-federated-auth /validate endpoint
// On HTTP 200, it returns decoded JWT claims with Kubernetes metadata
type ValidateResponse struct {
	KubernetesIO *KubernetesMetadata `json:"kubernetes.io,omitempty"`
}

// KubernetesMetadata contains the Kubernetes-specific claims from the token
type KubernetesMetadata struct {
	Namespace      string                `json:"namespace"`
	ServiceAccount *ServiceAccountInfo   `json:"serviceaccount,omitempty"`
}

// ServiceAccountInfo contains service account details from the token
type ServiceAccountInfo struct {
	Name string `json:"name"`
	UID  string `json:"uid"`
}

// ErrorResponse is the error response from kube-federated-auth /validate endpoint
type ErrorResponse struct {
	Error   string `json:"error"`
	Message string `json:"message"`
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
		// HTTP 200 means token is valid, response contains decoded JWT claims
		var validateResp ValidateResponse
		if err := json.NewDecoder(resp.Body).Decode(&validateResp); err != nil {
			return nil, fmt.Errorf("failed to decode response: %w", err)
		}
		if validateResp.KubernetesIO == nil || validateResp.KubernetesIO.ServiceAccount == nil {
			return nil, fmt.Errorf("invalid response: missing kubernetes.io metadata")
		}
		identity := &CallerIdentity{
			Namespace:      validateResp.KubernetesIO.Namespace,
			ServiceAccount: validateResp.KubernetesIO.ServiceAccount.Name,
			UID:            validateResp.KubernetesIO.ServiceAccount.UID,
			Cluster:        c.cluster,
		}
		c.logger.Info("authentication successful",
			"identity", fmt.Sprintf("%s/%s/%s", identity.Cluster, identity.Namespace, identity.ServiceAccount),
		)
		return identity, nil
	case http.StatusUnauthorized, http.StatusBadRequest:
		// Error responses contain error code and message
		var errResp ErrorResponse
		if err := json.NewDecoder(resp.Body).Decode(&errResp); err != nil {
			return nil, fmt.Errorf("auth failed: status %d", resp.StatusCode)
		}
		return nil, fmt.Errorf("%s: %s", errResp.Error, errResp.Message)
	default:
		body, _ := io.ReadAll(resp.Body)
		c.logger.Error("auth service error",
			"status", resp.StatusCode,
			"body", string(body),
		)
		return nil, fmt.Errorf("auth service error: status %d", resp.StatusCode)
	}
}
