package io.github.caichengle666.sbtun

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import android.app.PendingIntent
import android.content.Context
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
        // 应用自己（VpnPlatform）拉起服务时带上请求编号；系统拉起时没有。
        private const val EXTRA_REQUEST = "io.github.caichengle666.sbtun.REQUEST"
        private const val NOTIFICATION_ID = 1
        private const val CHANNEL_ID = "vpn"

        @Volatile
        var current: SbtunVpnService? = null
            private set
        private var ready = CompletableFuture<SbtunVpnService>()
        private var lastRequest = 0
        private var wanted = 0 // 正在等待的请求编号，0 表示没有

        // 登记一次启动请求，返回拉起服务用的 intent 和等服务就绪的 future。
        @Synchronized
        fun request(context: Context): Pair<Intent, CompletableFuture<SbtunVpnService>> {
            wanted = ++lastRequest
            return Intent(context, SbtunVpnService::class.java).putExtra(EXTRA_REQUEST, wanted) to ready
        }

        // 请求失败或超时：作废编号（服务稍后才起来时自己退出），已起来的服务直接收掉。
        @Synchronized
        fun abandon(intent: Intent) {
            if (wanted == intent.getIntExtra(EXTRA_REQUEST, 0)) wanted = 0
            current?.shutdown()
        }

        // 系统拉起（id 为 0）总是接上；应用的请求只接仍在等待的那个。
        @Synchronized
        private fun attach(service: SbtunVpnService, id: Int): Boolean {
            if (id != 0 && id != wanted) return false
            current = service
            ready.complete(service)
            return true
        }

        @Synchronized
        private fun detach(service: SbtunVpnService) {
            if (current !== service) return
            current = null
            ready = CompletableFuture()
        }
    }

    @Volatile private var established = false

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        if (intent?.action == ACTION_STOP) {
            stopAsync()
            return START_NOT_STICKY
        }
        val id = intent?.getIntExtra(EXTRA_REQUEST, 0) ?: 0
        val running = current === this
        val attached = running || attach(this, id)
        enterForeground() // 每次 startForegroundService 都要及时进入前台，作废的请求也一样
        when {
            !attached -> { // 应用已放弃这次请求（超时）；只在没有更新的请求时退出
                ServiceCompat.stopForeground(this, ServiceCompat.STOP_FOREGROUND_REMOVE)
                stopSelfResult(startId)
            }
            // 系统拉起（始终开启的 VPN、在系统设置里点 sbtun）：自动连接，没连上就退出。
            id == 0 && !running -> Thread {
                Mobile.call("Start", "[]")
                if (current === this && !established) shutdown()
            }.start()
        }
        return START_NOT_STICKY
    }

    private fun enterForeground() {
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTIFICATION_ID, notification(), ServiceInfo.FOREGROUND_SERVICE_TYPE_SPECIAL_USE)
        } else {
            startForeground(NOTIFICATION_ID, notification())
        }
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
        // 不开 IPv6 时不加任何 IPv6 地址/路由/DNS：系统默认把 IPv6 流量拦掉（不可达路由，立即返回
        // ENETUNREACH），应用马上回落 IPv4，也不会绕过 VPN。别为此把 ::/0 收进隧道，那样 UDP 会黑洞到超时。
        if (ipv6) builder.addAddress("fdfe:dcba:9876::1", 126).addRoute("::", 0)
        if (Build.VERSION.SDK_INT >= 29) builder.setMetered(false)
        val tun = builder.establish() ?: throw Exception("无法建立 VPN，请确认已允许 VPN 权限")
        established = true
        getSystemService(NotificationManager::class.java).notify(NOTIFICATION_ID, notification())
        return tun.detachFd()
    }

    fun shutdown() {
        established = false
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
            .setContentTitle(if (established) "sbtun 已连接" else "sbtun 正在连接…")
            .setOngoing(true)
            .setCategory(NotificationCompat.CATEGORY_SERVICE)
            .setForegroundServiceBehavior(NotificationCompat.FOREGROUND_SERVICE_IMMEDIATE)
            .setContentIntent(openAppIntent())
            .addAction(0, "断开", stop)
            .build()
    }
}
