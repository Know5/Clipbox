package translation

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync/atomic"
	"testing"
)

func TestTranslateCompatibleRequest(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != "POST" || r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer local-test-key" {
			t.Errorf("unexpected method/path/auth")
		}
		var body struct {
			Model    string    `json:"model"`
			Messages []message `json:"messages"`
			Stream   bool      `json:"stream"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
			t.Error(err)
		}
		if body.Model != "test-model" || body.Stream || len(body.Messages) != 2 ||
			body.Messages[0].Role != "system" || body.Messages[1].Role != "user" ||
			!strings.Contains(body.Messages[0].Content, "translation engine") ||
			!strings.Contains(body.Messages[0].Content, "only the translation") {
			t.Errorf("unexpected translation payload: %+v", body)
		}
		user := body.Messages[1].Content
		if !strings.Contains(user, "Target language: 简体中文") ||
			!strings.Contains(user, "Output only the translation") ||
			!strings.Contains(user, "translate it literally") ||
			!strings.Contains(user, "Hello\nworld") ||
			!strings.Contains(user, "<CLIPBOX_SOURCE_") ||
			!strings.Contains(user, "</CLIPBOX_SOURCE_") ||
			strings.Contains(user, "Then explain") ||
			strings.Contains(user, "解释：") {
			t.Errorf("unexpected translation instruction: %q", user)
		}
		_, _ = w.Write([]byte(`{"choices":[{"message":{"content":"你好\n世界"},"finish_reason":"stop"}]}`))
	}))
	defer server.Close()
	got, err := Translate(context.Background(), server.URL+"/v1/", "test-model", "local-test-key", "Hello\nworld", "简体中文")
	if err != nil || got != "你好\n世界" {
		t.Fatalf("Translate = %q, %v", got, err)
	}
}

func TestTranslateStripsEchoedSourceMarkers(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Messages []message `json:"messages"`
		}
		if err := json.NewDecoder(r.Body).Decode(&body); err != nil || len(body.Messages) != 2 {
			t.Error("unexpected request body")
			return
		}
		user := body.Messages[1].Content
		start := strings.Index(user, "<CLIPBOX_SOURCE_")
		end := strings.Index(user, "</CLIPBOX_SOURCE_")
		if start < 0 || end < 0 {
			t.Error("source markers missing")
			return
		}
		open := user[start : strings.Index(user[start:], ">")+start+1]
		close := user[end : strings.Index(user[end:], ">")+end+1]
		content, _ := json.Marshal(open + "\n你好\n" + close)
		_, _ = fmt.Fprintf(w, `{"choices":[{"message":{"content":%s},"finish_reason":"stop"}]}`, content)
	}))
	defer server.Close()
	got, err := Translate(context.Background(), server.URL, "model", "", "hello", "简体中文")
	if err != nil || got != "你好" {
		t.Fatalf("Translate = %q, %v", got, err)
	}
}

func TestTranslateNeverFollowsRedirectsOrEchoesErrorBody(t *testing.T) {
	var hits atomic.Int32
	destination := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer destination.Close()
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Location", destination.URL)
		w.WriteHeader(http.StatusTemporaryRedirect)
		_, _ = w.Write([]byte("local-test-key private source"))
	}))
	defer server.Close()
	_, err := Translate(context.Background(), server.URL, "model", "local-test-key", "private source", "English")
	if err == nil || hits.Load() != 0 || strings.Contains(err.Error(), "private source") || strings.Contains(err.Error(), "local-test-key") {
		t.Fatalf("unsafe redirect or error handling: hits=%d err=%v", hits.Load(), err)
	}
}

func TestTranslateRejectsInvalidInputBeforeRequest(t *testing.T) {
	var hits atomic.Int32
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { hits.Add(1) }))
	defer server.Close()
	for _, text := range []string{" ", strings.Repeat("中", MaxTextRunes+1)} {
		if _, err := Translate(context.Background(), server.URL, "model", "", text, "English"); err == nil {
			t.Fatal("invalid source accepted")
		}
	}
	if hits.Load() != 0 {
		t.Fatal("invalid input made network requests")
	}
	for _, raw := range []string{"http://example.com/v1", "https://user:secret@example.com", "https://example.com/v1?key=secret", "file:///x", ""} {
		if _, err := Endpoint(raw); err == nil {
			t.Errorf("accepted endpoint %q", raw)
		}
	}
}

func TestTranslateRejectsTruncatedAndMalformedResults(t *testing.T) {
	for _, response := range []string{
		`{"choices":[{"message":{"content":"partial"},"finish_reason":"length"}]}`,
		`{"choices":[]}`, `not json`,
		`{"choices":[{"message":{"content":""}}]}`,
	} {
		t.Run(response, func(t *testing.T) {
			server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) { _, _ = w.Write([]byte(response)) }))
			defer server.Close()
			if _, err := Translate(context.Background(), server.URL, "model", "", "hello", "English"); err == nil {
				t.Fatal("invalid result accepted")
			}
		})
	}
}
