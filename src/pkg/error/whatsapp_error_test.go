package error

import (
	"errors"
	"fmt"
	"net/http"
	"testing"
)

func TestWaRateLimitedErrorContract(t *testing.T) {
	if got := ErrWaRateLimited.ErrCode(); got != "WA_RATE_LIMITED" {
		t.Fatalf("expected code WA_RATE_LIMITED, got %q", got)
	}
	if got := ErrWaRateLimited.StatusCode(); got != http.StatusTooManyRequests {
		t.Fatalf("expected status %d, got %d", http.StatusTooManyRequests, got)
	}
	if ErrWaRateLimited.Error() == "" {
		t.Fatal("expected non-empty error message")
	}
}

func TestIsWarnLevelError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "rate limited sentinel",
			err:  ErrWaRateLimited,
			want: true,
		},
		{
			name: "reachout timelock sentinel",
			err:  ErrWaReachoutTimelock,
			want: true,
		},
		{
			name: "wrapped rate limited sentinel",
			err:  fmt.Errorf("send failed: %w", ErrWaRateLimited),
			want: true,
		},
		{
			name: "wrapped reachout timelock sentinel",
			err:  fmt.Errorf("send failed: %w", ErrWaReachoutTimelock),
			want: true,
		},
		{
			name: "other generic error is not warn level",
			err:  ErrInvalidJID,
			want: false,
		},
		{
			name: "plain error",
			err:  errors.New("something failed"),
			want: false,
		},
		{
			name: "nil error",
			err:  nil,
			want: false,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsWarnLevelError(tt.err); got != tt.want {
				t.Fatalf("IsWarnLevelError(%v) = %v, want %v", tt.err, got, tt.want)
			}
		})
	}
}
