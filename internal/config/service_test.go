package config

import (
	"context"
	"testing"
)

type mockProvider struct {
	pulled    Config
	published Config
	deleted   ConfigID
}

func (m *mockProvider) Pull(ctx context.Context, id ConfigID) (*Config, error) {
	return &m.pulled, nil
}

func (m *mockProvider) Publish(ctx context.Context, cfg Config) error {
	m.published = cfg
	return nil
}

func (m *mockProvider) Delete(ctx context.Context, id ConfigID) error {
	m.deleted = id
	return nil
}

func TestServicePull(t *testing.T) {
	provider := &mockProvider{
		pulled: Config{
			ID: ConfigID{
				DataID: "application.yaml",
				Group:  "DEFAULT_GROUP",
			},
			Content: "server.port: 8080",
		},
	}

	service := NewService(provider)

	cfg, err := service.Pull(context.Background(), provider.pulled.ID)
	if err != nil {
		t.Fatalf("unexcepted error: %v", err)
	}

	if cfg.Content != "server.port: 8080" {
		t.Fatalf("unexpected content: %s", cfg.Content)
	}
}

func TestServiceRejectsEmptyDataID(t *testing.T) {
	service := NewService(&mockProvider{})

	_, err := service.Pull(context.Background(), ConfigID{})
	if err != ErrEmptyDataID {
		t.Fatalf("expected ErrEmptyDataID, got: %v", err)
	}
}

func TestServiceRejectsNilProvider(t *testing.T) {
	service := NewService(nil)

	_, err := service.Pull(context.Background(), ConfigID{
		DataID: "app.yaml",
	})

	if err != ErrNilProvider {
		t.Fatalf("expected ErrNilProvider, got: %v", err)
	}
}
