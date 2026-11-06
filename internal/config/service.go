package config

import (
	"context"
	"errors"
)

var (
	ErrNilProvider    = errors.New("config provider is nil")
	ErrEmptyDataID    = errors.New("config data ID is required")
	ErrConfigNotFound = errors.New("config not found")
)

type ConfigService struct {
	provider ConfigProvider
}

func NewService(provider ConfigProvider) *ConfigService {
	return &ConfigService{
		provider: provider,
	}
}

func (s *ConfigService) validate(id ConfigID) error {
	if s == nil || s.provider == nil {
		return ErrNilProvider
	}

	if id.DataID == "" {
		return ErrEmptyDataID
	}

	return nil
}

func (s *ConfigService) Pull(ctx context.Context, id ConfigID) (*Config, error) {
	if err := s.validate(id); err != nil {
		return nil, err
	}

	return s.provider.Pull(ctx, id)
}

func (s *ConfigService) Publish(ctx context.Context, cfg Config) error {
	if err := s.validate(cfg.ID); err != nil {
		return err
	}

	return s.provider.Publish(ctx, cfg)
}

func (s *ConfigService) Delete(ctx context.Context, id ConfigID) error {
	if err := s.validate(id); err != nil {
		return err
	}

	return s.provider.Delete(ctx, id)
}
