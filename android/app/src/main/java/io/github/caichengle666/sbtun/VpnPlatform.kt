package io.github.caichengle666.sbtun

import android.content.Context
import androidx.core.content.ContextCompat
import java.util.concurrent.TimeUnit
import java.util.concurrent.TimeoutException
import mobile.Platform

// Go 在连接时从后台线程调用：拉起 VPN 服务并返回 TUN 文件描述符。
class VpnPlatform(private val context: Context) : Platform {
    override fun openTun(ipv6: Boolean): Int {
        // 服务已在运行（切换 IPv6 重建接口、系统拉起后自动连接）：直接建立，不再拉起前台服务。
        SbtunVpnService.current?.let { return it.establishTun(ipv6) }
        val (intent, ready) = SbtunVpnService.request(context)
        return try {
            try {
                ContextCompat.startForegroundService(context, intent)
            } catch (e: IllegalStateException) { // 后台不允许启动服务（Android 8+，12+ 为其子类）
                throw Exception("sbtun 在后台时系统不允许启动 VPN，请打开 sbtun 后再连接")
            }
            ready.get(10, TimeUnit.SECONDS).establishTun(ipv6)
        } catch (e: Exception) {
            // 不能 stopService：服务可能还欠一次 startForeground，被直接停掉系统会杀进程。
            SbtunVpnService.abandon(intent)
            throw if (e is TimeoutException) Exception("VPN 服务启动超时，请重试") else e
        }
    }

    override fun closeTun() {
        SbtunVpnService.current?.shutdown()
    }
}
