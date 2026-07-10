package config

import (
	"time"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
)

var (
	AppVersion                              = "v8.10.0"
	AppPort                                 = "3000"
	AppHost                                 = "0.0.0.0"
	AppDebug                                = false
	AppOs                                   = "Retena"
	AppPlatform                             = waCompanionReg.DeviceProps_PlatformType(1)
	AppBasicAuthCredential                  []string
	AppBasePath                             = ""
	AppTrustedProxies                       []string // Trusted proxy IP ranges (e.g., "0.0.0.0/0" for all, or specific CIDRs)
	HistorySyncWriteFiles                   = false
	RetenaPassiveListenerMode               = true
	RetenaPassivePresenceHeartbeat          = false
	RetenaPassivePresenceAvailableHeartbeat = false
	RetenaStaleDeviceCleanupGraceMinutes    = 30
	RetenaStaleDeviceCleanupMaxPerRun       = 25
	RetenaProtectedGowaDeviceIDs            []string

	McpPort = "8080"
	McpHost = "localhost"

	PathQrCode    = "statics/qrcode"
	PathSendItems = "statics/senditems"
	PathMedia     = "statics/media"
	PathStorages  = "storages"

	DBURI     = "file:storages/whatsapp.db?_foreign_keys=on&_journal_mode=WAL&_busy_timeout=5000"
	DBKeysURI = ""

	WhatsappAutoReplyMessage          string
	WhatsappAutoMarkRead              = false // Auto-mark incoming messages as read
	WhatsappAutoDownloadMedia         = false // Auto-download media from incoming messages
	WhatsappWebhook                   []string
	WhatsappWebhookSecret             = "secret"
	WhatsappWebhookInsecureSkipVerify = false          // Skip TLS certificate verification for webhooks (insecure)
	WhatsappWebhookEvents             []string         // Whitelist of events to forward to webhook (empty = all events)
	WhatsappAutoRejectCall                     = false // Auto-reject incoming calls
	WhatsappLogLevel                           = "ERROR"
	WhatsappSettingMaxImageSize       int64    = 20000000  // 20MB
	WhatsappSettingMaxFileSize        int64    = 50000000  // 50MB
	WhatsappSettingMaxVideoSize       int64    = 100000000 // 100MB
	WhatsappSettingMaxDownloadSize    int64    = 500000000 // 500MB
	WhatsappTypeUser                           = "@s.whatsapp.net"
	WhatsappTypeGroup                          = "@g.us"
	WhatsappTypeLid                            = "@lid"
	WhatsappTypeNewsletter                     = "@newsletter"
	WhatsappAccountValidation                  = true
	WhatsappPresenceOnConnect                  = "none" // Presence to send on connect: "available", "unavailable", or "none"
	WhatsappPresencePulseEnabled               = false  // Passive Retena default: do not periodically alter presence.
	WhatsappPresencePulseInterval              = 24 * time.Hour
	WhatsappPresencePulseDuration              = 5 * time.Minute

	ChatStorageURI               = "file:storages/chatstorage.db"
	ChatStorageEnableForeignKeys = true
	ChatStorageEnableWAL         = true
	ChatStorageMaxOpenConns      = 5 // Max concurrent SQLite connections for chat storage (WAL allows concurrent readers + 1 writer)

	ChatwootEnabled       = false
	ChatwootURL           = ""
	ChatwootAPIToken      = ""
	ChatwootAccountID     = 0
	ChatwootInboxID       = 0
	ChatwootDeviceID      = "" // Device ID for outbound messages (required for multi-device)
	ChatwootWebhookSecret = "" // Dedicated inbound webhook secret for Chatwoot -> GOWA sends

	// Chatwoot History Sync settings
	ChatwootImportMessages          = false // Enable message history import to Chatwoot
	ChatwootDaysLimitImportMessages = 3     // Days of history to import (default: 3)

	// ChatwootImportDBURI, when set, enables the direct-Postgres import path.
	// Historical sync will INSERT directly into Chatwoot's schema instead of
	// using the public REST API. Live forwarding and inbound handling always
	// use REST regardless of this flag.
	ChatwootImportDBURI = ""
	// ChatwootImportPlaceholderMediaMessage controls what is inserted for media
	// messages when the importer cannot download the media file.
	ChatwootImportPlaceholderMediaMessage = true
	// ChatwootImportMediaWithREST sends media history rows through Chatwoot's
	// REST attachment endpoint while direct-DB import handles non-media rows.
	ChatwootImportMediaWithREST = false

	ChatwootAutoCreate = false
	ChatwootInboxName  = "WhatsApp"
	ChatwootWebhookURL = ""

	ChatwootReopenConversation  = true
	ChatwootConversationPending = false
	ChatwootIgnoreJids          []string
	ChatwootSignMsg             = false
	ChatwootSignDelimiter       = "\n\n"
	ChatwootForwardEdits        = true
	ChatwootForwardDeletes      = true
	ChatwootMessageRead         = false
	ChatwootMessageDelete       = false
)
