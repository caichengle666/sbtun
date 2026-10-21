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
import com.journeyapps.barcodescanner.ScanContract
import com.journeyapps.barcodescanner.ScanOptions
import java.util.concurrent.CompletableFuture

class MainActivity : ComponentActivity() {
    private lateinit var web: WebView
    private var safeArea = "" // 最近一次计算出的安全区 JS，页面加载完后补发
    private var lastInsets: WindowInsetsCompat? = null
    private var vpnConsent: CompletableFuture<Boolean>? = null
    private var qrResult: CompletableFuture<String>? = null

    private val vpnPermission = registerForActivityResult(ActivityResultContracts.StartActivityForResult()) {
        vpnConsent?.complete(it.resultCode == RESULT_OK)
    }
    private val qrScanner = registerForActivityResult(ScanContract()) { qrResult?.complete(it.contents.orEmpty()) }
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

        ViewCompat.setOnApplyWindowInsetsListener(web) { _, insets ->
            lastInsets = insets
            pushSafeArea()
            WindowInsetsCompat.CONSUMED
        }
        // 有的系统（如 MIUI）键盘弹出时会把 WebView 缩短，有的不会；布局变化后按实际遮挡重新计算。
        web.addOnLayoutChangeListener { _, _, top, _, bottom, _, oldTop, _, oldBottom ->
            if (bottom - top != oldBottom - oldTop) pushSafeArea()
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

    // 把系统栏和键盘真正盖住 WebView 的部分传给页面：WebView 已被系统缩短时不再重复留空。
    private fun pushSafeArea() {
        val insets = lastInsets ?: return
        val bars = insets.getInsets(WindowInsetsCompat.Type.systemBars() or WindowInsetsCompat.Type.displayCutout())
        val ime = insets.getInsets(WindowInsetsCompat.Type.ime())
        val loc = IntArray(2).also(web::getLocationInWindow)
        val windowH = window.decorView.height
        if (windowH == 0 || web.height == 0) return
        val gap = windowH - (loc[1] + web.height) // WebView 底边到窗口底边的距离
        fun covered(px: Int) = maxOf(0, px - gap)
        val keyboard = ime.bottom > bars.bottom
        val shrunk = gap > bars.bottom + 50 // 系统还没把 WebView 恢复原高
        val density = resources.displayMetrics.density
        fun css(name: String, px: Int) = "s.setProperty('--$name','${(px / density).toInt()}px');"
        // 键盘高度单独给 --keyboard-h，并打上 data-keyboard，页面据此藏起底部标签栏，不跟着键盘重排。
        val js = "(r=>{const s=r.style;" + css("safe-top", bars.top) + css("safe-bottom", covered(bars.bottom)) +
            css("safe-left", bars.left) + css("safe-right", bars.right) + css("keyboard-h", if (keyboard) covered(ime.bottom) else 0) +
            "r.toggleAttribute('data-keyboard'," + (keyboard || shrunk) + ")})(document.documentElement)"
        if (js == safeArea) return
        safeArea = js
        web.evaluateJavascript(js, null)
    }

    override fun onDestroy() {
        vpnConsent?.complete(false) // 不让等待授权的后台线程永远卡住
        qrResult?.complete("")
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

    // 在后台线程调用：打开摄像头扫二维码，返回识别到的文字，取消时返回空串。相机权限由扫码界面自己申请。
    fun scanQRCode(): String {
        val result = CompletableFuture<String>()
        runOnUiThread {
            qrResult = result
            try {
                qrScanner.launch(
                    ScanOptions()
                        .setDesiredBarcodeFormats(ScanOptions.QR_CODE)
                        .setPrompt("对准节点或订阅二维码")
                        .setBeepEnabled(false)
                        .setOrientationLocked(false)
                )
            } catch (e: Exception) {
                result.complete("")
            }
        }
        return result.get()
    }

    private companion object {
        val DARK_BG = Color.parseColor("#111617")
        val LIGHT_BG = Color.parseColor("#f2ede1")
    }
}
