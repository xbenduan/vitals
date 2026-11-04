package translation

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"testing"
	"time"
)

func TestTranslateSeparatesPromptAndText(t *testing.T) {
	for _, provider := range []string{Ollama, OpenAI} {
		t.Run(provider, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Model    string    `json:"model"`
					Messages []message `json:"messages"`
					Stream   bool      `json:"stream"`
					Think    *bool     `json:"think"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if r.Method != http.MethodPost || body.Model != "test-model" || body.Stream ||
					!slices.Equal(body.Messages, []message{{"system", "custom prompt"}, {"user", "const value = 1;\n请翻译"}}) {
					t.Errorf("unexpected request: %s %+v", r.Method, body)
				}
				if r.Header.Get("Authorization") != "Bearer test-key" {
					t.Error("API key was not sent")
				}
				if provider == Ollama {
					if r.URL.Path != "/api/chat" || body.Think == nil || *body.Think {
						t.Error("Ollama request must use chat without thinking")
					}
					_, _ = w.Write([]byte(`{"message":{"content":"const value = 1;\nPlease translate"},"done_reason":"stop"}`))
				} else {
					if r.URL.Path != "/v1/chat/completions" || body.Think != nil {
						t.Error("OpenAI request must respect API prefix")
					}
					_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"const value = 1;\nPlease translate"},"finish_reason":"stop"}]}`))
				}
			}))
			defer server.Close()
			base := server.URL
			if provider == OpenAI {
				base += "/v1/"
			}
			client, err := New(Config{Provider: provider, BaseURL: base, Model: "test-model", APIKey: "test-key", Prompt: "custom prompt"})
			if err != nil {
				t.Fatal(err)
			}
			result, err := client.Translate(context.Background(), "const value = 1;\n请翻译")
			if err != nil || result != "const value = 1;\nPlease translate" {
				t.Fatalf("result=%q err=%v", result, err)
			}
		})
	}
}

func TestOllamaModelDiscoverySkipsEmbeddingModels(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/api/tags":
			_, _ = w.Write([]byte(`{"models":[{"name":"large","size":9000},{"name":"embedding","size":100},{"name":"small","size":500}]}`))
		case "/api/show":
			var body map[string]string
			_ = json.NewDecoder(r.Body).Decode(&body)
			capability := "completion"
			if body["model"] == "embedding" {
				capability = "embedding"
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"capabilities": []string{capability}})
		default:
			t.Errorf("unexpected path %s", r.URL.Path)
		}
	}))
	defer server.Close()
	cfg := Default()
	cfg.BaseURL = server.URL
	client, _ := New(cfg)
	models, err := client.Models(context.Background())
	if err != nil || !slices.Equal(models, []string{"small", "large"}) {
		t.Fatalf("models=%v err=%v", models, err)
	}
}

func TestTranslateRejectsInvalidResponses(t *testing.T) {
	for _, body := range []string{
		`{"message":{"content":""}}`,
		`{"message":{"content":"partial"},"done_reason":"length"}`,
		`{"error":"model missing"}`,
		`<html>wrong endpoint</html>`,
	} {
		t.Run(body, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				_, _ = w.Write([]byte(body))
			}))
			defer server.Close()
			cfg := Default()
			cfg.BaseURL, cfg.Model = server.URL, "test"
			client, _ := New(cfg)
			if _, err := client.Translate(context.Background(), "text"); err == nil {
				t.Fatal("invalid response must fail")
			}
		})
	}
}

func TestTranslateCancellationReachesServer(t *testing.T) {
	started, canceled := make(chan struct{}), make(chan struct{})
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Read the request so HTTP/1 disconnect detection can observe cancellation.
		var body any
		_ = json.NewDecoder(r.Body).Decode(&body)
		close(started)
		<-r.Context().Done()
		close(canceled)
	}))
	defer server.Close()
	cfg := Default()
	cfg.BaseURL, cfg.Model = server.URL, "test"
	client, _ := New(cfg)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := client.Translate(ctx, "cancel me")
		result <- err
	}()
	select {
	case <-started:
	case <-time.After(3 * time.Second):
		t.Fatal("request did not start")
	}
	cancel()
	select {
	case err := <-result:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("cancellation error = %v", err)
		}
	case <-time.After(3 * time.Second):
		t.Fatal("request did not cancel")
	}
	select {
	case <-canceled:
	case <-time.After(3 * time.Second):
		t.Fatal("server did not observe cancellation")
	}
}

func TestLocalOllamaIntegration(t *testing.T) {
	model := os.Getenv("VITALS_TEST_OLLAMA_MODEL")
	if model == "" {
		t.Skip("set VITALS_TEST_OLLAMA_MODEL to verify an installed local model")
	}
	cfg := Default()
	cfg.Model = model
	client, err := New(cfg)
	if err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	models, err := client.Models(ctx)
	if err != nil || !slices.Contains(models, model) {
		t.Fatalf("configured model unavailable: %v", err)
	}
	text, err := client.Translate(ctx, "请使用 AbortController 取消请求，避免组件卸载后更新状态。")
	if err != nil || text == "" {
		t.Fatalf("translation failed: %v", err)
	}
	t.Logf("local translation: %s", text)
}
