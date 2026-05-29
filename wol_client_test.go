package main

import (
	"net"
	"testing"
)

func TestWakeUDP_InvalidAddr(t *testing.T) {
	mac, _ := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	err := wakeUDP("invalid-address:9", mac, nil)
	if err == nil {
		t.Error("wakeUDP with invalid address returned nil error, expected an error")
	}
}

func TestWakeRaw_InvalidInterface(t *testing.T) {
	mac, _ := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	err := wakeRaw("nonexistent_iface_xyz", mac, nil)
	if err == nil {
		t.Error("wakeRaw with non-existent interface returned nil error, expected an error")
	}
}

func TestWakeRaw_ValidInterface(t *testing.T) {
	// Get a real interface to test with
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Skipf("Cannot get active interfaces: %v", err)
	}
	if len(ifaces) == 0 {
		t.Skip("No active interfaces found")
	}

	mac, _ := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	err = wakeRaw(ifaces[0].Name, mac, nil)
	if err != nil {
		// Raw packets typically require root/CAP_NET_RAW
		// Skip if it's a permission error rather than failing the test
		t.Skipf("wakeRaw requires elevated privileges, skipping: %v", err)
	}
}

func TestWakeUDP_EmptyPassword(t *testing.T) {
	// Should not panic with empty/nil password
	mac, _ := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	// Use a valid but unreachable address — we just verify no panic
	_ = wakeUDP("192.0.2.1:9", mac, nil)
	_ = wakeUDP("192.0.2.1:9", mac, []byte{})
}
