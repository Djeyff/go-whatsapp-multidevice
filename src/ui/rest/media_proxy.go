package rest

import (
	"fmt"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/gofiber/fiber/v2"
	"go.mau.fi/whatsmeow/proto/waE2E"
	"google.golang.org/protobuf/proto"
)

// StreamMedia streams raw audio bytes for a message directly to the caller — no disk write.
// GET /media/stream/:message_id
// Requires Basic Auth (same as all other endpoints).
// Used by retena-whatsapp processor to fetch voice notes without touching GOWA disk.
func StreamMedia(c *fiber.Ctx) error {
	messageID := c.Params("message_id")
	if messageID == "" {
		return c.Status(400).JSON(fiber.Map{"code": "BAD_REQUEST", "message": "message_id required"})
	}

	device, ok := c.Locals("device").(*whatsapp.DeviceInstance)
	if !ok || device == nil {
		return c.Status(503).JSON(fiber.Map{"code": "NO_DEVICE", "message": "no device available"})
	}

	client := device.GetClient()
	if client == nil {
		return c.Status(503).JSON(fiber.Map{"code": "NO_CLIENT", "message": "WhatsApp client not connected"})
	}

	chatStorage := device.GetChatStorage()
	if chatStorage == nil {
		return c.Status(503).JSON(fiber.Map{"code": "NO_STORAGE", "message": "chat storage not available"})
	}

	msg, err := chatStorage.GetMessageByID(messageID)
	if err != nil || msg == nil {
		return c.Status(404).JSON(fiber.Map{"code": "NOT_FOUND", "message": fmt.Sprintf("message %s not found", messageID)})
	}

	if msg.URL == "" || len(msg.MediaKey) == 0 {
		return c.Status(400).JSON(fiber.Map{"code": "NO_MEDIA", "message": "message has no downloadable media"})
	}

	// Build AudioMessage for whatsmeow Download (works for voice notes / ptt)
	downloadable := &waE2E.AudioMessage{
		URL:           proto.String(msg.URL),
		MediaKey:      msg.MediaKey,
		FileEncSHA256: msg.FileEncSHA256,
		FileSHA256:    msg.FileSHA256,
		FileLength:    proto.Uint64(msg.FileLength),
		Mimetype:      proto.String("audio/ogg; codecs=opus"),
	}

	// Download in memory — no disk write
	data, err := client.Download(c.UserContext(), downloadable)
	if err != nil {
		return c.Status(500).JSON(fiber.Map{"code": "DOWNLOAD_ERROR", "message": err.Error()})
	}

	c.Set("Content-Type", "audio/ogg; codecs=opus")
	c.Set("Content-Disposition", fmt.Sprintf(`inline; filename="%s.ogg"`, messageID))
	return c.Send(data)
}
