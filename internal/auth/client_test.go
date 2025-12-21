package auth

import (
	"context"
	"encoding/json"
	"log/slog"
	"net/http"
	"net/http/httptest"
	"os"
	"testing"
	"time"
)

func testLogger() *slog.Logger {
	return slog.New(slog.NewTextHandler(os.Stdout, &slog.HandlerOptions{Level: slog.LevelError}))
}

func TestClient_Validate_Success(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost {
			t.Errorf("expected POST, got %s", r.Method)
		}
		if r.URL.Path != "/validate" {
			t.Errorf("expected /validate, got %s", r.URL.Path)
		}

		var req ValidateRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			t.Fatalf("failed to decode request: %v", err)
		}

		if req.Token != "test-token" {
			t.Errorf("expected token 'test-token', got '%s'", req.Token)
		}
		if req.Cluster != "test-cluster" {
			t.Errorf("expected cluster 'test-cluster', got '%s'", req.Cluster)
		}

		w.Header().Set("Content-Type", "application/json")
		// Response matches actual kube-federated-auth format
		json.NewEncoder(w).Encode(ValidateResponse{
			KubernetesIO: &KubernetesMetadata{
				Namespace: "test-ns",
				ServiceAccount: &ServiceAccountInfo{
					Name: "test-sa",
					UID:  "test-uid",
				},
			},
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-cluster", 5*time.Second, testLogger())
	identity, err := client.Validate(context.Background(), "test-token")

	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if identity.Namespace != "test-ns" {
		t.Errorf("expected namespace 'test-ns', got '%s'", identity.Namespace)
	}
	if identity.ServiceAccount != "test-sa" {
		t.Errorf("expected serviceAccount 'test-sa', got '%s'", identity.ServiceAccount)
	}
	if identity.UID != "test-uid" {
		t.Errorf("expected uid 'test-uid', got '%s'", identity.UID)
	}
	if identity.Cluster != "test-cluster" {
		t.Errorf("expected cluster 'test-cluster', got '%s'", identity.Cluster)
	}
}

func TestClient_Validate_InvalidToken(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "invalid_signature",
			Message: "Token signature verification failed",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-cluster", 5*time.Second, testLogger())
	_, err := client.Validate(context.Background(), "invalid-token")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "invalid_signature: Token signature verification failed" {
		t.Errorf("expected 'invalid_signature' error, got '%s'", err.Error())
	}
}

func TestClient_Validate_UnknownCluster(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusBadRequest)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "cluster_not_found",
			Message: "Unknown cluster: unknown-cluster",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "unknown-cluster", 5*time.Second, testLogger())
	_, err := client.Validate(context.Background(), "test-token")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "cluster_not_found: Unknown cluster: unknown-cluster" {
		t.Errorf("expected 'cluster_not_found' error, got '%s'", err.Error())
	}
}

func TestClient_Validate_ServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-cluster", 5*time.Second, testLogger())
	_, err := client.Validate(context.Background(), "test-token")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "auth service error: status 500" {
		t.Errorf("expected 'auth service error' error, got '%s'", err.Error())
	}
}

func TestClient_Validate_TokenExpired(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusUnauthorized)
		json.NewEncoder(w).Encode(ErrorResponse{
			Error:   "token_expired",
			Message: "Token has expired",
		})
	}))
	defer server.Close()

	client := NewClient(server.URL, "test-cluster", 5*time.Second, testLogger())
	_, err := client.Validate(context.Background(), "expired-token")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
	if err.Error() != "token_expired: Token has expired" {
		t.Errorf("expected 'token_expired' error, got '%s'", err.Error())
	}
}

func TestClient_Validate_NetworkError(t *testing.T) {
	client := NewClient("http://localhost:99999", "test-cluster", 1*time.Second, testLogger())
	_, err := client.Validate(context.Background(), "test-token")

	if err == nil {
		t.Fatal("expected error, got nil")
	}
}
