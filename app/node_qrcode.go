package app

import (
	"encoding/base64"
	"fmt"

	qrcode "github.com/skip2/go-qrcode"
)

type NodeQRCodeDTO struct {
	Link  string `json:"link"`
	Image string `json:"image"`
}

// NodeQRCode 返回节点分享链接及其二维码（PNG data URL）。
func (a *App) NodeQRCode(id string) (NodeQRCodeDTO, error) {
	links, err := a.ExportNodes([]string{id})
	if err != nil {
		return NodeQRCodeDTO{}, err
	}
	png, err := qrcode.Encode(links[0], qrcode.Medium, 320)
	if err != nil {
		return NodeQRCodeDTO{}, fmt.Errorf("生成二维码失败: %w", err)
	}
	return NodeQRCodeDTO{
		Link:  links[0],
		Image: "data:image/png;base64," + base64.StdEncoding.EncodeToString(png),
	}, nil
}

// TerminalQRCode 把文本渲染成终端可扫描的二维码。
func TerminalQRCode(text string) (string, error) {
	code, err := qrcode.New(text, qrcode.Low)
	if err != nil {
		return "", fmt.Errorf("生成二维码失败: %w", err)
	}
	return code.ToSmallString(false), nil
}
