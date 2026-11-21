package aiagent

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"
)

func TestRunHeadlessInferenceHonorsContextDeadlineAndDecodesErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/internal/v1/inference" {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		if r.Header.Get("Accept") != "application/json" {
			t.Errorf("Accept = %q, want application/json", r.Header.Get("Accept"))
		}
		var req HeadlessInferenceRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			w.WriteHeader(http.StatusBadRequest)
			return
		}
		if req.ActorUserID == "usr_no_money" {
			w.WriteHeader(http.StatusUnprocessableEntity)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"error": map[string]any{"code": "ai.wallet_balance_insufficient"},
			})
			return
		}
		// Sleep longer than client's base 30ms timeout to prove UseContextDeadline bypasses fixed client timeout.
		time.Sleep(80 * time.Millisecond)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"runId":     "airun_test",
			"taskType":  req.TaskType,
			"modelId":   "aimod_1",
			"modelName": "gemini-test",
			"output":    map[string]any{"summary": "ok"},
			"usage":     map[string]any{"inputTokens": 10, "outputTokens": 5, "totalTokens": 15},
		})
	}))
	defer srv.Close()

	// Base client timeout is 30ms, while context deadline is 500ms.
	client, err := NewHTTPClientWithTimeout(srv.URL, "svc-token-32-bytes-minimum-test!", "actor-key-32-bytes-minimum-test!", 30*time.Millisecond)
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()
	got, err := RunHeadlessInference(ctx, client, ActorContext{UserID: "usr_ok"}, HeadlessInferenceRequest{
		TaskType: HeadlessTaskRepoConfigInfer,
	})
	if err != nil {
		t.Fatalf("RunHeadlessInference failed despite 500ms context deadline: %v", err)
	}
	if got.RunID != "airun_test" || got.Usage.TotalTokens != 15 {
		t.Fatalf("unexpected response: %+v", got)
	}

	_, err = RunHeadlessInference(context.Background(), client, ActorContext{UserID: "usr_no_money"}, HeadlessInferenceRequest{
		TaskType: HeadlessTaskRepoConfigInfer,
	})
	var upErr *UpstreamError
	if !errors.As(err, &upErr) || upErr.StatusCode != http.StatusUnprocessableEntity || upErr.Code != "ai.wallet_balance_insufficient" {
		t.Fatalf("expected 422 ai.wallet_balance_insufficient UpstreamError, got %v", err)
	}
}
