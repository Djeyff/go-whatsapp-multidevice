package middleware

import (
	"errors"
	"fmt"
	"io"
	"net/http/httptest"
	"strings"
	"testing"

	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
)

type levelRecorder struct {
	entries []logrus.Entry
}

func (r *levelRecorder) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (r *levelRecorder) Fire(entry *logrus.Entry) error {
	r.entries = append(r.entries, *entry)
	return nil
}

func (r *levelRecorder) hasLevel(t *testing.T, level logrus.Level, msgContains string) bool {
	t.Helper()
	for i := range r.entries {
		entry := &r.entries[i]
		if entry.Level == level && strings.Contains(entry.Message, msgContains) {
			return true
		}
	}
	return false
}

func newRecoveryTestApp(t *testing.T, rec *levelRecorder, panicWith any) *fiber.App {
	t.Helper()

	logrus.StandardLogger().AddHook(rec)
	t.Cleanup(func() {
		logrus.StandardLogger().ReplaceHooks(logrus.LevelHooks{})
	})

	app := fiber.New()
	app.Use(Recovery())

	app.Get("/boom", func(c *fiber.Ctx) error {
		if panicWith != nil {
			panic(panicWith)
		}
		return c.SendString("ok")
	})

	return app
}

func TestRecovery_RateLimitedErrorResponds429WithRetryAfter(t *testing.T) {
	rec := &levelRecorder{}
	app := newRecoveryTestApp(t, rec, pkgError.ErrWaRateLimited)

	req := httptest.NewRequest("GET", "/boom", nil)
	resp, err := app.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusTooManyRequests, resp.StatusCode)
	assert.Equal(t, "60", resp.Header.Get(fiber.HeaderRetryAfter))

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "WA_RATE_LIMITED")

	// Known rate-limit responses log at warn level, not error.
	assert.True(t, rec.hasLevel(t, logrus.WarnLevel, "rate limited"),
		"expected a warn-level log for the rate limit response")
	assert.False(t, rec.hasLevel(t, logrus.ErrorLevel, ""), "no error-level log expected for known WhatsApp errors")
}

func TestRecovery_ReachoutTimelockErrorResponds429(t *testing.T) {
	rec := &levelRecorder{}
	app := newRecoveryTestApp(t, rec, pkgError.ErrWaReachoutTimelock)

	req := httptest.NewRequest("GET", "/boom", nil)
	resp, err := app.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusTooManyRequests, resp.StatusCode)

	body, _ := io.ReadAll(resp.Body)
	assert.Contains(t, string(body), "WA_REACHOUT_TIMELOCK")

	assert.True(t, rec.hasLevel(t, logrus.WarnLevel, "463"),
		"expected warn-level log for reachout timelock")
	assert.False(t, rec.hasLevel(t, logrus.ErrorLevel, ""), "no error-level log expected for known WhatsApp errors")
}

func TestRecovery_UnknownGenericErrorKeepsErrorLevel(t *testing.T) {
	rec := &levelRecorder{}
	app := newRecoveryTestApp(t, rec, pkgError.ErrWaCLI)

	req := httptest.NewRequest("GET", "/boom", nil)
	resp, err := app.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	assert.False(t, rec.hasLevel(t, logrus.WarnLevel, ""), "warn level is reserved for known WhatsApp errors")
	assert.True(t, rec.hasLevel(t, logrus.ErrorLevel, ""), "unknown panics must stay at error level")
}

func TestRecovery_NonErrorPanicStillHandled(t *testing.T) {
	rec := &levelRecorder{}
	app := newRecoveryTestApp(t, rec, "plain string panic")

	req := httptest.NewRequest("GET", "/boom", nil)
	resp, err := app.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusInternalServerError, resp.StatusCode)

	assert.True(t, rec.hasLevel(t, logrus.ErrorLevel, ""), "string panics must stay at error level")
}

func TestRecovery_WrappedRateLimitedError(t *testing.T) {
	rec := &levelRecorder{}
	app := newRecoveryTestApp(t, rec, fmt.Errorf("send failed: %w", pkgError.ErrWaRateLimited))

	req := httptest.NewRequest("GET", "/boom", nil)
	resp, err := app.Test(req, -1)
	assert.NoError(t, err)
	assert.Equal(t, fiber.StatusTooManyRequests, resp.StatusCode,
		"errors.As unwraps to the typed rate-limit error")
	assert.Equal(t, "60", resp.Header.Get(fiber.HeaderRetryAfter))
	assert.True(t, rec.hasLevel(t, logrus.WarnLevel, ""), "wrapped known errors still log at warn via errors.As")
}

// Compile-time guard documenting that wrapped sentinels are matched by the
// middleware through errors.As, consistent with pkgError.IsWarnLevelError.
var _ = errors.As
