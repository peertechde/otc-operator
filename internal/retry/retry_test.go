package retry_test

import (
	"context"
	"testing"

	"github.com/peertech.de/otc-operator/internal/retry"
)

func TestRetry(t *testing.T) {
	var n int
	fn := func() (bool, error) {
		if n >= 5 {
			return false, nil
		}
		n++
		return true, nil
	}

	err := retry.Do(context.Background(), func() (bool, error) {
		return fn()
	}, retry.WithMaxAttempts(4))

	if err != retry.ErrMaxRetriesReached {
		t.Fatalf("Expected %s error, got %s error", retry.ErrMaxRetriesReached, err)
	}
}
