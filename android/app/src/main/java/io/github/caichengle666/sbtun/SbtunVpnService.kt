package io.github.caichengle666.sbtun

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Intent
import android.content.pm.ServiceInfo
import android.net.VpnService
import android.os.Build
import androidx.core.app.NotificationCompat
import androidx.core.app.ServiceCompat
import java.util.concurrent.CompletableFuture
import mobile.Mobile

class SbtunVpnService : VpnService() {
    companion object {
        private const val ACTION_STOP = "io.github.caichengle666.sbtun.STOP"
        private const val NOTIFICATION_ID = 1
        private const val CHANNEL_ID = "vpn"

        @Volatile
        var current: SbtunVpnService? = null
            private set
        private var ready = CompletableFuture<SbtunVpnService>()

        @Synchronized
        fun whenReady(): CompletableFuture<SbtunVpnService> = ready

        @Synchronized
        private fun attach(service: SbtunVpnService) {
            current = service
            ready.complete(service)
        }

        @Synchronized
        private fun detach(service: SbtunVpnService) {
            if (current !== service) return
            current = null
            ready = CompletableFuture()
        }
    }

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopAsync()
            return START_NOT_STICKY
        }
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTIFICATION_ID, notification(), ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIFICATION_ID, notification())
        }
        attach(this)
        return START_NOT_STICKY
    }

    // 用户在系统设置里关掉 VPN，或被其他 VPN 应用顶掉。
    override fun onRevoke() = stopAsync()

    override fun onDestroy() {
        detach(this)
        super.onDestroy()
    }

    fun establishTun(ipv6: Boolean): Int {
        val builder = Builder()
            .setSession("sbtun")
            .setMtu(Mobile.mtu().toInt())
            .addAddress("172.19.0.1", 30)
            .addRoute("0.0.0.0", 0)
            .addDnsServer("172.19.0.2")
            .addDisallowedApplication(packageName) // 内置的 sing-box 进程与应用同 UID，必须绕过 VPN
            .setConfigureIntent(openAppIntent())
        if (ipv6) builder.addAddress("fdfe:dcba:9876::1", 126).addRoute("::", 0)
        if (Build.VERSION.SDK_INT >= 29) builder.setMetered(false)
        val tun = builder.establish() ?: throw Exception("无法建立 VPN，请确认已允许 VPN 权限")
        return tun.detachFd()
    }

    fun shutdown() {
        detach(this) // 立即重新连接时等新的服务实例，而不是这个正在退出的
        ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    private fun stopAsync() {
        Thread {
            Mobile.call("Stop", "[]")
            shutdown() // sing-box 未在运行时 Stop 不会回调 closeTun
        }.start()
    }

    private fun openAppIntent(): PendingIntent = PendingIntent.getActivity(
        this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE,
    )

    private fun notification(): Notification {
        if (Build.VERSION.SDK_INT >= 26) {
            getSystemService(NotificationManager::class.java).createNotificationChannel(
                NotificationChannel(CHANNEL_ID, "VPN 状态", NotificationManager.IMPORTANCE_LOW),
            )
        }
        val stop = PendingIntent.getService(
            this, 1, Intent(this, SbtunVpnService::class.java).setAction(ACTION_STOP), PendingIntent.FLAG_IMMUTABLE,
        )
        return NotificationCompat.Builder(this, CHANNEL_ID)
            .setSmallIcon(R.drawable.ic_stat_sbtun)
            .setContentTitle("sbtun 已连接")
            .setOngoing(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .setContentIntent(openAppIntent())
            .addAction(0, "断开", stop)
            .build()
    }
}
