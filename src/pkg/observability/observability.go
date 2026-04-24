package observability

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"os"
	"strings"
	"time"

	"github.com/getsentry/sentry-go"
	"github.com/sirupsen/logrus"
)

type betterStackHook struct {
	endpoint string
	service  string
	version  string
	commit   string
	client   *http.Client
}

type betterStackPayload struct {
	Dt      string         `json:"dt"`
	Level   string         `json:"level"`
	Message string         `json:"message"`
	Service string         `json:"service"`
	Version string         `json:"version,omitempty"`
	Commit  string         `json:"commit,omitempty"`
	Data    map[string]any `json:"data,omitempty"`
}

func Setup(service string) {
	logrus.SetFormatter(&logrus.JSONFormatter{TimestampFormat: time.RFC3339Nano})
	if hook, err := newBetterStackHook(service); err != nil {
		logrus.WithError(err).Warn("[betterstack] hook disabled")
	} else if hook != nil {
		logrus.AddHook(hook)
		logrus.WithField("service", service).Info("[betterstack] log shipping enabled")
	}
}

func Status() map[string]bool {
	return map[string]bool{
		"sentry":      strings.TrimSpace(os.Getenv("SENTRY_DSN")) != "",
		"betterStack": betterStackConfigured(),
	}
}

func CaptureSyntheticSentry(service string, tags map[string]string, extra map[string]any) (string, bool) {
	if strings.TrimSpace(os.Getenv("SENTRY_DSN")) == "" {
		return "", false
	}
	message := fmt.Sprintf("%s synthetic sentry smoke @ %s", service, time.Now().UTC().Format(time.RFC3339))
	if hub := sentry.CurrentHub(); hub != nil {
		hub.WithScope(func(scope *sentry.Scope) {
			for k, v := range tags {
				scope.SetTag(k, v)
			}
			for k, v := range extra {
				scope.SetExtra(k, v)
			}
			hub.CaptureException(&syntheticError{message: message})
		})
	}
	return message, true
}

type syntheticError struct{ message string }

func (e *syntheticError) Error() string { return e.message }

func betterStackConfigured() bool {
	return strings.TrimSpace(firstNonEmpty(os.Getenv("BETTER_STACK_SOURCE_TOKEN"), os.Getenv("BETTERSTACK_SOURCE_TOKEN"))) != "" &&
		strings.TrimSpace(firstNonEmpty(os.Getenv("BETTER_STACK_INGEST_HOST"), os.Getenv("BETTER_STACK_INGESTING_HOST"), os.Getenv("BETTERSTACK_INGEST_HOST"))) != ""
}

func newBetterStackHook(service string) (*betterStackHook, error) {
	endpoint := betterStackEndpoint()
	if endpoint == "" {
		return nil, nil
	}
	return &betterStackHook{
		endpoint: endpoint,
		service:  service,
		version:  strings.TrimSpace(firstNonEmpty(os.Getenv("SERVICE_VERSION"), os.Getenv("APP_VERSION"), "dev")),
		commit:   strings.TrimSpace(firstNonEmpty(os.Getenv("COMMIT_SHA"), os.Getenv("GIT_COMMIT"), "unknown")),
		client:   &http.Client{Timeout: 5 * time.Second},
	}, nil
}

func (h *betterStackHook) Levels() []logrus.Level {
	return logrus.AllLevels
}

func (h *betterStackHook) Fire(entry *logrus.Entry) error {
	data := map[string]any{}
	for k, v := range entry.Data {
		data[k] = v
	}
	payload := betterStackPayload{
		Dt:      entry.Time.UTC().Format(time.RFC3339Nano),
		Level:   entry.Level.String(),
		Message: entry.Message,
		Service: h.service,
		Version: h.version,
		Commit:  h.commit,
		Data:    data,
	}
	body, err := json.Marshal(payload)
	if err != nil {
		return err
	}
	go func(buf []byte) {
		req, reqErr := http.NewRequest(http.MethodPost, h.endpoint, bytes.NewBuffer(buf))
		if reqErr != nil {
			return
		}
		req.Header.Set("Content-Type", "application/json")
		resp, postErr := h.client.Do(req)
		if postErr != nil {
			return
		}
		_ = resp.Body.Close()
	}([]byte(body))
	return nil
}

func betterStackEndpoint() string {
	host := strings.TrimSpace(firstNonEmpty(os.Getenv("BETTER_STACK_INGEST_HOST"), os.Getenv("BETTER_STACK_INGESTING_HOST"), os.Getenv("BETTERSTACK_INGEST_HOST")))
	token := strings.TrimSpace(firstNonEmpty(os.Getenv("BETTER_STACK_SOURCE_TOKEN"), os.Getenv("BETTERSTACK_SOURCE_TOKEN")))
	if host == "" || token == "" {
		return ""
	}
	if !strings.HasPrefix(host, "http://") && !strings.HasPrefix(host, "https://") {
		host = "https://" + host
	}
	host = strings.TrimRight(host, "/")
	return fmt.Sprintf("%s/?source_token=%s", host, token)
}

func firstNonEmpty(values ...string) string {
	for _, v := range values {
		if strings.TrimSpace(v) != "" {
			return v
		}
	}
	return ""
}
