package io.github.caichengle666.sbtun

import android.app.Application
import java.io.File
import mobile.Mobile

class App : Application() {
    override fun onCreate() {
        super.onCreate()
        syncRules()
        Mobile.init(filesDir.absolutePath, applicationInfo.nativeLibraryDir + "/libsingbox.so", VpnPlatform(this))
    }

    // 把 APK 自带的规则集复制到 filesDir/rules：缺失时补齐，应用升级后整体覆盖。
    private fun syncRules() {
        val prefs = getSharedPreferences("sbtun", MODE_PRIVATE)
        val upgraded = prefs.getInt("rulesVersion", -1) != BuildConfig.VERSION_CODE
        val dir = File(filesDir, "rules").apply { mkdirs() }
        assets.list("rules").orEmpty().forEach { name ->
            val target = File(dir, name)
            if (upgraded || !target.exists()) {
                assets.open("rules/$name").use { input -> target.outputStream().use { input.copyTo(it) } }
            }
        }
        if (upgraded) prefs.edit().putInt("rulesVersion", BuildConfig.VERSION_CODE).apply()
    }
}
