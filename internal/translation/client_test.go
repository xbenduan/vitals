package translation

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"
	"unicode"
)

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

func testClient(t *testing.T, target string, handler http.HandlerFunc) *Client {
	t.Helper()
	server := httptest.NewServer(handler)
	t.Cleanup(server.Close)
	client, err := New(Config{User: "test-user", Token: "test-token", TargetLanguage: target})
	if err != nil {
		t.Fatal(err)
	}
	destination, _ := url.Parse(server.URL)
	transport := http.DefaultTransport.(*http.Transport).Clone()
	t.Cleanup(transport.CloseIdleConnections)
	client.http.Transport = roundTripFunc(func(r *http.Request) (*http.Response, error) {
		if r.URL.String() != endpoint || r.Method != http.MethodPost || r.Header.Get("Content-Type") != "application/json" {
			t.Error("request must POST JSON to the official endpoint")
		}
		request := r.Clone(r.Context())
		request.URL.Scheme, request.URL.Host = destination.Scheme, destination.Host
		return transport.RoundTrip(request)
	})
	return client
}

func TestTranslateDetectsLanguageAndAuthenticatesBothRequests(t *testing.T) {
	input := "### Notes\nUse `fetch(url)`.\n\n```ts\nconst value = 1;\n```"
	for _, tc := range []struct {
		name, source, target, wantTarget string
	}{
		{"EnglishToChinese", "en", TargetAuto, "zh"},
		{"ChineseToEnglish", "zh", TargetAuto, "en"},
		{"ChineseVariant", "zh-TW", TargetAuto, "en"},
		{"OtherLanguageToChinese", "ja", TargetAuto, "zh"},
		{"ExplicitEnglish", "ja", TargetEnglish, "en"},
		{"ExplicitChinese", "en", TargetChinese, "zh"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			client := testClient(t, tc.target, func(w http.ResponseWriter, r *http.Request) {
				var body struct {
					Header    requestHeader `json:"header"`
					Type      string        `json:"type"`
					Text      string        `json:"text"`
					Normalize struct {
						MergeBrokenLine *bool `json:"merge_broken_line"`
					} `json:"normalize"`
					Source struct {
						Lang     string   `json:"lang"`
						TextList []string `json:"text_list"`
					} `json:"source"`
					Target struct {
						Lang string `json:"lang"`
					} `json:"target"`
				}
				if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
					t.Error(err)
				}
				if body.Header.User != "test-user" || body.Header.Token != "test-token" || r.Header.Get("Authorization") != "" {
					t.Error("TranSmart requires user/token in the JSON header")
				}
				switch calls.Add(1) {
				case 1:
					if body.Header.Fn != "text_analysis" || body.Type != "plain" || body.Text != input ||
						body.Normalize.MergeBrokenLine == nil || *body.Normalize.MergeBrokenLine {
						t.Error("source detection must preserve the submitted text")
					}
					_ = json.NewEncoder(w).Encode(map[string]any{
						"header": map[string]string{"ret_code": "succ"}, "language": tc.source,
						"text": "normalized text must not replace the original",
					})
				case 2:
					if body.Header.Fn != "auto_translation" || body.Type != "markdown" || body.Source.Lang != tc.source ||
						body.Target.Lang != tc.wantTarget || !slices.Equal(body.Source.TextList, []string{input}) {
						t.Error("translation must use the explicit language pair and original Markdown")
					}
					_, _ = io.WriteString(w, `{"header":{"ret_code":"succ"},"auto_translation":["### 笔记\n使用 `+"`fetch(url)`"+`。\n\n"]}`)
				default:
					t.Error("unexpected extra request")
				}
			})
			result, err := client.Translate(context.Background(), input)
			if err != nil || result != "### 笔记\n使用 `fetch(url)`。\n\n" || calls.Load() != 2 {
				t.Fatalf("result=%q err=%v calls=%d", result, err, calls.Load())
			}
		})
	}
}

func TestTranslateAlreadyInTargetLanguage(t *testing.T) {
	var calls atomic.Int32
	client := testClient(t, TargetEnglish, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"header":{"ret_code":"succ"},"language":"en"}`)
	})
	text, err := client.Translate(context.Background(), "Keep this text.\n")
	if err != nil || text != "Keep this text.\n" || calls.Load() != 1 {
		t.Fatalf("same-language translation=%q err=%v calls=%d", text, err, calls.Load())
	}
}

func TestTranslateRejectsServiceErrors(t *testing.T) {
	for _, tc := range []struct {
		name, body, want string
		status           int
	}{
		{"Unauthorized", "", "认证失败", 401},
		{"Forbidden", "", "认证失败", 403},
		{"RateLimit", "", "过于频繁", 429},
		{"HTTPFailure", "", "HTTP 503", 503},
		{"AuthRetCode", `{"header":{"ret_code":"Auth-Failed"}}`, "认证失败", 200},
		{"SessionOut", `{"status":401,"error":"Session-Out"}`, "认证失败", 200},
		{"Quota", `{"status":429}`, "过于频繁", 200},
		{"ApplicationFailure", `{"header":{"ret_code":"fail"},"error":"test-token"}`, "未能完成", 200},
		{"ErrorWithSuccessHeader", `{"header":{"ret_code":"succ"},"error":{"message":"test-token"}}`, "未能完成", 200},
		{"MissingHeader", `{}`, "未能完成", 200},
		{"InvalidJSON", `<html>test-token</html>`, "格式无效", 200},
		{"MissingLanguage", `{"header":{"ret_code":"succ"}}`, "识别原文语言", 200},
		{"UnknownLanguage", `{"header":{"ret_code":"succ"},"language":"unknown"}`, "识别原文语言", 200},
		{"Oversized", strings.Repeat("x", maxResponseBytes+1), "超过 2 MB", 200},
	} {
		t.Run(tc.name, func(t *testing.T) {
			var calls atomic.Int32
			client := testClient(t, TargetAuto, func(w http.ResponseWriter, r *http.Request) {
				calls.Add(1)
				w.WriteHeader(tc.status)
				_, _ = io.WriteString(w, tc.body)
			})
			_, err := client.Translate(context.Background(), "test")
			if err == nil || !strings.Contains(err.Error(), tc.want) || strings.Contains(err.Error(), "test-token") || calls.Load() != 1 {
				t.Fatalf("error=%v calls=%d", err, calls.Load())
			}
		})
	}
}

func TestTranslateRequiresOneCompleteResult(t *testing.T) {
	for _, body := range []string{
		`{"header":{"ret_code":"succ"}}`,
		`{"header":{"ret_code":"succ"},"auto_translation":[]}`,
		`{"header":{"ret_code":"succ"},"auto_translation":["  "]}`,
		`{"header":{"ret_code":"succ"},"auto_translation":["one","two"]}`,
		`{"header":{"ret_code":"succ"},"auto_translation":[null]}`,
		`{"header":{"ret_code":"succ"},"auto_translation":"not an array"}`,
		`{"header":{"ret_code":"Auth-Failed"}}`,
	} {
		t.Run(body, func(t *testing.T) {
			var calls atomic.Int32
			client := testClient(t, TargetAuto, func(w http.ResponseWriter, r *http.Request) {
				if calls.Add(1) == 1 {
					_, _ = io.WriteString(w, `{"header":{"ret_code":"succ"},"language":"en"}`)
				} else {
					_, _ = io.WriteString(w, body)
				}
			})
			if _, err := client.Translate(context.Background(), "test"); err == nil {
				t.Fatal("invalid translation response must fail")
			}
		})
	}
}

func TestTranslateLimitsInputBeforeSending(t *testing.T) {
	var calls atomic.Int32
	client := testClient(t, TargetChinese, func(w http.ResponseWriter, r *http.Request) {
		calls.Add(1)
		_, _ = io.WriteString(w, `{"header":{"ret_code":"succ"},"language":"zh"}`)
	})
	for _, text := range []string{"", " \n\t", strings.Repeat("文", 5001), string([]byte{0xff})} {
		if _, err := client.Translate(context.Background(), text); err == nil {
			t.Fatal("invalid or oversized text must fail")
		}
	}
	if calls.Load() != 0 {
		t.Fatal("invalid input must not be sent")
	}
	text := strings.Repeat("文", 5000)
	if got, err := client.Translate(context.Background(), text); err != nil || got != text || calls.Load() != 1 {
		t.Fatalf("limit must count Unicode characters, err=%v", err)
	}
}

func TestTranslateCancellationReachesBothStages(t *testing.T) {
	for _, cancelStage := range []int32{1, 2} {
		t.Run(map[int32]string{1: "Detection", 2: "Translation"}[cancelStage], func(t *testing.T) {
			started, canceled := make(chan struct{}), make(chan struct{})
			var calls atomic.Int32
			client := testClient(t, TargetAuto, func(w http.ResponseWriter, r *http.Request) {
				// Read the body so HTTP/1 disconnect detection can observe cancellation.
				_, _ = io.Copy(io.Discard, r.Body)
				if calls.Add(1) < cancelStage {
					_, _ = io.WriteString(w, `{"header":{"ret_code":"succ"},"language":"en"}`)
					return
				}
				close(started)
				<-r.Context().Done()
				close(canceled)
			})
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
					t.Fatalf("cancellation error=%v", err)
				}
			case <-time.After(3 * time.Second):
				t.Fatal("request did not cancel")
			}
			select {
			case <-canceled:
			case <-time.After(3 * time.Second):
				t.Fatal("server did not observe cancellation")
			}
		})
	}
}

func TestTranslateDoesNotFollowCredentialRedirect(t *testing.T) {
	var reached atomic.Bool
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reached.Store(true)
	}))
	defer destination.Close()
	client := testClient(t, TargetAuto, func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, destination.URL, http.StatusTemporaryRedirect)
	})
	if _, err := client.Translate(context.Background(), "Hello"); err == nil || reached.Load() {
		t.Fatal("redirects must fail without replaying credentials")
	}
}

func TestNewRequiresTranSmartCredentials(t *testing.T) {
	for _, cfg := range []Config{
		Default(), {User: "user"}, {Token: "token"}, {User: " ", Token: "token"},
		{User: "user", Token: "token", TargetLanguage: "unsupported"},
	} {
		if _, err := New(cfg); err == nil {
			t.Fatal("invalid configuration must fail")
		}
	}
	client, err := New(Config{User: " user ", Token: " token "})
	if err != nil || client.config.User != "user" || client.config.Token != "token" || client.config.TargetLanguage != TargetAuto {
		t.Fatalf("credential normalization failed: %v", err)
	}
}

func TestTranSmartIntegration(t *testing.T) {
	user, token := os.Getenv("TRANSMART_USER"), os.Getenv("TRANSMART_TOKEN")
	if user == "" || token == "" {
		t.Skip("set TRANSMART_USER and TRANSMART_TOKEN to test the authenticated API")
	}
	client, err := New(Config{User: user, Token: token})
	if err != nil {
		t.Fatal(err)
	}
	for _, tc := range []struct {
		name, input string
		chinese     bool
	}{
		{"ChineseToEnglish", "请保存更改，然后刷新页面。", false},
		{"EnglishToChinese", "Save changes, then refresh the page.", true},
		{"ShortEnglish", "Save changes", true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			text, err := client.Translate(context.Background(), tc.input)
			if err != nil {
				t.Fatal(err)
			}
			han := 0
			for _, r := range text {
				if unicode.Is(unicode.Han, r) {
					han++
				}
			}
			if strings.TrimSpace(text) == "" || text == tc.input || (tc.chinese && han < 2) || (!tc.chinese && han > 0) {
				t.Fatalf("wrong translation direction: %q", text)
			}
		})
	}
}
