package cmd

import (
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
	default:
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"code":    "PASSIVE_LISTENER_MODE",
			"message": "Retena passive listener mode blocks outbound and mutating WhatsApp routes",
			"path":    c.Path(),
		})
	}
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

	// Chatwoot webhook - registered BEFORE basic auth middleware
	// This allows Chatwoot to send webhooks without authentication
	if config.ChatwootEnabled {
		chatwootHandler := rest.NewChatwootHandler(appUsecase, sendUsecase, dm, chatStorageRepo)
		webhookPath := "/chatwoot/webhook"
		if config.AppBasePath != "" {
			webhookPath = config.AppBasePath + webhookPath
		}
		app.Post(webhookPath, chatwootHandler.HandleWebhook)
	}

	if len(config.AppBasicAuthCredential) > 0 {
		account := make(map[string]string)
		for _, basicAuth := range config.AppBasicAuthCredential {
			ba := strings.Split(basicAuth, ":")
			if len(ba) != 2 {
				logrus.Fatalln("Basic auth is not valid, please this following format <user>:<secret>")
			}
			account[ba[0]] = ba[1]
		}

		app.Use(basicauth.New(basicauth.Config{
			Users: account,
		}))
	}

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
		chatwootHandler := rest.NewChatwootHandler(appUsecase, sendUsecase, dm, chatStorageRepo)
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
