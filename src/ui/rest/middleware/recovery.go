package middleware

import (
	"context"
	"errors"
	"fmt"

	pkgError "github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/error"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/gofiber/fiber/v2"
	"github.com/sirupsen/logrus"
)

func Recovery() fiber.Handler {
	return func(ctx *fiber.Ctx) error {
		defer func() {
			err := recover()
			if err != nil {
				var res utils.ResponseData
				res.Status = 500
				res.Code = "INTERNAL_SERVER_ERROR"
				res.Message = fmt.Sprintf("%v", err)

				// Known WhatsApp server conditions (rate limiting, reach-out
				// timelock) are expected operational events: log them at warn
				// level instead of error, and expose Retry-After on 429s.
				if ctxErr, ok := err.(error); ok && pkgError.IsWarnLevelError(ctxErr) {
					logrus.Warnf("WhatsApp server rejected request: %v", ctxErr)
				} else {
					// Log the panic using logrus
					logrus.Errorf("Panic recovered in middleware: %v", err)
				}

				// Check for context deadline exceeded (timeout)
				if ctxErr, ok := err.(error); ok && ctxErr == context.DeadlineExceeded {
					res.Status = 504
					res.Code = "GATEWAY_TIMEOUT"
					res.Message = "Request timed out waiting for WhatsApp server response"
				}

				var errValidation pkgError.GenericError
				if ctxErr, ok := err.(error); ok && errors.As(ctxErr, &errValidation) {
					res.Status = errValidation.StatusCode()
					res.Code = errValidation.ErrCode()
					res.Message = errValidation.Error()
					if _, isRateLimited := errValidation.(pkgError.WaRateLimitedError); isRateLimited {
						ctx.Set(fiber.HeaderRetryAfter, "60")
					}
				}

				_ = ctx.Status(res.Status).JSON(res)
			}
		}()

		return ctx.Next()
	}
}
