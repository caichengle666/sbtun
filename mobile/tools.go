//go:build tools

package mobile

// gomobile bind 需要模块里有 golang.org/x/mobile。
import _ "golang.org/x/mobile/bind"
