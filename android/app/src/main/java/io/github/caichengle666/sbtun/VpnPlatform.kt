package io.github.caichengle666.sbtun

import android.content.Context
import android.content.Intent
import androidx.core.content.ContextCompat
import java.util.concurrent.TimeUnit
import java.util.concurrent.TimeoutException
import mobile.Platform

// Go 在用户点击连接时从后台线程调用：拉起 VPN 服务并返回 TUN 文件描述符。
class VpnPlatform(private val context: Context) : Platform {
    override fun openTun(ipv6: Boolean): Int {
        val ready = SbtunVpnService.whenReady()
        val intent = Intent(context, SbtunVpnService::class.java).putExtra(SbtunVpnService.EXTRA_FROM_APP, true)
        ContextCompat.startForegroundService(context, intent)
        return try {
            ready.get(10, TimeUnit.SECONDS).establishTun(ipv6)
        } catch (e: Exception) {
            // 失败时收掉前台服务和"已连接"通知；超时时服务可能稍后才起来，也一并停掉。
            SbtunVpnService.current?.shutdown()
            context.stopService(Intent(context, SbtunVpnService::class.java))
            throw if (e is TimeoutException) Exception("VPN 服务启动超时") else e
        }
    }

    override fun closeTun() {
        SbtunVpnService.current?.shutdown()
    }
}
