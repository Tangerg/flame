package mcp

import (
	"context"
	"errors"
)

type BackgroundConfig struct {
	Prepare func(context.Context) error
	Run     func(context.Context)
	Release func() error
}

type Background struct {
	lifetime context.Context
	cancel   context.CancelFunc
	config   BackgroundConfig
	done     chan struct{}
}

func NewBackground(lifetime context.Context, config BackgroundConfig) (*Background, error) {
	if lifetime == nil || config.Run == nil || config.Release == nil {
		return nil, errors.New("plugin: background lifetime, loop and release are required")
	}
	ctx, cancel := context.WithCancel(lifetime)
	return &Background{lifetime: ctx, cancel: cancel, config: config}, nil
}

func (b *Background) Start() error {
	if b.config.Prepare != nil {
		if err := b.config.Prepare(b.lifetime); err != nil {
			return err
		}
	}
	b.done = make(chan struct{})
	go func() {
		defer close(b.done)
		b.config.Run(b.lifetime)
	}()
	return nil
}

func (b *Background) Close() error {
	b.cancel()
	if b.done != nil {
		<-b.done
	}
	return b.config.Release()
}
