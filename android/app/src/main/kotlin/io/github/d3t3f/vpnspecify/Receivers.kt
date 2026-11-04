package io.github.d3t3f.vpnspecify

import android.content.BroadcastReceiver
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.net.VpnService
import android.os.Build
import android.widget.Toast

/** Reconnects when the phone starts (if chosen) and after the app is updated. */
class BootReceiver : BroadcastReceiver() {
    override fun onReceive(ctx: Context, intent: Intent) {
        val store = Store.of(ctx)
        val wanted = when (intent.action) {
            Intent.ACTION_BOOT_COMPLETED -> store.connectOnBoot
            Intent.ACTION_MY_PACKAGE_REPLACED -> store.wantOn
            else -> false
        }
        if (!wanted || store.active == null || store.apps.isEmpty()) return
        if (VpnService.prepare(ctx) != null) return // no permission: only the app can ask
        TunnelService.start(ctx)
    }
}

/** Receives what PackageInstaller says about the update being installed. */
class InstallReceiver : BroadcastReceiver() {
    override fun onReceive(ctx: Context, intent: Intent) {
        when (intent.getIntExtra(PackageInstaller.EXTRA_STATUS, PackageInstaller.STATUS_FAILURE)) {
            PackageInstaller.STATUS_PENDING_USER_ACTION -> {
                val confirm = if (Build.VERSION.SDK_INT >= 33) {
                    intent.getParcelableExtra(Intent.EXTRA_INTENT, Intent::class.java)
                } else {
                    @Suppress("DEPRECATION") intent.getParcelableExtra(Intent.EXTRA_INTENT)
                }
                confirm?.let { ctx.startActivity(it.addFlags(Intent.FLAG_ACTIVITY_NEW_TASK)) }
            }
            PackageInstaller.STATUS_SUCCESS -> Unit // the process is replaced now
            else -> Toast.makeText(
                ctx,
                T("The update failed: {0}", intent.getStringExtra(PackageInstaller.EXTRA_STATUS_MESSAGE) ?: ""),
                Toast.LENGTH_LONG,
            ).show()
        }
    }
}
