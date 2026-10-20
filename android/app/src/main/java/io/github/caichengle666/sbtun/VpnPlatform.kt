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
        ContextCompat.startForegroundService(context, Intent(context, SbtunVpnService::class.java))
        val service = try {
            ready.get(10, TimeUnit.SECONDS)
        } catch (e: TimeoutException) {
            throw Exception("VPN 服务启动超时")
        }
        return service.establishTun(ipv6)
    }

    override fun closeTun() {
        SbtunVpnService.current?.shutdown()
    }
}
