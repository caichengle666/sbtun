//go:build linux

package tun

import "net"

// sing-box owns the Linux TUN route lifecycle. Killing it removes the TUN
// device, so cleanup does not need to mutate the user's default route.
func cleanupPlatform(name string) error { return nil }

func interfaceExists(name string) bool {
	_, err := net.InterfaceByName(name)
	return err == nil
}
