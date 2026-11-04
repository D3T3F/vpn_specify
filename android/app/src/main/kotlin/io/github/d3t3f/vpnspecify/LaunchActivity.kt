package io.github.d3t3f.vpnspecify

import android.app.Activity
import android.content.Context
import android.content.Intent
import android.net.VpnService
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.view.Gravity
import android.view.View
import android.widget.Button
import android.widget.LinearLayout
import android.widget.ProgressBar
import android.widget.TextView

/**
 * What a "Discord (VPN)" shortcut opens: connects the VPN with that app in it,
 * then opens the app. The app only opens once the tunnel exists, so it never
 * gets a moment outside the VPN.
 */
class LaunchActivity : Activity() {
    private val handler = Handler(Looper.getMainLooper())
    private lateinit var message: TextView
    private lateinit var spinner: ProgressBar
    private lateinit var close: Button
    private lateinit var pkg: String
    private var started = 0L
    private val onChange: () -> Unit = { check() }

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        window.decorView.layoutDirection = if (I18n.rtl) View.LAYOUT_DIRECTION_RTL else View.LAYOUT_DIRECTION_LTR
        val pad = (20 * resources.displayMetrics.density).toInt()
        val row = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            gravity = Gravity.CENTER_HORIZONTAL
            setPadding(pad, pad, pad, pad)
        }
        spinner = ProgressBar(this)
        message = TextView(this).apply {
            setPadding(0, pad / 2, 0, pad / 2)
            gravity = Gravity.CENTER
            textSize = 15f
        }
        close = Button(this).apply {
            text = T("Close")
            visibility = View.GONE
            setOnClickListener { finish() }
        }
        row.addView(spinner)
        row.addView(message)
        row.addView(close)
        setContentView(row)

        pkg = intent.getStringExtra(EXTRA_PACKAGE).orEmpty()
        val store = Store.of(this)
        if (packageManager.getLaunchIntentForPackage(pkg) == null) return stop(T("This app is no longer installed."))
        val active = store.active ?: return stop(T("Import a VPN first."))
        if (store.isOpenVpn(active) && store.login(active) == null &&
            runCatching { Native.planOpenVpn(store.read(active)).needsAuth }.getOrDefault(false)
        ) {
            return stop(T("This VPN asks for a user and password. Open the app and connect."))
        }
        store.apps = store.apps + pkg
        message.text = T("Connecting…")

        val consent = VpnService.prepare(this)
        if (consent != null) startActivityForResult(consent, REQUEST_VPN) else connect()
    }

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        @Suppress("DEPRECATION") super.onActivityResult(requestCode, resultCode, data)
        if (requestCode != REQUEST_VPN) return
        if (resultCode == RESULT_OK) connect() else stop(T("VPN permission is missing. Open the app and connect once."))
    }

    private fun connect() {
        started = System.currentTimeMillis()
        VpnState.listen(onChange)
        if (!VpnState.tunUp || !VpnState.runningWith(Store.of(this).apps)) {
            VpnState.set(VpnState.Phase.CONNECTING, T("Connecting…"), tunUp = false, apps = emptySet())
            TunnelService.start(this)
        }
        handler.postDelayed(tick, 500)
        check()
    }

    private val tick = object : Runnable {
        override fun run() {
            check()
            handler.postDelayed(this, 500)
        }
    }

    private fun check() {
        if (isFinishing) return
        when {
            VpnState.phase == VpnState.Phase.ERROR -> stop(VpnState.detail)
            VpnState.phase == VpnState.Phase.CONNECTED && VpnState.tunUp -> open()
            // No answer yet, but its traffic is already held inside the tunnel.
            VpnState.tunUp && System.currentTimeMillis() - started > WAIT_MS -> open()
            VpnState.detail.isNotEmpty() -> message.text = VpnState.detail
        }
    }

    private fun open() {
        cleanup()
        packageManager.getLaunchIntentForPackage(pkg)?.let {
            startActivity(it.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK))
        }
        finish()
    }

    private fun stop(text: String) {
        cleanup()
        spinner.visibility = View.GONE
        message.text = text
        close.visibility = View.VISIBLE
    }

    private fun cleanup() {
        handler.removeCallbacks(tick)
        VpnState.unlisten(onChange)
    }

    override fun onDestroy() {
        cleanup()
        super.onDestroy()
    }

    companion object {
        private const val EXTRA_PACKAGE = "package"
        private const val REQUEST_VPN = 1
        private const val WAIT_MS = 8000L

        fun intent(ctx: Context, pkg: String): Intent =
            Intent(ctx, LaunchActivity::class.java).setAction(Intent.ACTION_VIEW).putExtra(EXTRA_PACKAGE, pkg)
    }
}
