package config

import "testing"

func TestLinkedDeviceDisplayNameUsesRetenaProductVersion(t *testing.T) {
	t.Setenv("GOWA_INSTANCE_ID", "")
	t.Setenv("RETENA_GOWA_INSTANCE_ID", "")
	t.Setenv("RETENA_HA_ROLE", "")

	got := LinkedDeviceDisplayName()
	want := "Retena v" + RetenaProductVersion
	if got != want {
		t.Fatalf("LinkedDeviceDisplayName() = %q, want %q", got, want)
	}
	if got == "Chrome "+AppVersion || got == "Retena "+AppVersion {
		t.Fatalf("linked device name leaked GOWA AppVersion %q", AppVersion)
	}
}

func TestLinkedDeviceDisplayNameBlueUsesRetenaProductVersion(t *testing.T) {
	t.Setenv("GOWA_INSTANCE_ID", "gowa-blue")
	t.Setenv("RETENA_GOWA_INSTANCE_ID", "")
	t.Setenv("RETENA_HA_ROLE", "")

	got := LinkedDeviceDisplayName()
	want := "Retena Blue v" + RetenaProductVersion
	if got != want {
		t.Fatalf("LinkedDeviceDisplayName() = %q, want %q", got, want)
	}
}

func TestApplyLinkedDeviceOsOverrideIgnoresChrome(t *testing.T) {
	previous := AppOs
	t.Cleanup(func() { AppOs = previous })
	AppOs = "Retena"
	ApplyLinkedDeviceOsOverride("Chrome")
	if AppOs != "Retena" {
		t.Fatalf("AppOs = %q after Chrome override, want Retena", AppOs)
	}
}
