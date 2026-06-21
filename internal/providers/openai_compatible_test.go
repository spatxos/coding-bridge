package providers

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"sync/atomic"
	"testing"
	"time"
)

func TestOpenAICompatibleGenerateRetriesWithFreshRequestBody(t *testing.T) {
	var attempts atomic.Int32
	var firstBody string
	var secondBody string

	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		body, err := io.ReadAll(r.Body)
		if err != nil {
			t.Errorf("ReadAll() error = %v", err)
			w.WriteHeader(http.StatusInternalServerError)
			return
		}
		switch attempts.Add(1) {
		case 1:
			firstBody = string(body)
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte("temporary failure"))
		default:
			secondBody = string(body)
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(map[string]any{
				"model": "test-model",
				"choices": []map[string]any{{
					"message": map[string]any{
						"role":    "assistant",
						"content": "diff --git a/a b/a",
					},
					"finish_reason": "stop",
				}},
				"usage": map[string]any{
					"prompt_tokens":     1,
					"completion_tokens": 2,
					"total_tokens":      3,
				},
			})
		}
	}))
	defer server.Close()

	provider := NewOpenAICompatible(ProviderConfig{
		Type:     ProviderDeepSeek,
		BaseURL:  server.URL,
		Model:    "test-model",
		Timeout:  5 * time.Second,
		MaxRetry: 1,
	})
	response, err := provider.Generate(context.Background(), &GenerateRequest{
		Messages:  []ChatMessage{{Role: "user", Content: "make a patch"}},
		MaxTokens: 100,
	})
	if err != nil {
		t.Fatalf("Generate() error = %v", err)
	}
	if response.Content == "" {
		t.Fatal("Generate() returned empty content")
	}
	if attempts.Load() != 2 {
		t.Fatalf("attempts = %d, want 2", attempts.Load())
	}
	if firstBody == "" || secondBody == "" || firstBody != secondBody {
		t.Fatalf("request bodies differ or are empty: first=%q second=%q", firstBody, secondBody)
	}
}
