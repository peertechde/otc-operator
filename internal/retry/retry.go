package retry

import (
	"context"
	"fmt"
	"time"
)

var (
	ErrMaxRetriesReached = fmt.Errorf("exceeded max attempts")
)

type Option func(*Options)

func newDefaultOptions() *Options {
	return &Options{}
}

type Options struct {
	Delay       time.Duration
	MaxAttempts int
}

func WithDelay(d time.Duration) Option {
	return func(o *Options) {
		o.Delay = d
	}
}

func WithMaxAttempts(n int) Option {
	return func(o *Options) {
		o.MaxAttempts = n
	}
}

type Func func() (retry bool, err error)

func Do(ctx context.Context, fn Func, opts ...Option) error {
	options := newDefaultOptions()
	for _, opt := range opts {
		opt(options)
	}

	var err error
	var retry bool
	var n int

	if err := ctx.Err(); err != nil {
		return err
	}

	for {
		retry, err = fn()
		if !retry {
			break
		}

		n++
		if options.MaxAttempts != 0 && n > options.MaxAttempts {
			return ErrMaxRetriesReached
		}

		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(options.Delay):
		}
	}

	return err
}
