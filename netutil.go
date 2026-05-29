package main

import (
	"fmt"
	"net"
	"os"
	"strings"
)

// InterfaceInfo holds information about an active network interface.
type InterfaceInfo struct {
	Name      string        // Interface name (e.g. "eth0", "ens33")
	IP        net.IP        // IPv4 address assigned to the interface
	Network   *net.IPNet    // Subnet/CIDR the interface belongs to
	Interface net.Interface // Underlying net.Interface
}

// isPhysicalInterface checks whether a network interface is a physical device
// (e.g. eth0, ens33, wlp2s0) rather than a virtual one (docker0, virbr0, tun0, etc.).
// On Linux it checks sysfs; on other platforms it falls back to name-prefix filtering.
func isPhysicalInterface(name string) bool {
	// On Linux, physical NICs have a "device" symlink under sysfs
	// that points to the PCI/USB bus device. Virtual interfaces lack this.
	devicePath := "/sys/class/net/" + name + "/device"
	if _, err := os.Lstat(devicePath); err == nil {
		return true
	}

	// If the sysfs directory for the interface exists but has no "device" link,
	// it's a virtual interface on Linux — reject it.
	sysDir := "/sys/class/net/" + name
	if _, err := os.Stat(sysDir); err == nil {
		return false
	}

	// Non-Linux fallback: reject known virtual interface name prefixes
	virtualPrefixes := []string{
		"docker", "br-", "veth", "virbr", "vnet",
		"tun", "tap", "wg", "tailscale", "zt", "wt",
		"vmnet", "vbox",
	}
	nameLower := strings.ToLower(name)
	for _, prefix := range virtualPrefixes {
		if strings.HasPrefix(nameLower, prefix) {
			return false
		}
	}
	return true
}

// GetActiveInterfaces returns a list of physical network interfaces that are UP,
// not loopback, not virtual, and have at least one IPv4 address assigned.
func GetActiveInterfaces() ([]InterfaceInfo, error) {
	ifaces, err := net.Interfaces()
	if err != nil {
		return nil, fmt.Errorf("failed to list interfaces: %w", err)
	}

	var result []InterfaceInfo
	for _, iface := range ifaces {
		// Skip interfaces that are not up
		if iface.Flags&net.FlagUp == 0 {
			continue
		}
		// Skip loopback interfaces
		if iface.Flags&net.FlagLoopback != 0 {
			continue
		}
		// Skip virtual interfaces (docker, vpn, bridges, etc.)
		if !isPhysicalInterface(iface.Name) {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			continue
		}

		for _, addr := range addrs {
			ipNet, ok := addr.(*net.IPNet)
			if !ok {
				continue
			}
			// Only consider IPv4 addresses
			ipv4 := ipNet.IP.To4()
			if ipv4 == nil {
				continue
			}

			result = append(result, InterfaceInfo{
				Name:      iface.Name,
				IP:        ipv4,
				Network:   ipNet,
				Interface: iface,
			})
		}
	}

	if len(result) == 0 {
		return nil, fmt.Errorf("no active network interfaces found")
	}

	return result, nil
}

// FindInterfaceForIP finds the network interface whose subnet contains the
// given target IP address. The targetIP can be in "ip" or "ip:port" format.
// Returns nil (with no error) if no matching interface is found.
func FindInterfaceForIP(targetIP string, interfaces []InterfaceInfo) (*InterfaceInfo, error) {
	// Strip port if present (e.g. "192.168.1.100:9" -> "192.168.1.100")
	host, _, err := net.SplitHostPort(targetIP)
	if err != nil {
		// No port present, use as-is
		host = targetIP
	}

	ip := net.ParseIP(host)
	if ip == nil {
		return nil, fmt.Errorf("invalid IP address: %s", host)
	}

	// Convert to IPv4 for consistent comparison
	ipv4 := ip.To4()
	if ipv4 == nil {
		return nil, fmt.Errorf("not an IPv4 address: %s", host)
	}

	for i := range interfaces {
		if interfaces[i].Network.Contains(ipv4) {
			return &interfaces[i], nil
		}
	}

	return nil, nil
}
