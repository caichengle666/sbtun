package io.github.caichengle666.sbtun

import android.Manifest
import android.annotation.SuppressLint
import android.app.AlertDialog
import android.content.Intent
import android.content.pm.PackageManager
import android.graphics.Color
import android.net.VpnService
import android.os.Build
import android.os.Bundle
import android.webkit.JsResult
import android.webkit.WebChromeClient
import android.webkit.WebResourceRequest
import android.webkit.WebResourceResponse
import android.webkit.WebView
import android.webkit.WebViewClient
import androidx.activity.ComponentActivity
import androidx.activity.addCallback
import androidx.activity.result.contract.ActivityResultContracts
import androidx.core.content.ContextCompat
import androidx.core.view.ViewCompat
import androidx.core.view.WindowCompat
import androidx.core.view.WindowInsetsCompat
import androidx.webkit.WebViewAssetLoader
import java.util.concurrent.CompletableFuture

class MainActivity : ComponentActivity() {
    private lateinit var web: WebView
    private var safeArea = "" // 最近一次计算出的安全区 JS，页面加载完后补发
    private var vpnConsent: CompletableFuture<Boolean>? = null

    private val vpnPermission = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) {
        vpnConsent?.complete(it.resultCode == RESULT_OK)
    }
    private val notificationPermission = registerForActivityResult(ActivityResultContracts.RequestPermission()) {}

    @SuppressLint("SetJavaScriptEnabled")
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        WindowCompat.setDecorFitsSystemWindows(window, false)

        // 页面固定在 /，资源路径都是绝对的 /assets/...，统一映射到 APK 的 assets/web/。
        val assets = WebViewAssetLoader.Builder()
            .addPathHandler("/", WebViewAssetLoader.AssetsPathHandler(this).let { h -> WebViewAssetLoader.PathHandler { h.handle("web/$it") } })
            .build()

        web = WebView(this).apply {
            setBackgroundColor(DARK_BG)
            settings.javaScriptEnabled = true
            settings.domStorageEnabled = true
            settings.setSupportZoom(false)
            settings.builtInZoomControls = false
            settings.textZoom = 100
            addJavascriptInterface(Bridge(this@MainActivity, this), "SbtunNative")
            webViewClient = object : WebViewClient() {
                override fun shouldInterceptRequest(view: WebView, request: WebResourceRequest): WebResourceResponse? =
                    assets.shouldInterceptRequest(request.url)

                override fun onPageFinished(view: WebView, url: String?) {
                    if (safeArea.isNotEmpty()) view.evaluateJavascript(safeArea, null)
                }

                // 页面之外的链接交给系统浏览器，不在应用里跳走。
                override fun shouldOverrideUrlLoading(view: WebView, request: WebResourceRequest): Boolean {
                    if (request.url.host == WebViewAssetLoader.DEFAULT_DOMAIN) return false
                    runCatching { startActivity(Intent(Intent.ACTION_VIEW, request.url)) }
                    return true
                }
            }
            // WebView 默认不弹 alert/confirm（confirm 直接返回 false），这里用系统对话框实现。
            webChromeClient = object : WebChromeClient() {
                override fun onJsAlert(view: WebView, url: String?, message: String?, result: JsResult): Boolean =
                    showDialog(message, result, cancel = false)

                override fun onJsConfirm(view: WebView, url: String?, message: String?, result: JsResult): Boolean =
                    showDialog(message, result, cancel = true)
            }
        }
        setContentView(web)

        ViewCompat.setOnApplyWindowInsetsListener(web) { view, insets ->
            val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout())
            val ime = insets.getInsets(WindowInsetsCompat.Type.ime())
            val density = resources.displayMetrics.density
            fun css(name: String, px: Int) = "s.setProperty('--safe-$name','${(px / density).toInt()}px');"
            safeArea = "(s=>{" + css("top", bars.top) + css("bottom", maxOf(bars.bottom, ime.bottom)) +
                css("left", bars.left) + css("right", bars.right) + "})(document.documentElement.style)"
            (view as WebView).evaluateJavascript(safeArea, null)
            WindowInsetsCompat.CONSUMED
        }

        onBackPressedDispatcher.addCallback(this) {
            web.evaluateJavascript("window.__sbtunBack ? window.__sbtunBack() : false") { consumed ->
                if (consumed != "true") moveTaskToBack(true)
            }
        }

        applyTheme(true)
        if (Build.VERSION.SDK_INT >= 33 &&
            ContextCompat.checkSelfPermission(this, Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED
        ) {
            notificationPermission.launch(Manifest.permission.POST_NOTIFICATIONS)
        }
        web.loadUrl("https://${WebViewAssetLoader.DEFAULT_DOMAIN}/index.html")
    }

    override fun onDestroy() {
        vpnConsent?.complete(false) // 不让等待授权的后台线程永远卡住
        web.destroy()
        super.onDestroy()
    }

    // 系统栏图标深浅和窗口底色跟随页面主题。
    fun applyTheme(dark: Boolean) {
        val bg = if (dark) DARK_BG else LIGHT_BG
        window.decorView.setBackgroundColor(bg)
        web.setBackgroundColor(bg)
        WindowCompat.getInsetsController(window, window.decorView).apply {
            isAppearanceLightStatusBars = !dark
            isAppearanceLightNavigationBars = !dark
        }
    }

    private fun showDialog(message: String?, result: JsResult, cancel: Boolean): Boolean {
        AlertDialog.Builder(this)
            .setMessage(message)
            .setPositiveButton("确定") { _, _ -> result.confirm() }
            .apply { if (cancel) setNegativeButton("取消") { _, _ -> result.cancel() } }
            .setOnCancelListener { if (cancel) result.cancel() else result.confirm() }
            .show()
        return true
    }

    // 在后台线程调用：需要时弹出系统 VPN 授权框并等待用户选择。
    fun ensureVpnPermission(): Boolean {
        val intent = VpnService.prepare(this) ?: return true
        val consent = CompletableFuture<Boolean>()
        runOnUiThread {
            vpnConsent = consent
            try {
                vpnPermission.launch(intent)
            } catch (e: Exception) {
                consent.complete(false)
            }
        }
        return consent.get()
    }

    private companion object {
        val DARK_BG = Color.parseColor("#111617")
        val LIGHT_BG = Color.parseColor("#f2ede1")
    }
}
