package error

import (
	"errors"
	"net/http"
)

type InvalidJID string

// Error for complying the error interface
func (e InvalidJID) Error() string {
	return string(e)
}

// ErrCode will return the error code based on the error data type
func (e InvalidJID) ErrCode() string {
	return "INVALID_JID"
}

// StatusCode will return the HTTP status code based on the error data type
func (e InvalidJID) StatusCode() int {
	return http.StatusBadRequest
}

type WebhookError string

// Error for complying the error interface
func (e WebhookError) Error() string {
	return string(e)
}

// ErrCode will return the error code based on the error data type
func (e WebhookError) ErrCode() string {
	return "WEBHOOK_ERROR"
}

// StatusCode will return the HTTP status code based on the error data type
func (e WebhookError) StatusCode() int {
	return http.StatusInternalServerError
}

type WaCliError string

// Error for complying the error interface
func (e WaCliError) Error() string {
	return string(e)
}

// ErrCode will return the error code based on the error data type
func (e WaCliError) ErrCode() string {
	return "INVALID_WA_CLI"
}

// StatusCode will return the HTTP status code based on the error data type
func (e WaCliError) StatusCode() int {
	return http.StatusInternalServerError
}

type WaUploadMediaError string

// Error for complying the error interface
func (e WaUploadMediaError) Error() string {
	return string(e)
}

// ErrCode will return the error code based on the error data type
func (e WaUploadMediaError) ErrCode() string {
	return "UPLOAD_MEDIA_ERROR"
}

// StatusCode will return the HTTP status code based on the error data type
func (e WaUploadMediaError) StatusCode() int {
	return http.StatusInternalServerError
}

// WaReachoutTimelockError represents WhatsApp error 463 (reach-out timelock),
// the server-side anti-spam restriction on starting new chats.
type WaReachoutTimelockError string

// Error for complying the error interface
func (e WaReachoutTimelockError) Error() string {
	return string(e)
}

// ErrCode will return the error code based on the error data type
func (e WaReachoutTimelockError) ErrCode() string {
	return "WA_REACHOUT_TIMELOCK"
}

// StatusCode will return the HTTP status code based on the error data type
func (e WaReachoutTimelockError) StatusCode() int {
	return http.StatusTooManyRequests
}

// WaRateLimitedError represents WhatsApp server-side rate limiting (464
// NackCallerRateOverlimit / 429 rate-overlimit). WhatsApp enforces per-account
// request/send limits; the client cannot bypass them and callers must back off.
type WaRateLimitedError string

// Error for complying the error interface
func (e WaRateLimitedError) Error() string {
	return string(e)
}

// ErrCode will return the error code based on the error data type
func (e WaRateLimitedError) ErrCode() string {
	return "WA_RATE_LIMITED"
}

// StatusCode will return the HTTP status code based on the error data type
func (e WaRateLimitedError) StatusCode() int {
	return http.StatusTooManyRequests
}

const (
	ErrInvalidJID         = InvalidJID("your JID is invalid")
	ErrUserNotRegistered  = InvalidJID("user is not registered")
	ErrWaCLI              = WaCliError("your WhatsApp CLI is invalid or empty")
	ErrWaReachoutTimelock = WaReachoutTimelockError("WhatsApp rejected this send with error 463 (reach-out timelock). This is WhatsApp's server-side anti-spam restriction on starting new chats and cannot be bypassed by the API. It usually means there is no prior conversation with this recipient, or the sending account is temporarily restricted from reaching new contacts. To get through: ask the recipient to message you first, reply within an already-established conversation, or open and send the chat once from the phone linked to this session, then reduce your sending rate and wait for the restriction to clear. Newly-linked or low-activity numbers are affected most.")
	ErrWaRateLimited      = WaRateLimitedError("WhatsApp rejected this request because the account is rate limited (server error 464/429, rate-overlimit). This is WhatsApp's server-side throttle on how fast or how often the linked account may act; it is enforced by WhatsApp and cannot be bypassed by the API. To get through: slow down and space out requests (add a delay and retry the same action later, starting with a minute of backoff), reduce bulk/broadcast volume, and avoid hammering the same endpoint. If 464/429 keeps returning, pause automation for a longer period and review sending patterns for the account.")
)

// IsWarnLevelError reports whether err is a known, expected WhatsApp server
// condition (rate limiting, reach-out timelock) rather than an unexpected
// failure. Callers use this to log at warn level instead of error, keeping
// noisy operational alerts focused on real faults.
func IsWarnLevelError(err error) bool {
	var target GenericError
	if err == nil {
		return false
	}
	if errors.As(err, &target) {
		switch target.(type) {
		case WaRateLimitedError, WaReachoutTimelockError:
			return true
		}
	}
	return false
}
