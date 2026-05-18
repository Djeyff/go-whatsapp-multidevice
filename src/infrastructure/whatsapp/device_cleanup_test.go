package whatsapp

import (
	"slices"
	"testing"
	"time"

	domainChatStorage "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/chatstorage"
	domainDevice "github.com/aldinokemal/go-whatsapp-web-multidevice/domains/device"
)

func TestUpdateStateFromClientDoesNotRefreshRepeatedDisconnectedObservation(t *testing.T) {
	stale := time.Now().Add(-2 * time.Hour)
	instance := NewDeviceInstance("stale-device", nil, nil)
	instance.state = domainDevice.DeviceStateDisconnected
	instance.lastSeenAt = stale

	state := instance.UpdateStateFromClient()

	if state != domainDevice.DeviceStateDisconnected {
		t.Fatalf("expected disconnected state, got %s", state)
	}
	if !instance.LastSeenAt().Equal(stale) {
		t.Fatalf("expected repeated disconnected observation to preserve lastSeenAt %s, got %s", stale, instance.LastSeenAt())
	}
}

func TestCleanupStaleDevicesRemovesAgedDisconnectedDevice(t *testing.T) {
	now := time.Now()
	manager := &DeviceManager{devices: make(map[string]*DeviceInstance)}
	instance := NewDeviceInstance("stale-device", nil, nil)
	instance.state = domainDevice.DeviceStateDisconnected
	instance.lastSeenAt = now.Add(-2 * time.Hour)
	manager.devices[instance.ID()] = instance

	report := manager.CleanupStaleDevicesWithOptions(now, StaleDeviceCleanupOptions{
		GracePeriod: 30 * time.Minute,
		MaxRemovals: 10,
	})

	if report.Removed != 1 {
		t.Fatalf("expected one stale device to be removed, got report %+v", report)
	}
	if !slices.Contains(report.RemovedIDs, "stale-device") {
		t.Fatalf("expected stale-device in removed ids, got %+v", report.RemovedIDs)
	}
	if _, ok := manager.GetDevice("stale-device"); ok {
		t.Fatal("expected stale disconnected device to be removed from manager")
	}
}

func TestCleanupStaleDevicesKeepsProtectedDevice(t *testing.T) {
	now := time.Now()
	manager := &DeviceManager{devices: make(map[string]*DeviceInstance)}
	instance := NewDeviceInstance("current-device", nil, nil)
	instance.state = domainDevice.DeviceStateDisconnected
	instance.lastSeenAt = now.Add(-2 * time.Hour)
	manager.devices[instance.ID()] = instance

	report := manager.CleanupStaleDevicesWithOptions(now, StaleDeviceCleanupOptions{
		GracePeriod:        30 * time.Minute,
		MaxRemovals:        10,
		ProtectedDeviceIDs: map[string]bool{"current-device": true},
	})

	if report.Removed != 0 || report.SkippedProtected != 1 {
		t.Fatalf("expected protected device to be skipped, got report %+v", report)
	}
	if _, ok := manager.GetDevice("current-device"); !ok {
		t.Fatal("expected protected device to remain in manager")
	}
}

func TestCleanupStaleDevicesDryRunAndCap(t *testing.T) {
	now := time.Now()
	manager := &DeviceManager{devices: make(map[string]*DeviceInstance)}
	for _, id := range []string{"stale-a", "stale-b", "stale-c"} {
		instance := NewDeviceInstance(id, nil, nil)
		instance.state = domainDevice.DeviceStateDisconnected
		instance.lastSeenAt = now.Add(-2 * time.Hour)
		manager.devices[instance.ID()] = instance
	}

	report := manager.CleanupStaleDevicesWithOptions(now, StaleDeviceCleanupOptions{
		GracePeriod: 30 * time.Minute,
		MaxRemovals: 2,
		DryRun:      true,
	})

	if report.Candidates != 3 || report.Removed != 0 || report.SkippedByCap != 1 {
		t.Fatalf("expected dry-run report to show three candidates and one capped item, got %+v", report)
	}
	for _, id := range []string{"stale-a", "stale-b", "stale-c"} {
		if _, ok := manager.GetDevice(id); !ok {
			t.Fatalf("expected dry-run to keep %s in manager", id)
		}
	}
}

func TestLoadFromRegistryPreservesRecordTimestampsForCleanup(t *testing.T) {
	now := time.Now()
	createdAt := now.Add(-4 * time.Hour)
	updatedAt := now.Add(-2 * time.Hour)
	manager := &DeviceManager{devices: make(map[string]*DeviceInstance)}

	manager.loadFromRegistry([]*domainChatStorage.DeviceRecord{{
		DeviceID:  "stale-registry-device",
		CreatedAt: createdAt,
		UpdatedAt: updatedAt,
	}})

	instance, ok := manager.GetDevice("stale-registry-device")
	if !ok {
		t.Fatal("expected device to load from registry")
	}
	if !instance.CreatedAt().Equal(createdAt) {
		t.Fatalf("expected createdAt %s, got %s", createdAt, instance.CreatedAt())
	}
	if !instance.LastSeenAt().Equal(updatedAt) {
		t.Fatalf("expected lastSeenAt %s, got %s", updatedAt, instance.LastSeenAt())
	}
}
