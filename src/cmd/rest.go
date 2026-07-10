package cmd

import (
	"crypto/subtle"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/infrastructure/whatsapp"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/pkg/observability"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest/helpers"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/rest/middleware"
	"github.com/aldinokemal/go-whatsapp-web-multidevice/ui/websocket"
	"github.com/dustin/go-humanize"
	"github.com/gofiber/fiber/v2"
	"github.com/gofiber/fiber/v2/middleware/basicauth"
	"github.com/gofiber/fiber/v2/middleware/cors"
	"github.com/gofiber/fiber/v2/middleware/filesystem"
	"github.com/gofiber/fiber/v2/middleware/logger"
	"github.com/gofiber/template/html/v2"
	"github.com/sirupsen/logrus"
	"github.com/spf13/cobra"
)

func passivePresenceHeartbeatPath() string {
	base := strings.TrimRight(config.AppBasePath, "/")
	if base == "" {
		return "/send/presence"
	}
	return base + "/send/presence"
}

func passiveSafetyStatus() fiber.Map {
	commit := firstNonEmpty(os.Getenv("COMMIT_SHA"), os.Getenv("GIT_COMMIT"), "unknown")
	autoReplyEnabled := strings.TrimSpace(config.WhatsappAutoReplyMessage) != ""
	return fiber.Map{
		"ok":                           true,
		"service":                      "go-whatsapp-multidevice",
		"version":                      config.AppVersion,
		"commit":                       commit,
		"app_os":                       config.AppOs,
		"app_platform":                 config.AppPlatform.String(),
		"passive_mode":                 config.RetenaPassiveListenerMode,
		"passive_listener_mode":        config.RetenaPassiveListenerMode,
		"presence_heartbeat":           config.RetenaPassivePresenceHeartbeat,
		"passive_presence_heartbeat":   config.RetenaPassivePresenceHeartbeat,
		"presence_available_heartbeat": config.RetenaPassivePresenceAvailableHeartbeat,
		"passive_presence_available_heartbeat": config.RetenaPassivePresenceAvailableHeartbeat,
		"auto_reply_enabled":           autoReplyEnabled,
		"whatsapp_auto_reply_enabled":  autoReplyEnabled,
		"auto_mark_read":               config.WhatsappAutoMarkRead,
		"whatsapp_auto_mark_read":      config.WhatsappAutoMarkRead,
		"auto_download_media":          config.WhatsappAutoDownloadMedia,
		"whatsapp_auto_download_media": config.WhatsappAutoDownloadMedia,
		"auto_reject_call":             config.WhatsappAutoRejectCall,
		"whatsapp_auto_reject_call":    config.WhatsappAutoRejectCall,
		"presence_on_connect":          config.WhatsappPresenceOnConnect,
		"whatsapp_presence_on_connect": config.WhatsappPresenceOnConnect,
		"session_event_webhook_secret_value_exposed": false,
	}
}

func passivePresenceHeartbeatAllowed(c *fiber.Ctx) bool {
	if !config.RetenaPassivePresenceHeartbeat {
		return false
	}
	if c.Method() != fiber.MethodPost || strings.TrimRight(c.Path(), "/") != passivePresenceHeartbeatPath() {
		return false
	}
	var request struct {
		Type string `json:"type" form:"type"`
	}
	if err := json.Unmarshal(c.Body(), &request); err != nil {
		request.Type = c.FormValue("type")
	}
	switch strings.ToLower(strings.TrimSpace(request.Type)) {
	case "unavailable":
		return true
	case "available":
		return config.RetenaPassivePresenceAvailableHeartbeat
	default:
		return false
	}
}

// rootCmd represents the base command when called without any subcommands
var restCmd = &cobra.Command{
	Use:   "rest",
	Short: "Send whatsapp API over http",
	Long:  `This application is from clone https://github.com/aldinokemal/go-whatsapp-web-multidevice`,
	Run:   restServer,
}

func init() {
	rootCmd.AddCommand(restCmd)
}

func passiveListenerGuard(c *fiber.Ctx) error {
	if !config.RetenaPassiveListenerMode {
		return c.Next()
	}

	switch c.Method() {
	case fiber.MethodGet, fiber.MethodHead, fiber.MethodOptions:
		return c.Next()
	case fiber.MethodPost:
		if passivePresenceHeartbeatAllowed(c) {
			return c.Next()
		}
	default:
	}
	return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
		"code":    "PASSIVE_LISTENER_MODE",
		"message": "Retena passive listener mode blocks outbound and mutating WhatsApp routes",
		"path":    c.Path(),
	})
}

func chatwootWebhookSecretCandidates(c *fiber.Ctx) []string {
	candidates := []string{
		c.Get("X-Chatwoot-Webhook-Secret"),
		c.Get("X-Retena-Webhook-Secret"),
		c.Get("X-Api-Secret"),
	}

	auth := strings.TrimSpace(c.Get(fiber.HeaderAuthorization))
	if len(auth) > 7 && strings.EqualFold(auth[:7], "Bearer ") {
		candidates = append(candidates, strings.TrimSpace(auth[7:]))
	}
	return candidates
}

func chatwootWebhookAuthorized(c *fiber.Ctx) bool {
	secret := strings.TrimSpace(config.ChatwootWebhookSecret)
	if secret == "" {
		return false
	}
	for _, candidate := range chatwootWebhookSecretCandidates(c) {
		candidate = strings.TrimSpace(candidate)
		if candidate == "" || len(candidate) != len(secret) {
			continue
		}
		if subtle.ConstantTimeCompare([]byte(candidate), []byte(secret)) == 1 {
			return true
		}
	}
	return false
}

func chatwootWebhookGate(handler fiber.Handler) fiber.Handler {
	return func(c *fiber.Ctx) error {
		if strings.TrimSpace(config.ChatwootWebhookSecret) == "" {
			logrus.Warn("Chatwoot webhook rejected: CHATWOOT_WEBHOOK_SECRET is required when Chatwoot is enabled")
			return c.Status(fiber.StatusServiceUnavailable).JSON(fiber.Map{
				"code":    "CHATWOOT_WEBHOOK_SECRET_REQUIRED",
				"message": "Chatwoot webhook secret is required",
			})
		}
		if !chatwootWebhookAuthorized(c) {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"code":    "CHATWOOT_WEBHOOK_UNAUTHORIZED",
				"message": "Unauthorized Chatwoot webhook",
			})
		}
		return handler(c)
	}
}

func basicAuthRequiredEnvironment() bool {
	env := strings.ToLower(strings.TrimSpace(firstNonEmpty(os.Getenv("APP_ENV"), os.Getenv("NODE_ENV"), "production")))
	switch env {
	case "local", "dev", "development", "test", "testing":
		return false
	default:
		return true
	}
}

func basicAuthAccounts(credentials []string) (map[string]string, error) {
	account := make(map[string]string)
	for _, basicAuth := range credentials {
		basicAuth = strings.TrimSpace(basicAuth)
		if basicAuth == "" {
			continue
		}
		user, password, ok := strings.Cut(basicAuth, ":")
		if !ok || strings.TrimSpace(user) == "" || password == "" {
			return nil, fmt.Errorf("basic auth is not valid, use <user>:<secret>")
		}
		account[strings.TrimSpace(user)] = password
	}
	if len(account) == 0 && basicAuthRequiredEnvironment() {
		return nil, fmt.Errorf("APP_BASIC_AUTH is required outside local/dev/test environments")
	}
	return account, nil
}

func restServer(_ *cobra.Command, _ []string) {
	engine := html.NewFileSystem(http.FS(EmbedIndex), ".html")
	engine.AddFunc("isEnableBasicAuth", func(token any) bool {
		return token != nil
	})
	fiberConfig := fiber.Config{
		Views:                   engine,
		EnableTrustedProxyCheck: true,
		BodyLimit:               int(config.WhatsappSettingMaxVideoSize),
		Network:                 "tcp",
	}

	// Configure proxy settings if trusted proxies are specified
	if len(config.AppTrustedProxies) > 0 {
		fiberConfig.TrustedProxies = config.AppTrustedProxies
		fiberConfig.ProxyHeader = fiber.HeaderXForwardedHost
	}

	app := fiber.New(fiberConfig)

	app.Static(config.AppBasePath+"/statics", "./statics")
	app.Use(config.AppBasePath+"/components", filesystem.New(filesystem.Config{
		Root:       http.FS(EmbedViews),
		PathPrefix: "views/components",
		Browse:     true,
	}))
	app.Use(config.AppBasePath+"/assets", filesystem.New(filesystem.Config{
		Root:       http.FS(EmbedViews),
		PathPrefix: "views/assets",
		Browse:     true,
	}))

	app.Use(func(c *fiber.Ctx) error {
		correlationID := c.Get("X-Correlation-ID")
		if correlationID == "" {
			correlationID = fmt.Sprintf("gowa-%d", time.Now().UnixNano())
		}
		c.Set("X-Correlation-ID", correlationID)
		c.Locals("correlation_id", correlationID)
		return c.Next()
	})
	app.Use(middleware.Recovery())
	app.Use(middleware.RequestTimeout(middleware.DefaultRequestTimeout))
	app.Use(middleware.BasicAuth())
	if config.AppDebug {
		app.Use(logger.New())
	}
	app.Use(cors.New(cors.Config{
		AllowOrigins: "*",
		AllowHeaders: "Origin, Content-Type, Accept",
	}))

	// Device manager - needed for chatwoot webhook and health check
	dm := whatsapp.GetDeviceManager()

	// Health check endpoint (public, no auth)
	// Registered at root path (ignoring AppBasePath) to ensure fixed availability
	// for infrastructure health probes (Kubernetes liveness/readiness, Docker healthcheck, etc.)
	app.Get("/health", func(c *fiber.Ctx) error {
		if dm != nil && dm.IsHealthy() {
			return c.JSON(fiber.Map{
				"ok":      true,
				"service": "go-whatsapp-multidevice",
				"version": config.AppVersion,
				"commit":  firstNonEmpty(os.Getenv("COMMIT_SHA"), os.Getenv("GIT_COMMIT"), "unknown"),
			})
		}
		return c.Status(http.StatusServiceUnavailable).JSON(fiber.Map{
			"ok":      false,
			"service": "go-whatsapp-multidevice",
			"error":   "Service Unavailable",
		})
	})

	// Chatwoot webhook - registered before optional app basic auth, but guarded
	// by a dedicated shared secret so it cannot be used as a public send proxy.
	if config.ChatwootEnabled {
		chatwootHandler := rest.NewChatwootHandler(appUsecase, sendUsecase, messageUsecase, dm, chatStorageRepo)
		webhookPath := "/chatwoot/webhook"
		if config.AppBasePath != "" {
			webhookPath = config.AppBasePath + webhookPath
		}
		app.Post(webhookPath, chatwootWebhookGate(chatwootHandler.HandleWebhook))
	}

	account, err := basicAuthAccounts(config.AppBasicAuthCredential)
	if err != nil {
		logrus.Fatalln(err)
	}
	if len(account) > 0 {
		app.Use(basicauth.New(basicauth.Config{
			Users: account,
		}))
	}

	app.Get("/health/passive-safety", func(c *fiber.Ctx) error {
		return c.JSON(passiveSafetyStatus())
	})

	app.Get("/health/observability", func(c *fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"ok":            true,
			"service":       "go-whatsapp-multidevice",
			"version":       config.AppVersion,
			"commit":        firstNonEmpty(os.Getenv("COMMIT_SHA"), os.Getenv("GIT_COMMIT"), "unknown"),
			"observability": observability.Status(),
			"smoke": fiber.Map{
				"sentryCaptureRoute": "/debug/sentry",
				"healthRoute":        "/health",
				"observabilityRoute": "/health/observability",
			},
		})
	})

	app.Post("/debug/sentry", func(c *fiber.Ctx) error {
		message, ok := observability.CaptureSyntheticSentry("go-whatsapp-multidevice", map[string]string{
			"smoke":          "true",
			"service":        "go-whatsapp-multidevice",
			"correlation_id": fmt.Sprint(c.Locals("correlation_id")),
		}, map[string]any{
			"route":          "/debug/sentry",
			"version":        config.AppVersion,
			"commit":         firstNonEmpty(os.Getenv("COMMIT_SHA"), os.Getenv("GIT_COMMIT"), "unknown"),
			"correlation_id": fmt.Sprint(c.Locals("correlation_id")),
		})
		if !ok {
			return c.Status(http.StatusPreconditionFailed).JSON(fiber.Map{"ok": false, "error": "Sentry not configured"})
		}
		sentry.Flush(2 * time.Second)
		return c.JSON(fiber.Map{
			"ok":             true,
			"sentryCaptured": true,
			"message":        message,
			"version":        config.AppVersion,
			"commit":         firstNonEmpty(os.Getenv("COMMIT_SHA"), os.Getenv("GIT_COMMIT"), "unknown"),
			"correlationId":  fmt.Sprint(c.Locals("correlation_id")),
		})
	})

	// Create base path group or use app directly
	var apiGroup fiber.Router = app
	if config.AppBasePath != "" {
		apiGroup = app.Group(config.AppBasePath)
	}

	registerDeviceScopedRoutes := func(r fiber.Router) {
		rest.InitRestApp(r, appUsecase)
		rest.InitRestChat(r, chatUsecase)
		rest.InitRestSend(r, sendUsecase)
		rest.InitRestUser(r, userUsecase)
		rest.InitRestMessage(r, messageUsecase)
		rest.InitRestGroup(r, groupUsecase)
		rest.InitRestNewsletter(r, newsletterUsecase)
		websocket.RegisterRoutes(r, appUsecase)
		// Retena media proxy — streams audio bytes without disk write
		r.Get("/media/stream/:message_id", rest.StreamMedia)
	}

	// Device management routes (no device_id required)
	rest.InitRestDevice(apiGroup, deviceUsecase)

	// Device-scoped operations (header-based)
	headerDeviceGroup := apiGroup.Group("", middleware.DeviceMiddleware(dm), passiveListenerGuard)
	registerDeviceScopedRoutes(headerDeviceGroup)

	// Chatwoot sync routes - require authentication (webhook is registered earlier without auth)
	if config.ChatwootEnabled {
		chatwootHandler := rest.NewChatwootHandler(appUsecase, sendUsecase, messageUsecase, dm, chatStorageRepo)
		apiGroup.Post("/chatwoot/sync", chatwootHandler.SyncHistory)
		apiGroup.Get("/chatwoot/sync/status", chatwootHandler.SyncStatus)
	}

	apiGroup.Get("/", func(c *fiber.Ctx) error {
		return c.Render("views/index", fiber.Map{
			"AppHost":        fmt.Sprintf("%s://%s", c.Protocol(), c.Hostname()),
			"AppVersion":     config.AppVersion,
			"AppBasePath":    config.AppBasePath,
			"BasicAuthToken": c.UserContext().Value(middleware.AuthorizationValue("BASIC_AUTH")),
			"MaxFileSize":    humanize.Bytes(uint64(config.WhatsappSettingMaxFileSize)),
			"MaxVideoSize":   humanize.Bytes(uint64(config.WhatsappSettingMaxVideoSize)),
		})
	})

	go websocket.RunHub()

	// Set auto reconnect to whatsapp server after booting
	go helpers.SetAutoConnectAfterBooting(appUsecase)

	// Set auto reconnect checking with a guaranteed client instance
	startAutoReconnectCheckerIfClientAvailable()

	// Version banner
	{
		commitSHA := os.Getenv("COMMIT_SHA")
		if len(commitSHA) > 8 {
			commitSHA = commitSHA[:8]
		}
		if commitSHA == "" {
			commitSHA = "dev"
		}
		pipelineID := os.Getenv("PIPELINE_ID")
		if len(pipelineID) > 8 {
			pipelineID = pipelineID[:8]
		}
		if pipelineID == "" {
			pipelineID = "dev"
		}
		logrus.Infof("[version] 🟣 commit=%s pipeline=%s go=%s", commitSHA, pipelineID, runtime.Version())
	}

	if err := app.Listen(config.AppHost + ":" + config.AppPort); err != nil {
		logrus.Fatalln("Failed to start: ", err.Error())
	}
}
