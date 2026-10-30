package app

import (
	"io"
	"time"
)

// DownloadProgress 报告下载进度：kind 是 "app" 或 "kernel"，total<=0 表示大小未知。
type DownloadProgress func(kind string, done, total int64)

// SetDownloadProgress 让 CLI 自己显示进度；GUI 不设置，进度以事件发给界面。
func (a *App) SetDownloadProgress(report DownloadProgress) { a.downloadProgress = report }

func (a *App) reportDownload(kind string, done, total int64) {
	if a.downloadProgress != nil {
		a.downloadProgress(kind, done, total)
	} else if a.gui {
		a.emitDownloadProgress(kind, done, total)
	}
}

// progressReader 包住下载流，最多每 200ms 报告一次，读完（EOF）时再报告一次。
type progressReader struct {
	r           io.Reader
	done, total int64
	last        time.Time
	report      func(done, total int64)
}

func (a *App) trackDownload(kind string, r io.Reader, total int64) *progressReader {
	return &progressReader{r: r, total: total, report: func(done, total int64) { a.reportDownload(kind, done, total) }}
}

func (p *progressReader) Read(b []byte) (int, error) {
	n, err := p.r.Read(b)
	p.done += int64(n)
	if now := time.Now(); err == io.EOF || now.Sub(p.last) >= 200*time.Millisecond {
		p.last = now
		p.report(p.done, p.total)
	}
	return n, err
}
