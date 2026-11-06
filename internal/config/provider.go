package config

import "context"

type ConfigProvider interface {
	Pull(ctx context.Context, id ConfigID) (*Config, error)
	Publish(ctx context.Context, cfg Config) error
	Delete(ctx context.Context, id ConfigID) error
}
