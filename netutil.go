package main

import (
	"bufio"
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

// isBridgeWithPhysicalPort checks whether a network interface is a Linux bridge
// that has at least one physical NIC enslaved as a port.
// This is useful because when a physical NIC (e.g. eno1) is a bridge port,
// it loses its IP address — the bridge holds the IP instead.
// Raw packets (including WOL) sent on the bridge are forwarded through the
// physical port to the wire, so such bridges are valid for WOL.
func isBridgeWithPhysicalPort(name string) bool {
	// Check if this interface is a bridge by looking for the bridge directory
	bridgePath := "/sys/class/net/" + name + "/bridge"
	fi, err := os.Stat(bridgePath)
	if err != nil || !fi.IsDir() {
		return false
	}

	// List bridge ports from /sys/class/net/<bridge>/brif/
	brIfPath := "/sys/class/net/" + name + "/brif"
	ports, err := os.ReadDir(brIfPath)
	if err != nil {
		return false
	}

	// Check if any port is a physical device (has a "device" symlink)
	for _, port := range ports {
		portDevicePath := "/sys/class/net/" + port.Name() + "/device"
		if _, err := os.Lstat(portDevicePath); err == nil {
			return true
		}
	}
	return false
}

// isPhysicalInterface checks whether a network interface is a physical device
// (e.g. eth0, ens33, wlp2s0) or a bridge backed by a physical device,
// rather than a purely virtual one (docker0, virbr0, tun0, etc.).
// On Linux it checks sysfs; on other platforms it falls back to name-prefix filtering.
func isPhysicalInterface(name string) bool {
	// On Linux, physical NICs have a "device" symlink under sysfs
	// that points to the PCI/USB bus device. Virtual interfaces lack this.
	devicePath := "/sys/class/net/" + name + "/device"
	if _, err := os.Lstat(devicePath); err == nil {
		return true
	}

	// Check if this is a bridge with at least one physical port.
	// Such bridges can carry raw WOL packets to the wire.
	if isBridgeWithPhysicalPort(name) {
		return true
	}

	// If the sysfs directory for the interface exists but has no "device" link
	// and is not a bridge with physical ports, it's a virtual interface — reject it.
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

// BroadcastAddr calculates the IPv4 broadcast address for a given subnet.
func BroadcastAddr(network *net.IPNet) net.IP {
	ip := network.IP.To4()
	if ip == nil {
		return nil
	}
	mask := network.Mask
	broadcast := make(net.IP, len(ip))
	for i := range ip {
		broadcast[i] = ip[i] | ^mask[i]
	}
	return broadcast
}

// LookupIPByMAC searches the system ARP table (/proc/net/arp) for an entry
// matching the given MAC address. Returns the associated IP address if found.
// Only returns entries with complete ARP resolution (flags 0x2).
//
// /proc/net/arp format:
//
//	IP address       HW type     Flags       HW address            Mask     Device
func LookupIPByMAC(mac net.HardwareAddr) (net.IP, error) {
	f, err := os.Open("/proc/net/arp")
	if err != nil {
		return nil, fmt.Errorf("failed to open /proc/net/arp: %w", err)
	}
	defer f.Close()

	targetMAC := strings.ToLower(mac.String())
	scanner := bufio.NewScanner(f)

	// Skip header line
	if !scanner.Scan() {
		return nil, fmt.Errorf("empty /proc/net/arp")
	}

	for scanner.Scan() {
		fields := strings.Fields(scanner.Text())
		if len(fields) < 4 {
			continue
		}

		// fields[0] = IP, fields[2] = Flags, fields[3] = HW address
		flags := fields[2]
		hwAddr := strings.ToLower(fields[3])

		// Only consider complete ARP entries (flags 0x2)
		if flags != "0x2" {
			continue
		}

		if hwAddr == targetMAC {
			ip := net.ParseIP(fields[0])
			if ip == nil {
				continue
			}
			return ip.To4(), nil
		}
	}

	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("error reading /proc/net/arp: %w", err)
	}

	return nil, nil // Not found
}
