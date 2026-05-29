package main

import (
	"testing"
)

func TestPcPing_Localhost(t *testing.T) {
	// Localhost should always be reachable
	if !PcPing("127.0.0.1") {
		t.Error("PcPing(127.0.0.1) returned false, expected true")
	}
}

func TestPcPing_Unreachable(t *testing.T) {
	// TEST-NET-1 (RFC 5737) — should not be reachable
	if PcPing("192.0.2.1") {
		t.Error("PcPing(192.0.2.1) returned true, expected false (unreachable)")
	}
}

func TestPcPing_WithPort(t *testing.T) {
	// Should strip port and still ping localhost successfully
	if !PcPing("127.0.0.1:9") {
		t.Error("PcPing(127.0.0.1:9) returned false, expected true (port should be stripped)")
	}
}

func TestPcPing_InvalidAddress(t *testing.T) {
	// Garbage input should return false, not panic
	if PcPing("not-a-valid-address") {
		t.Error("PcPing(not-a-valid-address) returned true, expected false")
	}
}
