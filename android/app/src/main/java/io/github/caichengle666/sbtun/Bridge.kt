package io.github.caichengle666.sbtun

import android.content.ClipData
import android.content.ClipboardManager
import android.content.Intent
import android.net.Uri
import android.webkit.JavascriptInterface
import android.webkit.WebView
import java.util.concurrent.Executors
import mobile.Mobile
import org.json.JSONArray
import org.json.JSONObject

// 页面通过 SbtunNative.call 调用 Go 后端或少量系统能力，结果经 window.__sbtunResolve 回传。
class Bridge(private val activity: MainActivity, private val web: WebView) {
    private val pool = Executors.newCachedThreadPool()
    @Volatile private var closed = false

    // id 由页面生成（带页面加载标记），原样回传，页面重载后旧结果不会投给新请求。
    @JavascriptInterface
    fun call(id: String, kind: String, name: String, argsJson: String) {
        pool.execute {
            val result = try {
                dispatch(kind, name, argsJson)
            } catch (e: Exception) {
                fail(e.message ?: e.toString())
            }
            if (!closed) web.post { if (!closed) web.evaluateJavascript("window.__sbtunResolve(${js(id)}, ${js(result)})", null) }
        }
    }

    // WebView 销毁时调用：不再回传结果，空闲线程随即退出。
    fun close() {
        closed = true
        pool.shutdown()
    }

    // JSONObject.quote 不转义 U+2028/U+2029，旧 WebView（Chrome 66 以下）会当成换行而语法错误。
    private fun js(s: String) = JSONObject.quote(s).replace("\u2028", "\\u2028").replace("\u2029", "\\u2029")

    @JavascriptInterface
    fun setTheme(dark: Boolean) = activity.runOnUiThread { activity.applyTheme(dark) }

    private fun dispatch(kind: String, name: String, argsJson: String): String = when (kind) {
        "app" ->
            if (name == "Start" && !activity.ensureVpnPermission()) fail("需要允许 VPN 权限才能连接")
            else Mobile.call(name, argsJson)
        "runtime" -> runtime(name, JSONArray(argsJson))
        else -> fail("未知调用：$kind")
    }

    private fun runtime(name: String, args: JSONArray): String {
        when (name) {
            "ClipboardSetText" -> activity.getSystemService(ClipboardManager::class.java)
                .setPrimaryClip(ClipData.newPlainText("sbtun", args.getString(0)))
            "ScanQRCode" -> return JSONObject().put("result", activity.scanQRCode()).toString()
            "BrowserOpenURL" -> try {
                activity.startActivity(Intent(Intent.ACTION_VIEW, Uri.parse(args.getString(0))))
            } catch (e: Exception) {
                return fail("没有可以打开链接的应用")
            }
            else -> return fail("不支持的调用：$name")
        }
        return "{}"
    }

    private fun fail(message: String) = JSONObject().put("error", message).toString()
}
