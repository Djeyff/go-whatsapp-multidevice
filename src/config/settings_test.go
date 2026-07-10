package config

import (
	"testing"

	"go.mau.fi/whatsmeow/proto/waCompanionReg"
)

func TestDefaultLinkedDeviceIdentityIsRetenaDesktop(t *testing.T) {
	if AppOs != "Retena" {
		t.Fatalf("AppOs = %q, want Retena", AppOs)
	}
	if AppPlatform != waCompanionReg.DeviceProps_DESKTOP {
		t.Fatalf("AppPlatform = %s, want DESKTOP", AppPlatform)
	}
}
