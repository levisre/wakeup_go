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

func TestIsBridgeWithPhysicalPort(t *testing.T) {
	// Docker/virtual-only bridges should return false
	virtualBridges := []string{"docker0", "br-abc123"}
	for _, name := range virtualBridges {
		if isBridgeWithPhysicalPort(name) {
			t.Errorf("isBridgeWithPhysicalPort(%q) = true, expected false (virtual-only bridge)", name)
		}
	}

	// Non-bridge interfaces should return false
	nonBridges := []string{"lo", "nonexistent_iface"}
	for _, name := range nonBridges {
		if isBridgeWithPhysicalPort(name) {
			t.Errorf("isBridgeWithPhysicalPort(%q) = true, expected false (not a bridge)", name)
		}
	}

	// On this machine, check if any active bridge has a physical port
	// We detect this dynamically rather than hard-coding interface names
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Skipf("No active interfaces: %v", err)
	}
	for _, iface := range ifaces {
		if isBridgeWithPhysicalPort(iface.Name) {
			t.Logf("Found bridge with physical port: %s (IP: %s)", iface.Name, iface.IP)
		}
	}
}

func TestGetActiveInterfaces_IncludesBridgeWithPhysicalPort(t *testing.T) {
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Fatalf("GetActiveInterfaces() returned error: %v", err)
	}

	// Verify that any bridge-backed-by-physical-port on this system is included
	foundBridge := false
	for _, iface := range ifaces {
		if isBridgeWithPhysicalPort(iface.Name) {
			foundBridge = true
			t.Logf("Bridge %s included with IP %s/%s", iface.Name, iface.IP, iface.Network)

			// Verify it has valid fields
			if iface.IP == nil {
				t.Errorf("Bridge %s has nil IP", iface.Name)
			}
			if iface.Network == nil {
				t.Errorf("Bridge %s has nil Network", iface.Name)
			}
			if iface.Interface.Flags&net.FlagUp == 0 {
				t.Errorf("Bridge %s is not UP", iface.Name)
			}
		}
	}

	if !foundBridge {
		t.Skip("No bridge with physical port found on this system, skipping")
	}
}

func TestBroadcastAddr(t *testing.T) {
	tests := []struct {
		cidr     string
		expected string
	}{
		{"192.168.1.0/24", "192.168.1.255"},
		{"172.16.1.0/27", "172.16.1.31"},
		{"10.0.0.0/8", "10.255.255.255"},
		{"192.168.1.128/25", "192.168.1.255"},
	}

	for _, tc := range tests {
		_, network, err := net.ParseCIDR(tc.cidr)
		if err != nil {
			t.Fatalf("Failed to parse CIDR %s: %v", tc.cidr, err)
		}
		bcast := BroadcastAddr(network)
		if bcast == nil {
			t.Errorf("BroadcastAddr(%s) returned nil", tc.cidr)
			continue
		}
		if bcast.String() != tc.expected {
			t.Errorf("BroadcastAddr(%s) = %s, expected %s", tc.cidr, bcast, tc.expected)
		}
	}
}

func TestLookupIPByMAC_KnownEntry(t *testing.T) {
	// Use the gateway MAC from the ARP table — it should always be present
	// on a machine with active network connections
	ifaces, err := GetActiveInterfaces()
	if err != nil {
		t.Skipf("No active interfaces: %v", err)
	}

	// The machine's own MAC won't be in the ARP table,
	// but any peer we've communicated with will be.
	// Just verify the function doesn't error out.
	for _, iface := range ifaces {
		mac := iface.Interface.HardwareAddr
		if mac == nil {
			continue
		}
		ip, err := LookupIPByMAC(mac)
		if err != nil {
			t.Errorf("LookupIPByMAC(%s) returned error: %v", mac, err)
		}
		if ip != nil {
			t.Logf("Found ARP entry: %s -> %s", mac, ip)
		}
	}
}

func TestLookupIPByMAC_NotFound(t *testing.T) {
	// A fabricated MAC should not be in the ARP table
	mac, _ := net.ParseMAC("AA:BB:CC:DD:EE:FF")
	ip, err := LookupIPByMAC(mac)
	if err != nil {
		t.Fatalf("LookupIPByMAC returned error: %v", err)
	}
	if ip != nil {
		t.Errorf("LookupIPByMAC(AA:BB:CC:DD:EE:FF) returned %s, expected nil", ip)
	}
}
