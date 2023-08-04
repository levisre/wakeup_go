package main

import (
	"github.com/mdlayher/wol"
	"log"
	"net"
)

func wakeRaw(iFace string, target net.HardwareAddr, password []byte) error {

	ifi, err := net.InterfaceByName(iFace)
	if err != nil {
		return err
	}

	c, err := wol.NewRawClient(ifi)
	if err != nil {
		return err
	}
	defer func(c *wol.RawClient) {
		_ = c.Close()
	}(c)

	// Attempt to wake target machine.
	return c.WakePassword(target, password)
}

func wakeUDP(addr string, target net.HardwareAddr, password []byte) error {
	log.Printf("sent UDP Wake-on-LAN magic packet using %s to %s", addr, target)
	c, err := wol.NewClient()
	if err != nil {
		return err
	}
	defer func(c *wol.Client) {
		err := c.Close()
		if err != nil {

		}
	}(c)
	// Attempt to wake target machine.
	return c.WakePassword(addr, target, password)
}
