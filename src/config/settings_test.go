package config

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
)

func TestDefaultLinkedDeviceIdentityIsRetenaDesktop(t *testing.T) {
	if AppOs != "Retena" {
		t.Fatalf("AppOs = %q, want Retena", AppOs)
	}
	if RetenaProductVersion != "2.2.1" {
		t.Fatalf("RetenaProductVersion = %q, want 2.2.1", RetenaProductVersion)
	}
	if AppPlatform != waCompanionReg.DeviceProps_DESKTOP {
		t.Fatalf("AppPlatform = %s, want DESKTOP", AppPlatform)
	}
}
