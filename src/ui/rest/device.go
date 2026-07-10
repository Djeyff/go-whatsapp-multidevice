package rest

import (
	"strings"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/domains/device"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/utils"
	"github.com/gofiber/fiber/v2"
)

type Device struct {
	Service device.IDeviceUsecase
}

func InitRestDevice(app fiber.Router, service device.IDeviceUsecase) Device {
	rest := Device{Service: service}

	app.Get("/devices", rest.ListDevices)
	app.Post("/devices", rest.AddDevice)

	app.Get("/devices/:device_id", rest.GetDevice)
	app.Delete("/devices/:device_id", rest.RemoveDevice)

	app.Get("/devices/:device_id/login", rest.LoginDevice)
	app.Post("/devices/:device_id/login/code", rest.LoginDeviceWithCode)
	app.Post("/devices/:device_id/logout", rest.LogoutDevice)
	app.Post("/devices/:device_id/reconnect", rest.ReconnectDevice)
	app.Post("/devices/:device_id/history-sync/full", rest.RequestFullHistorySync)
	app.Get("/devices/:device_id/status", rest.Status)

	return rest
}

func (handler *Device) ListDevices(c *fiber.Ctx) error {
	devices, err := handler.Service.ListDevices(c.UserContext())
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "List devices",
		Results: devices,
	})
}

func (handler *Device) GetDevice(c *fiber.Ctx) error {
	deviceID := c.Params("device_id")
	device, err := handler.Service.GetDevice(c.UserContext(), deviceID)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Device info",
		Results: device,
	})
}

func (handler *Device) AddDevice(c *fiber.Ctx) error {
	var req struct {
		DeviceID                  string `json:"device_id"`
		WebhookURL                string `json:"webhook_url"`
		WebhookSecret             string `json:"webhook_secret"`
		WebhookEvents             string `json:"webhook_events"`
		WebhookInsecureSkipVerify bool   `json:"webhook_insecure_skip_verify"`
	}

	if err := c.BodyParser(&req); err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  400,
			Code:    "BAD_REQUEST",
			Message: "Invalid request body",
			Results: nil,
		})
	}

	var webhook *chatstorage.DeviceWebhookConfig
	if strings.TrimSpace(req.WebhookURL) != "" ||
		strings.TrimSpace(req.WebhookSecret) != "" ||
		strings.TrimSpace(req.WebhookEvents) != "" ||
		req.WebhookInsecureSkipVerify {
		webhookURL := strings.TrimSpace(req.WebhookURL)
		webhook = &chatstorage.DeviceWebhookConfig{
			WebhookURL:                &webhookURL,
			WebhookSecret:             strings.TrimSpace(req.WebhookSecret),
			WebhookEvents:             strings.TrimSpace(req.WebhookEvents),
			WebhookInsecureSkipVerify: req.WebhookInsecureSkipVerify,
		}
	}

	device, err := handler.Service.AddDevice(c.UserContext(), req.DeviceID, webhook)
	utils.PanicIfNeeded(err)

	result := map[string]any{
		"id":           device.ID,
		"display_name": device.DisplayName,
		"jid":          device.JID,
		"state":        device.State,
		"created_at":   device.CreatedAt,
	}

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Device added",
		Results: result,
	})
}

func (handler *Device) RemoveDevice(c *fiber.Ctx) error {
	deviceID := c.Params("device_id")
	err := handler.Service.RemoveDevice(c.UserContext(), deviceID)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Device removed",
		Results: nil,
	})
}

func (handler *Device) LoginDevice(c *fiber.Ctx) error {
	deviceID := c.Params("device_id")
	err := handler.Service.LoginDevice(c.UserContext(), deviceID)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Login started",
		Results: map[string]any{"device_id": deviceID},
	})
}

func (handler *Device) LoginDeviceWithCode(c *fiber.Ctx) error {
	deviceID := c.Params("device_id")
	code, err := handler.Service.LoginDeviceWithCode(c.UserContext(), deviceID, c.Query("phone"))
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Login with code started",
		Results: map[string]any{
			"device_id": deviceID,
			"pair_code": code,
		},
	})
}

func (handler *Device) LogoutDevice(c *fiber.Ctx) error {
	deviceID := c.Params("device_id")
	err := handler.Service.LogoutDevice(c.UserContext(), deviceID)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Logout requested",
		Results: nil,
	})
}

func (handler *Device) ReconnectDevice(c *fiber.Ctx) error {
	deviceID := c.Params("device_id")
	err := handler.Service.ReconnectDevice(c.UserContext(), deviceID)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Reconnect requested",
		Results: nil,
	})
}

func (handler *Device) RequestFullHistorySync(c *fiber.Ctx) error {
	deviceID := strings.TrimSpace(c.Params("device_id"))
	if deviceID == "" {
		return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
			Status:  fiber.StatusBadRequest,
			Code:    "BAD_REQUEST",
			Message: "device_id is required",
			Results: nil,
		})
	}

	days := c.QueryInt("days", 0)
	if len(c.Body()) > 0 {
		var req struct {
			Days int `json:"days"`
		}
		if err := c.BodyParser(&req); err != nil {
			return c.Status(fiber.StatusBadRequest).JSON(utils.ResponseData{
				Status:  400,
				Code:    "BAD_REQUEST",
				Message: "Invalid request body",
				Results: nil,
			})
		}
		if req.Days > 0 {
			days = req.Days
		}
	}

	result, err := handler.Service.RequestFullHistorySync(c.UserContext(), deviceID, days)
	if err != nil {
		return fullHistorySyncErrorResponse(c, err)
	}

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Full history sync requested",
		Results: result,
	})
}

func fullHistorySyncErrorResponse(c *fiber.Ctx, err error) error {
	message := err.Error()
	status := fiber.StatusInternalServerError
	code := "FULL_HISTORY_SYNC_ERROR"

	switch {
	case strings.Contains(message, "device id is required"):
		status = fiber.StatusBadRequest
		code = "BAD_REQUEST"
	case strings.Contains(message, "not found"):
		status = fiber.StatusNotFound
		code = "DEVICE_NOT_FOUND"
	case strings.Contains(message, "not logged in"), strings.Contains(message, "not connected"), strings.Contains(message, "client not initialized"):
		status = fiber.StatusConflict
		code = "DEVICE_NOT_READY"
	case strings.Contains(message, "cooldown active"):
		status = fiber.StatusTooManyRequests
		code = "HISTORY_SYNC_COOLDOWN"
	case strings.Contains(message, "request full history sync"):
		status = fiber.StatusBadGateway
		code = "FULL_HISTORY_SYNC_SEND_FAILED"
	}

	return c.Status(status).JSON(utils.ResponseData{
		Status:  status,
		Code:    code,
		Message: message,
		Results: nil,
	})
}

func (handler *Device) Status(c *fiber.Ctx) error {
	deviceID := c.Params("device_id")
	isConnected, isLoggedIn, err := handler.Service.GetStatus(c.UserContext(), deviceID)
	utils.PanicIfNeeded(err)

	return c.JSON(utils.ResponseData{
		Status:  200,
		Code:    "SUCCESS",
		Message: "Device status",
		Results: map[string]any{
			"device_id":    deviceID,
			"is_connected": isConnected,
			"is_logged_in": isLoggedIn,
		},
	})
}
