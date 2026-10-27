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

    // 把 APK 自带的规则集复制到 filesDir/rules：缺失时补齐；应用升级后覆盖，但不覆盖构建之后用户更新过的。
    // 先写临时文件再改名，复制中途被杀不会留下截断的规则文件。
    private fun syncRules() {
        val prefs = getSharedPreferences("sbtun", MODE_PRIVATE)
        val upgraded = prefs.getInt("rulesVersion", -1) != BuildConfig.VERSION_CODE
        val dir = File(filesDir, "rules").apply { mkdirs() }
        assets.list("rules").orEmpty().forEach { name ->
            val target = File(dir, name)
            if (target.exists() && !(upgraded && target.lastModified() < BuildConfig.BUILD_TIME)) return@forEach
            val tmp = File(dir, "$name.tmp")
            assets.open("rules/$name").use { input -> tmp.outputStream().use { input.copyTo(it) } }
            if (!tmp.renameTo(target)) tmp.delete()
        }
        if (upgraded) prefs.edit().putInt("rulesVersion", BuildConfig.VERSION_CODE).apply()
    }
}
