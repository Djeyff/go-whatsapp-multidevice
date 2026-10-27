package whatsapp

import (
	"bytes"
	"context"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/aldinokemal/go-whatsapp-web-multidevice/config"
	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	"github.com/sirupsen/logrus"
)

// captureLogrus redirects the standard logger into a buffer for the duration
// of the test (explicit defer/restore, matching the fork's test seams).
func captureLogrus(t *testing.T) *bytes.Buffer {
	t.Helper()
	var buf bytes.Buffer
	logger := logrus.StandardLogger()
	prevOut := logger.Out
	prevLevel := logger.GetLevel()
	logrus.SetOutput(&buf)
	logrus.SetLevel(logrus.InfoLevel)
	t.Cleanup(func() {
		logrus.SetOutput(prevOut)
		logrus.SetLevel(prevLevel)
	})
	return &buf
}

func withAutoConnectSeams(t *testing.T, lookup func(string) (*domainChatStorage.DeviceRecord, error), identity func(context.Context, string) (bool, error)) {
	t.Helper()
	origLookup := autoConnectRecordLookupFn
	origIdentity := autoConnectIdentityCheckFn
	autoConnectRecordLookupFn = lookup
	autoConnectIdentityCheckFn = identity
	t.Cleanup(func() {
		autoConnectRecordLookupFn = origLookup
		autoConnectIdentityCheckFn = origIdentity
	})
}

func TestSessionIdentityForDevice(t *testing.T) {
	cases := []struct {
		name     string
		deviceID string
		jid      string
		want     string
	}{
		{"jid with agent device suffix", "", "584146485729:71@s.whatsapp.net", "584146485729@s.whatsapp.net"},
		{"device id that is a bare jid", "18498837876@s.whatsapp.net", "", "18498837876@s.whatsapp.net"},
		{"device id with suffix and empty jid", "18498837876:5@s.whatsapp.net", "", "18498837876@s.whatsapp.net"},
		{"jid wins over device id", "org_2", "18498837876:5@s.whatsapp.net", "18498837876@s.whatsapp.net"},
		{"manual id passthrough", "org_2", "", "org_2"},
		{"both empty", "", "", ""},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := SessionIdentityForDevice(tc.deviceID, tc.jid); got != tc.want {
				t.Fatalf("SessionIdentityForDevice(%q, %q) = %q, want %q", tc.deviceID, tc.jid, got, tc.want)
			}
		})
	}
}

func TestShouldAutoConnectSkipsUnregisteredSessionWithWarn(t *testing.T) {
	buf := captureLogrus(t)
	withAutoConnectSeams(t,
		func(string) (*domainChatStorage.DeviceRecord, error) { return nil, nil },
		func(context.Context, string) (bool, error) { return true, nil },
	)

	verdict := ShouldAutoConnect(context.Background(), "584146485729@s.whatsapp.net", "")
	if verdict.Allow {
		t.Fatalf("unregistered session must be skipped, got %+v", verdict)
	}
	if verdict.Reason != AutoConnectSkipNoRegisteredRecord {
		t.Fatalf("reason = %q, want %q", verdict.Reason, AutoConnectSkipNoRegisteredRecord)
	}
	if !strings.Contains(buf.String(), "no registered device record") {
		t.Fatalf("expected a Warn for the skip, log was: %q", buf.String())
	}
}

func TestShouldAutoConnectSkipsIdentityDisconnectedSessionWithWarn(t *testing.T) {
	buf := captureLogrus(t)
	withAutoConnectSeams(t,
		func(string) (*domainChatStorage.DeviceRecord, error) { return &domainChatStorage.DeviceRecord{}, nil },
		func(context.Context, string) (bool, error) { return false, nil },
	)

	verdict := ShouldAutoConnect(context.Background(), "584146485729@s.whatsapp.net", "584146485729:71@s.whatsapp.net")
	if verdict.Allow {
		t.Fatalf("identity-disconnected session must be skipped, got %+v", verdict)
	}
	if verdict.Reason != AutoConnectSkipIdentityDisconnected {
		t.Fatalf("reason = %q, want %q", verdict.Reason, AutoConnectSkipIdentityDisconnected)
	}
	if !strings.Contains(buf.String(), "session disconnected") {
		t.Fatalf("expected a Warn for the skip, log was: %q", buf.String())
	}
}

func TestShouldAutoConnectAllowsRegisteredConnectedSession(t *testing.T) {
	buf := captureLogrus(t)
	withAutoConnectSeams(t,
		func(string) (*domainChatStorage.DeviceRecord, error) { return &domainChatStorage.DeviceRecord{}, nil },
		func(context.Context, string) (bool, error) { return true, nil },
	)

	verdict := ShouldAutoConnect(context.Background(), "18498837876@s.whatsapp.net", "18498837876:9@s.whatsapp.net")
	if !verdict.Allow || verdict.Reason != "" {
		t.Fatalf("registered connected session must be allowed, got %+v", verdict)
	}
	if strings.Contains(buf.String(), "skipping session") {
		t.Fatalf("no skip warn expected, log was: %q", buf.String())
	}
}

func TestShouldAutoConnectFailsOpenOnError(t *testing.T) {
	cases := []struct {
		name     string
		lookup   func(string) (*domainChatStorage.DeviceRecord, error)
		identity func(context.Context, string) (bool, error)
	}{
		{
			name:     "registry lookup error",
			lookup:   func(string) (*domainChatStorage.DeviceRecord, error) { return nil, errors.New("db is closed") },
			identity: func(context.Context, string) (bool, error) { return true, nil },
		},
		{
			name:     "identity service error",
			lookup:   func(string) (*domainChatStorage.DeviceRecord, error) { return &domainChatStorage.DeviceRecord{}, nil },
			identity: func(context.Context, string) (bool, error) { return true, errors.New("dial timeout") },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			withAutoConnectSeams(t, tc.lookup, tc.identity)
			verdict := ShouldAutoConnect(context.Background(), "18498837876@s.whatsapp.net", "")
			if !verdict.Allow {
				t.Fatalf("gate must fail open, got %+v", verdict)
			}
		})
	}
}

func TestDefaultAutoConnectRecordLookupWithoutRegistry(t *testing.T) {
	globalStateMu.Lock()
	prevManager := deviceManager
	deviceManager = nil
	globalStateMu.Unlock()
	defer func() {
		globalStateMu.Lock()
		deviceManager = prevManager
		globalStateMu.Unlock()
	}()

	record, err := defaultAutoConnectRecordLookup("584146485729@s.whatsapp.net")
	if err != nil {
		t.Fatalf("no registry must not error, got %v", err)
	}
	if record != nil {
		t.Fatalf("no registry must return no record, got %+v", record)
	}
}

func TestDefaultAutoConnectIdentityCheck(t *testing.T) {
	prevURL := config.RetenaAutoConnectIdentityURL
	defer func() { config.RetenaAutoConnectIdentityURL = prevURL }()

	t.Run("disabled by default", func(t *testing.T) {
		config.RetenaAutoConnectIdentityURL = ""
		connected, err := defaultAutoConnectIdentityCheck(context.Background(), "18498837876@s.whatsapp.net")
		if err != nil || !connected {
			t.Fatalf("default-off check must allow, got (%v, %v)", connected, err)
		}
	})

	t.Run("substitutes jid into the url", func(t *testing.T) {
		var gotPath string
		ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			gotPath = r.URL.Path
			_, _ = w.Write([]byte(`{"connected": true}`))
		}))
		defer ts.Close()
		config.RetenaAutoConnectIdentityURL = ts.URL + "/wa-sessions/{jid}"
		connected, err := defaultAutoConnectIdentityCheck(context.Background(), "18498837876@s.whatsapp.net")
		if err != nil || !connected {
			t.Fatalf("expected (true, nil), got (%v, %v)", connected, err)
		}
		if gotPath != "/wa-sessions/18498837876@s.whatsapp.net" {
			t.Fatalf("jid was not substituted into path, got %q", gotPath)
		}
	})

	shapes := []struct {
		name     string
		body     string
		status   int
		wantLive bool
		wantErr  bool
	}{
		{"connected false", `{"connected": false}`, 200, false, false},
		{"connected true", `{"connected": true}`, 200, true, false},
		{"state disconnected", `{"state": "disconnected"}`, 200, false, false},
		{"state connected", `{"state": "connected"}`, 200, true, false},
		{"http 500", `{"connected": true}`, 500, true, true},
		{"malformed body", `not-json`, 200, true, true},
		{"empty shape", `{}`, 200, true, true},
	}
	for _, tc := range shapes {
		t.Run(tc.name, func(t *testing.T) {
			ts := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
				w.WriteHeader(tc.status)
				_, _ = w.Write([]byte(tc.body))
			}))
			defer ts.Close()
			config.RetenaAutoConnectIdentityURL = ts.URL + "/{jid}"
			live, err := defaultAutoConnectIdentityCheck(context.Background(), "18498837876@s.whatsapp.net")
			if (err != nil) != tc.wantErr {
				t.Fatalf("error = %v, wantErr %v", err, tc.wantErr)
			}
			if live != tc.wantLive {
				t.Fatalf("live = %v, want %v", live, tc.wantLive)
			}
		})
	}

	t.Run("template without placeholder errors", func(t *testing.T) {
		config.RetenaAutoConnectIdentityURL = "https://identity.example/sessions"
		_, err := defaultAutoConnectIdentityCheck(context.Background(), "18498837876@s.whatsapp.net")
		if err == nil {
			t.Fatal("expected error for missing {jid} placeholder")
		}
	})
}
