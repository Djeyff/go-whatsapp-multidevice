package config

import (
	"os"
	"strings"
)

const (
	genericLinkedDeviceOsChrome = "chrome"
	genericLinkedDeviceOsGowa   = "gowa"
)

// LinkedDeviceDisplayName is the WhatsApp linked-device label.
// Main: "Retena v2.2.1". Blue: "Retena Blue v2.2.1".
// APP_OS=Chrome/GOWA must not leak into that label.
func LinkedDeviceDisplayName() string {
	version := strings.TrimSpace(RetenaProductVersion)
	if version == "" {
		version = "0.0"
	}
	if !strings.HasPrefix(strings.ToLower(version), "v") {
		version = "v" + version
	}
	if isBlueGowaInstance() {
		return "Retena Blue " + version
	}
	return "Retena " + version
}

func isBlueGowaInstance() bool {
	for _, raw := range []string{
		os.Getenv("GOWA_INSTANCE_ID"),
		os.Getenv("RETENA_GOWA_INSTANCE_ID"),
		os.Getenv("RETENA_HA_ROLE"),
	} {
		value := strings.ToLower(strings.TrimSpace(raw))
		if value == "gowa-blue" || value == "blue" {
			return true
		}
	}
	return false
}

func isGenericLinkedDeviceOs(value string) bool {
	normalized := strings.ToLower(strings.TrimSpace(value))
	return normalized == "" ||
		normalized == genericLinkedDeviceOsChrome ||
		normalized == genericLinkedDeviceOsGowa
}

// ApplyLinkedDeviceOsOverride keeps APP_OS/--os for observability, but ignores
// generic Chrome/GOWA so live env cannot rename the linked device.
func ApplyLinkedDeviceOsOverride(value string) {
	if isGenericLinkedDeviceOs(value) {
		return
	}
	AppOs = strings.TrimSpace(value)
}
