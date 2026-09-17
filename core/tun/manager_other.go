//go:build !windows && !linux

package tun

import "net"

func cleanupPlatform(name string) error { return nil }

func cleanupRoutesPlatform(name string) error { return nil }

func interfaceExists(name string) bool {
	_, err := net.InterfaceByName(name)
	return err == nil
}
