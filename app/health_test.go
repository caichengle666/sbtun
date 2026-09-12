package app

import (
	"context"
	"net"
	"testing"

	"github.com/caichengle666/sbtun/config"
)

func TestTestNodeReportsReachableTCPPort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	address := listener.Addr().(*net.TCPAddr)
	result := testNode(context.Background(), config.Node{ID: "n1", Server: address.IP.String(), Port: uint16(address.Port)})
	if !result.Port.OK || result.Port.Latency < 0 {
		t.Fatalf("unexpected health result: %+v", result)
	}
}

func TestTestNodeReportsUnavailablePort(t *testing.T) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	address := listener.Addr().(*net.TCPAddr)
	listener.Close()

	result := testNode(context.Background(), config.Node{ID: "n1", Server: address.IP.String(), Port: uint16(address.Port)})
	if result.Port.OK || result.Message == "" {
		t.Fatalf("unexpected health result: %+v", result)
	}
}

func TestTestNodeUsesUDPForHysteria2(t *testing.T) {
	listener, err := net.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("127.0.0.1")})
	if err != nil {
		t.Fatal(err)
	}
	defer listener.Close()

	address := listener.LocalAddr().(*net.UDPAddr)
	result := testNode(context.Background(), config.Node{ID: "hy2", Protocol: "hysteria2", Server: address.IP.String(), Port: uint16(address.Port)})
	if !result.Port.OK || result.Transport != "udp" || result.Port.Message != "UDP 可发送" {
		t.Fatalf("unexpected hysteria2 health result: %+v", result)
	}
}
