//go:build integration

package integration

import (
	"encoding/json"
	"io"
	"net/http"
	"os"
	"strings"
	"testing"
)

func baseURL() string {
	if url := os.Getenv("API_URL"); url != "" {
		return url
	}
	return "http://localhost:3000"
}

func TestHealthEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL() + "/api/v1/health")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["status"] != "healthy" {
		t.Errorf("expected healthy status, got %v", result["status"])
	}
}

func TestScanCleanFile(t *testing.T) {
	resp := uploadFile(t, "clean content", "test.txt")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("scan failed: %d - %s", resp.StatusCode, body)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["status"] != "clean" {
		t.Errorf("expected clean status, got %v", result["status"])
	}
}

func TestScanEicar(t *testing.T) {
	// EICAR test string with 'O' replaced by 'x' to avoid triggering local AV
	broken := "X5x!P%@AP[4\\PZX54(P^)7CC)7}$EICAR-STANDARD-ANTIVIRUS-TEST-FILE!$H+H*"
	eicar := strings.Replace(broken, "x", "O", 1)

	resp := uploadFile(t, eicar, "eicar.com")
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("scan failed: %d - %s", resp.StatusCode, body)
	}

	var result map[string]interface{}
	json.NewDecoder(resp.Body).Decode(&result)

	if result["status"] != "infected" {
		t.Errorf("expected infected status, got %v", result["status"])
	}
}

func TestMetricsEndpoint(t *testing.T) {
	resp, err := http.Get(baseURL() + "/metrics")
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("expected 200, got %d", resp.StatusCode)
	}

	body, _ := io.ReadAll(resp.Body)
	content := string(body)

	metrics := []string{"av_http_requests_total", "av_http_request_duration_seconds", "av_scans_total"}
	for _, m := range metrics {
		if !strings.Contains(content, m) {
			t.Errorf("missing metric: %s", m)
		}
	}
}

func uploadFile(t *testing.T, content, filename string) *http.Response {
	t.Helper()

	body := &strings.Builder{}
	boundary := "----TestBoundary"
	body.WriteString("--" + boundary + "\r\n")
	body.WriteString("Content-Disposition: form-data; name=\"file\"; filename=\"" + filename + "\"\r\n")
	body.WriteString("Content-Type: application/octet-stream\r\n\r\n")
	body.WriteString(content + "\r\n")
	body.WriteString("--" + boundary + "--\r\n")

	req, err := http.NewRequest("POST", baseURL()+"/api/v1/scan", strings.NewReader(body.String()))
	if err != nil {
		t.Fatalf("failed to create request: %v", err)
	}
	req.Header.Set("Content-Type", "multipart/form-data; boundary="+boundary)

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	return resp
}
