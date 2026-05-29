package main

import (
	"net"
	"strings"
	"testing"
)

func TestGetActiveInterfaces(t *testing.T) {
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Fatalf("GetActiveInterfaces() returned error: %v", err)
	}

	if len(ifaces) == 0 {
		t.Fatal("GetActiveInterfaces() returned empty list, expected at least one active interface")
	}

	// Known virtual interface prefixes that should be filtered out
	virtualPrefixes := []string{
		"docker", "br-", "veth", "virbr", "vnet",
		"tun", "tap", "wg", "tailscale", "zt", "wt",
		"vmnet", "vbox",
	}

	for _, iface := range ifaces {
		// Verify each interface has required fields populated
		if iface.Name == "" {
			t.Error("Interface has empty name")
		}
		if iface.IP == nil {
			t.Errorf("Interface %s has nil IP", iface.Name)
		}
		if iface.Network == nil {
			t.Errorf("Interface %s has nil Network", iface.Name)
		}
		// Verify the interface is UP
		if iface.Interface.Flags&net.FlagUp == 0 {
			t.Errorf("Interface %s is not UP", iface.Name)
		}
		// Verify the interface is not loopback
		if iface.Interface.Flags&net.FlagLoopback != 0 {
			t.Errorf("Interface %s is loopback, should be excluded", iface.Name)
		}
		// Verify it's an IPv4 address
		if iface.IP.To4() == nil {
			t.Errorf("Interface %s IP %s is not IPv4", iface.Name, iface.IP)
		}
		// Verify no virtual interfaces slipped through
		nameLower := strings.ToLower(iface.Name)
		for _, prefix := range virtualPrefixes {
			if strings.HasPrefix(nameLower, prefix) {
				t.Errorf("Virtual interface %s should have been filtered out", iface.Name)
			}
		}

		t.Logf("Found physical interface: %s with IP %s/%s",
			iface.Name, iface.IP, iface.Network)
	}
}

func TestFindInterfaceForIP_Match(t *testing.T) {
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Fatalf("GetActiveInterfaces() failed: %v", err)
	}

	// Use the first active interface's IP — it should match itself
	target := ifaces[0].IP.String()
	matched, err := FindInterfaceForIP(target, ifaces)
	if err != nil {
		t.Fatalf("FindInterfaceForIP(%s) returned error: %v", target, err)
	}
	if matched == nil {
		t.Fatalf("FindInterfaceForIP(%s) returned nil, expected to match %s", target, ifaces[0].Name)
	}
	if matched.Name != ifaces[0].Name {
		t.Errorf("FindInterfaceForIP(%s) matched %s, expected %s", target, matched.Name, ifaces[0].Name)
	}
}

func TestFindInterfaceForIP_WithPort(t *testing.T) {
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Fatalf("GetActiveInterfaces() failed: %v", err)
	}

	// Use ip:port format — port should be stripped
	target := ifaces[0].IP.String() + ":9"
	matched, err := FindInterfaceForIP(target, ifaces)
	if err != nil {
		t.Fatalf("FindInterfaceForIP(%s) returned error: %v", target, err)
	}
	if matched == nil {
		t.Fatalf("FindInterfaceForIP(%s) returned nil, expected a match", target)
	}
	if matched.Name != ifaces[0].Name {
		t.Errorf("FindInterfaceForIP(%s) matched %s, expected %s", target, matched.Name, ifaces[0].Name)
	}
}

func TestFindInterfaceForIP_NoMatch(t *testing.T) {
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Fatalf("GetActiveInterfaces() failed: %v", err)
	}

	// Use TEST-NET-3 (RFC 5737) — should not match any local interface
	matched, err := FindInterfaceForIP("203.0.113.1", ifaces)
	if err != nil {
		t.Fatalf("FindInterfaceForIP(203.0.113.1) returned error: %v", err)
	}
	if matched != nil {
		t.Errorf("FindInterfaceForIP(203.0.113.1) returned %s, expected nil (no match)", matched.Name)
	}
}

func TestFindInterfaceForIP_InvalidIP(t *testing.T) {
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Fatalf("GetActiveInterfaces() failed: %v", err)
	}

	_, err = FindInterfaceForIP("not-an-ip", ifaces)
	if err == nil {
		t.Error("FindInterfaceForIP(not-an-ip) returned nil error, expected an error")
	}
}

func TestFindInterfaceForIP_EmptyList(t *testing.T) {
	// With an empty interface list, no match should be found
	matched, err := FindInterfaceForIP("192.168.1.1", nil)
	if err != nil {
		t.Fatalf("FindInterfaceForIP with nil list returned error: %v", err)
	}
	if matched != nil {
		t.Errorf("FindInterfaceForIP with nil list returned %s, expected nil", matched.Name)
	}
}

func TestFindInterfaceForIP_IPv6Rejected(t *testing.T) {
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Fatalf("GetActiveInterfaces() failed: %v", err)
	}

	_, err = FindInterfaceForIP("::1", ifaces)
	if err == nil {
		t.Error("FindInterfaceForIP(::1) returned nil error, expected IPv6 rejection")
	}
}

func TestIsPhysicalInterface_KnownVirtual(t *testing.T) {
	// These should all be identified as virtual on Linux (via sysfs)
	// or via name-prefix fallback on other platforms
	virtualNames := []string{
		"docker0", "br-abc123", "vethXYZ", "virbr0",
		"tun0", "tap0", "tailscale0", "wg0",
		"vmnet1", "vmnet8", "vboxnet0", "wt0",
	}
	for _, name := range virtualNames {
		if isPhysicalInterface(name) {
			t.Errorf("isPhysicalInterface(%q) = true, expected false (virtual)", name)
		}
	}
}

func TestIsPhysicalInterface_RealInterfaces(t *testing.T) {
	// Get actual physical interfaces from the system and verify they pass
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Skipf("No active interfaces to test: %v", err)
	}
	for _, iface := range ifaces {
		if !isPhysicalInterface(iface.Name) {
			t.Errorf("isPhysicalInterface(%q) = false, but it was returned by GetActiveInterfaces", iface.Name)
		}
	}
}
