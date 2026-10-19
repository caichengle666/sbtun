//go:build darwin

package tun

import (
	"reflect"
	"testing"
)

func TestParseNetworkServices(t *testing.T) {
	out := "An asterisk (*) denotes that a network service is disabled.\nWi-Fi\n*Thunderbolt Bridge\nUSB 10/100/1000 LAN\n"
	if got, want := parseNetworkServices(out), []string{"Wi-Fi", "USB 10/100/1000 LAN"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}

func TestParseDNSServers(t *testing.T) {
	if got := parseDNSServers("There aren't any DNS Servers set on Wi-Fi.\n"); len(got) != 0 {
		t.Fatalf("expected DHCP DNS, got %v", got)
	}
	if got, want := parseDNSServers("8.8.8.8\n2001:4860:4860::8888\n"), []string{"8.8.8.8", "2001:4860:4860::8888"}; !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v want %v", got, want)
	}
}
