//go:build integration

package integration

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
	"os"
	"os/exec"
	"strings"
	"testing"
)

func authServiceURL() string {
	if url := os.Getenv("AUTH_SERVICE_URL"); url != "" {
		return url
	}
	return "http://127.0.0.1:8082"
}

func authClusterName() string {
	if cluster := os.Getenv("AUTH_CLUSTER_NAME"); cluster != "" {
		return cluster
	}
	return "minikube"
}

// getServiceAccountToken creates a token for the given namespace/serviceaccount
// If TEST_SA_TOKEN env var is set, it uses that instead of calling kubectl
func getServiceAccountToken(t *testing.T, namespace, serviceAccount string) string {
	t.Helper()

	// Use pre-created token if available
	if token := os.Getenv("TEST_SA_TOKEN"); token != "" {
		return token
	}

	cmd := exec.Command("kubectl", "-n", namespace, "create", "token", serviceAccount, "--duration=1h")
	output, err := cmd.Output()
	if err != nil {
		t.Fatalf("failed to create token for %s/%s: %v (set TEST_SA_TOKEN env var to provide token)", namespace, serviceAccount, err)
	}
	return strings.TrimSpace(string(output))
}

// TestAuthService_ValidateEndpoint tests that kube-federated-auth is working
func TestAuthService_ValidateEndpoint(t *testing.T) {
	token := getServiceAccountToken(t, "test-client", "scanner-client")

	reqBody, _ := json.Marshal(map[string]string{
		"token":   token,
		"cluster": authClusterName(),
	})

	resp, err := http.Post(authServiceURL()+"/validate", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to call auth service: %v", err)
	}
	defer resp.Body.Close()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", resp.StatusCode, body)
	}

	var result map[string]interface{}
	if err := json.Unmarshal(body, &result); err != nil {
		t.Fatalf("failed to unmarshal response: %v, body: %s", err, body)
	}

	kubeInfo, ok := result["kubernetes.io"].(map[string]interface{})
	if !ok {
		t.Fatalf("kubernetes.io field missing or invalid type, response: %s", body)
	}

	if kubeInfo["namespace"] != "test-client" {
		t.Errorf("expected namespace 'test-client', got %v", kubeInfo["namespace"])
	}

	saInfo, ok := kubeInfo["serviceaccount"].(map[string]interface{})
	if !ok {
		t.Fatalf("serviceaccount field missing or invalid type")
	}

	if saInfo["name"] != "scanner-client" {
		t.Errorf("expected serviceAccount 'scanner-client', got %v", saInfo["name"])
	}
}

// TestAuthService_InvalidToken tests that invalid tokens are rejected
func TestAuthService_InvalidToken(t *testing.T) {
	reqBody, _ := json.Marshal(map[string]string{
		"token":   "invalid-token",
		"cluster": authClusterName(),
	})

	resp, err := http.Post(authServiceURL()+"/validate", "application/json", bytes.NewReader(reqBody))
	if err != nil {
		t.Fatalf("failed to call auth service: %v", err)
	}
	defer resp.Body.Close()

	// kube-federated-auth returns 500 for validation errors
	if resp.StatusCode == http.StatusOK {
		t.Fatal("expected non-200 status for invalid token")
	}
}

// TestAuthMiddleware_AllowedServiceAccount tests that allowed service accounts can access protected endpoints
func TestAuthMiddleware_AllowedServiceAccount(t *testing.T) {
	// Skip if AUTH_ENABLED is not set
	if os.Getenv("AUTH_ENABLED") != "true" {
		t.Skip("AUTH_ENABLED not set, skipping auth middleware test")
	}

	token := getServiceAccountToken(t, "test-client", "scanner-client")

	req, _ := http.NewRequest("GET", baseURL()+"/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 200 for allowed service account, got %d: %s", resp.StatusCode, body)
	}
}

// TestAuthMiddleware_DeniedServiceAccount tests that non-allowlisted service accounts are rejected
func TestAuthMiddleware_DeniedServiceAccount(t *testing.T) {
	// Skip if AUTH_ENABLED is not set
	if os.Getenv("AUTH_ENABLED") != "true" {
		t.Skip("AUTH_ENABLED not set, skipping auth middleware test")
	}

	// Create a service account that's not in the allowlist
	exec.Command("kubectl", "create", "namespace", "denied-client").Run()
	exec.Command("kubectl", "-n", "denied-client", "create", "serviceaccount", "denied-sa").Run()

	token := getServiceAccountToken(t, "denied-client", "denied-sa")

	req, _ := http.NewRequest("GET", baseURL()+"/api/v1/health", nil)
	req.Header.Set("Authorization", "Bearer "+token)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusForbidden {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 403 for denied service account, got %d: %s", resp.StatusCode, body)
	}
}

// TestAuthMiddleware_MissingToken tests that requests without tokens are rejected
func TestAuthMiddleware_MissingToken(t *testing.T) {
	// Skip if AUTH_ENABLED is not set
	if os.Getenv("AUTH_ENABLED") != "true" {
		t.Skip("AUTH_ENABLED not set, skipping auth middleware test")
	}

	req, _ := http.NewRequest("GET", baseURL()+"/api/v1/health", nil)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusUnauthorized {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("expected 401 for missing token, got %d: %s", resp.StatusCode, body)
	}
}

// TestAuthMiddleware_ProbeEndpointsSkipAuth tests that liveness/readiness probes don't require auth
func TestAuthMiddleware_ProbeEndpointsSkipAuth(t *testing.T) {
	// Skip if AUTH_ENABLED is not set
	if os.Getenv("AUTH_ENABLED") != "true" {
		t.Skip("AUTH_ENABLED not set, skipping auth middleware test")
	}

	probeEndpoints := []string{"/api/v1/live", "/api/v1/ready"}

	for _, endpoint := range probeEndpoints {
		t.Run(endpoint, func(t *testing.T) {
			req, _ := http.NewRequest("GET", baseURL()+endpoint, nil)
			// No Authorization header

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				t.Fatalf("request failed: %v", err)
			}
			defer resp.Body.Close()

			// Should succeed even without auth
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(resp.Body)
				t.Errorf("expected 200 for %s without auth, got %d: %s", endpoint, resp.StatusCode, body)
			}
		})
	}
}
