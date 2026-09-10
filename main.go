package main

import (
	"fmt"
	"runtime"
)

func main() {
	fmt.Printf("sbtun - 轻量级 sing-box TUN 客户端\n")
	fmt.Printf("平台: %s/%s\n", runtime.GOOS, runtime.GOARCH)
}
