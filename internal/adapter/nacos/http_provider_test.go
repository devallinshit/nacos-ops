package nacos

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/allinshit/nacos-ops/internal/config"
)

func TestHTTPProviderPull(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodGet {
				t.Errorf("expected GET, got %s", r.Method)
			}

			if r.URL.Path != configAPIPath {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			query := r.URL.Query()

			if got := query.Get("dataId"); got != "app.yaml" {
				t.Errorf("unexpected dataId: %s", got)
			}

			if got := query.Get("group"); got != "DEFAULT_GROUP" {
				t.Errorf("unexpected group: %s", got)
			}

			if got := query.Get("tenant"); got != "test-namespace" {
				t.Errorf("unexpected tenant: %s", got)
			}

			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "server.port: 8080")
		},
	))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL, nil)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	id := config.ConfigID{
		DataID:    "app.yaml",
		Group:     "DEFAULT_GROUP",
		Namespace: "test-namespace",
	}

	cfg, err := provider.Pull(t.Context(), id)
	if err != nil {
		t.Fatalf("pull config: %v", err)
	}

	if cfg == nil {
		t.Fatal("expected config, got nil")
	}

	if cfg.ID != id {
		t.Errorf("unexpected config ID: %+v", cfg.ID)
	}

	if cfg.Content != "server.port: 8080" {
		t.Errorf("unexpected content: %q", cfg.Content)
	}
}

func TestHTTPProviderPublish(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodPost {
				t.Errorf("expected POST, got %s", r.Method)
			}

			if r.URL.Path != configAPIPath {
				t.Errorf("unexpected path: %s", r.URL.Path)
			}

			if got := r.Header.Get("Content-Type"); !strings.HasPrefix(
				got, "application/x-www-form-urlencoded",
			) {
				t.Errorf("unexpected Content-Type: %s", got)
			}

			if err := r.ParseForm(); err != nil {
				t.Errorf("parse form: %v", err)
			}

			if got := r.Form.Get("dataId"); got != "app.yaml" {
				t.Errorf("unexpected dataId: %s", got)
			}

			if got := r.Form.Get("group"); got != "DEFAULT_GROUP" {
				t.Errorf("unexpected group: %s", got)
			}

			if got := r.Form.Get("tenant"); got != "test-namespace" {
				t.Errorf("unexpected tenant: %s", got)
			}

			expectedContent := "server.port: 8080\nname: test & demo"
			if got := r.Form.Get("content"); got != expectedContent {
				t.Errorf("unexpected content: %q", got)
			}

			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "true")
		},
	))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL, nil)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	cfg := config.Config{
		ID: config.ConfigID{
			DataID:    "app.yaml",
			Group:     "DEFAULT_GROUP",
			Namespace: "test-namespace",
		},
		Content: "server.port: 8080\nname: test & demo",
	}

	if err := provider.Publish(t.Context(), cfg); err != nil {
		t.Fatalf("publish config: %v", err)
	}
}

func TestHTTPProviderDelete(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			if r.Method != http.MethodDelete {
				t.Errorf("expected DELETE, got %s", r.Method)
			}

			query := r.URL.Query()

			if got := query.Get("dataId"); got != "app.yaml" {
				t.Errorf("unexpected dataId: %s", got)
			}

			if got := query.Get("group"); got != "DEFAULT_GROUP" {
				t.Errorf("unexpected group: %s", got)
			}

			// Namespace 为空时，不应该发送 tenant 参数。
			if _, exists := query["tenant"]; exists {
				t.Errorf("tenant should be omitted, got: %s", query.Get("tenant"))
			}

			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "true")
		},
	))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL, nil)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	id := config.ConfigID{
		DataID: "app.yaml",
		Group:  "DEFAULT_GROUP",
	}

	if err := provider.Delete(t.Context(), id); err != nil {
		t.Fatalf("delete config: %v", err)
	}
}

func TestHTTPProviderPullNotFound(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusNotFound)
			_, _ = io.WriteString(w, "config not found")
		},
	))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL, nil)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	_, err = provider.Pull(t.Context(), config.ConfigID{
		DataID: "missing.yaml",
		Group:  "DEFAULT_GROUP",
	})

	if !errors.Is(err, config.ErrConfigNotFound) {
		t.Fatalf("expected ErrConfigNotFound, got: %v", err)
	}
}

func TestHTTPProviderPublishRejected(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusOK)
			_, _ = io.WriteString(w, "false")
		},
	))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL, nil)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	err = provider.Publish(t.Context(), config.Config{
		ID: config.ConfigID{
			DataID: "app.yaml",
			Group:  "DEFAULT_GROUP",
		},
		Content: "server.port: 8080",
	})

	if err == nil {
		t.Fatal("expected publish to fail")
	}
}

func TestHTTPProviderServerError(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = io.WriteString(w, "internal server error")
		},
	))
	defer server.Close()

	provider, err := NewHTTPProvider(server.URL, nil)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	_, err = provider.Pull(t.Context(), config.ConfigID{
		DataID: "app.yaml",
		Group:  "DEFAULT_GROUP",
	})

	if err == nil {
		t.Fatal("expected server error")
	}

	if !strings.Contains(err.Error(), "status=500") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestHTTPProviderAuthentication(t *testing.T) {
	loginCount := 0
	configRequestCount := 0

	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			switch r.URL.Path {
			case "/nacos/v1/auth/login":
				loginCount++

				if r.Method != http.MethodPost {
					t.Errorf("expected POST login, got %s", r.Method)
				}

				if err := r.ParseForm(); err != nil {
					t.Errorf("parse login form: %v", err)
				}

				if got := r.Form.Get("username"); got != "test-user" {
					t.Errorf("unexpected username: %s", got)
				}

				if got := r.Form.Get("password"); got != "p@ss&word" {
					t.Errorf("unexpected password: %s", got)
				}

				w.Header().Set("Content-Type", "application/json")
				_, _ = io.WriteString(w, `{
                    "accessToken": "test-token",
                    "tokenTtl": 18000,
                    "globalAdmin": true
                }`)

			case configAPIPath:
				configRequestCount++

				if got := r.URL.Query().Get("accessToken"); got != "test-token" {
					t.Errorf("unexpected accessToken: %s", got)
				}

				_, _ = io.WriteString(w, "server.port: 8080")

			default:
				t.Errorf("unexpected request path: %s", r.URL.Path)
				http.NotFound(w, r)
			}
		},
	))
	defer server.Close()

	provider, err := NewHTTPProviderWithAuth(
		server.URL,
		"test-user",
		"p@ss&word",
		nil,
	)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	id := config.ConfigID{
		DataID: "app.yaml",
		Group:  "DEFAULT_GROUP",
	}

	// 第一次请求应该先登录，再拉取配置。
	if _, err := provider.Pull(context.Background(), id); err != nil {
		t.Fatalf("first pull: %v", err)
	}

	// 第二次请求应该复用缓存的 Token。
	if _, err := provider.Pull(context.Background(), id); err != nil {
		t.Fatalf("second pull: %v", err)
	}

	if loginCount != 1 {
		t.Errorf("expected 1 login request, got %d", loginCount)
	}

	if configRequestCount != 2 {
		t.Errorf("expected 2 config requests, got %d", configRequestCount)
	}
}

func TestHTTPProviderLoginFailed(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(
		func(w http.ResponseWriter, r *http.Request) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = io.WriteString(w, "unknown user")
		},
	))
	defer server.Close()

	provider, err := NewHTTPProviderWithAuth(
		server.URL,
		"wrong-user",
		"wrong-password",
		nil,
	)
	if err != nil {
		t.Fatalf("create provider: %v", err)
	}

	_, err = provider.Pull(context.Background(), config.ConfigID{
		DataID: "app.yaml",
		Group:  "DEFAULT_GROUP",
	})

	if err == nil {
		t.Fatal("expected login error")
	}

	if !strings.Contains(err.Error(), "nacos login failed") {
		t.Errorf("unexpected error: %v", err)
	}
}
