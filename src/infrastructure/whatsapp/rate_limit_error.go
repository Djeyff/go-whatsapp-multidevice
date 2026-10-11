package whatsapp

import (
	"errors"
	"strings"

	"go.mau.fi/whatsmeow"
)

// IsRateLimitError reports whether err is WhatsApp's server-side rate limiting.
//
// Two wire shapes are covered:
//   - Message-send path (whatsmeow formats it as fmt.Errorf("%w %d",
//     ErrServerReturnedError, code) → "server returned error 464"). 464 is
//     NackCallerRateOverlimit, the sibling of the 463 reach-out timelock.
//   - Info-query path (whatsmeow's IQError with code 429 / text
//     "rate-overlimit", or any IQError whose text is "rate-overlimit").
//
// Like 463, the limit is enforced by WhatsApp and cannot be bypassed by the
// client — the caller must back off and retry later.
func IsRateLimitError(err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, whatsmeow.ErrServerReturnedError) &&
		strings.HasSuffix(err.Error(), " 464") {
		return true
	}
	var iqErr *whatsmeow.IQError
	if errors.As(err, &iqErr) {
		return iqErr.Code == 429 || iqErr.Text == "rate-overlimit"
	}
	return false
}
