package whatsapp

import (
	"errors"
	"fmt"
	"testing"

	"go.mau.fi/whatsmeow"
)

// rateLimitServerErr mimics whatsmeow's message-send server rejection shape:
// fmt.Errorf("%w %d", ErrServerReturnedError, code) → "server returned error 464".
func rateLimitServerErr(code int) error {
	return fmt.Errorf("%w %d", whatsmeow.ErrServerReturnedError, code)
}

func TestIsRateLimitError(t *testing.T) {
	tests := []struct {
		name string
		err  error
		want bool
	}{
		{
			name: "server returned error 464 (NackCallerRateOverlimit)",
			err:  rateLimitServerErr(464),
			want: true,
		},
		{
			name: "IQ error 429 rate-overlimit",
			err:  &whatsmeow.IQError{Code: 429, Text: "rate-overlimit"},
			want: true,
		},
		{
			name: "IQ error with rate-overlimit text and zero code",
			err:  &whatsmeow.IQError{Text: "rate-overlimit"},
			want: true,
		},
		{
			name: "wrapped IQ error 429",
			err:  errors.Join(errors.New("send failed"), &whatsmeow.IQError{Code: 429, Text: "rate-overlimit"}),
			want: true,
		},
		{
			name: "server returned error 463 is reachout timelock, not rate limit",
			err:  errors.Join(whatsmeow.ErrServerReturnedError, errors.New("server returned error 463")),
			want: false,
		},
		{
			name: "server returned error 500",
			err:  errors.Join(whatsmeow.ErrServerReturnedError, errors.New("server returned error 500")),
			want: false,
		},
		{
			name: "server returned error 4631 does not match 463/464 suffix",
			err:  errors.Join(whatsmeow.ErrServerReturnedError, errors.New("server returned error 4631")),
			want: false,
		},
		{
			name: "IQ error 500",
			err:  &whatsmeow.IQError{Code: 500, Text: "internal-server-error"},
			want: false,
		},
		{
			name: "unrelated error",
			err:  errors.New("something else failed"),
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
			if got := IsRateLimitError(tt.err); got != tt.want {
				t.Fatalf("IsRateLimitError() = %v, want %v (err: %v)", got, tt.want, tt.err)
			}
		})
	}
}
