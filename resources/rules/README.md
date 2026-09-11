# 内置分流规则

sbtun 使用 sing-box 1.14 的二进制规则集（`.srs`），不再使用已经废弃的旧 `geoip.db/geosite.db` 路线。

构建时由 GitHub Actions 下载并校验以下规则集：

- `geosite-geolocation-cn.srs`：国内域名集合
- `geosite-geolocation-!cn.srs`：非国内域名集合
- `geoip-cn.srs`：中国大陆 IP 集合

这些文件不会手工提交到源码仓库，而是在 Windows Release 构建阶段生成并打包。这样可以避免仓库长期保存不断变化的大型二进制数据库，同时保证每次发行包都有明确的规则版本和 SHA-256。

规则来源：SagerNet/sing-geosite 与 SagerNet/sing-geoip。
