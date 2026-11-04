package io.github.d3t3f.vpnspecify

import android.app.Notification
import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ServiceInfo
import android.net.ConnectivityManager
import android.net.LinkProperties
import android.net.Network
import android.net.NetworkCapabilities
import android.net.NetworkRequest
import android.net.VpnService
import android.os.Build
import android.os.Handler
import android.os.Looper
import android.util.Log
import java.util.concurrent.Executors
import kotlin.concurrent.thread

/**
 * The VPN. Android does what the desktop versions need a proxy for: only the
 * apps passed to addAllowedApplication use the tunnel, with all their TCP and
 * UDP, and this app's own WireGuard socket stays outside it.
 */
class TunnelService : VpnService() {
    // Starts and stops. While connected, monitor() keeps this thread busy.
    private val worker = Executors.newSingleThreadExecutor()
    // Network changes: they must not wait for the worker, which only frees up
    // when the tunnel goes down.
    private val networkWorker = Executors.newSingleThreadExecutor()
    private val main = Handler(Looper.getMainLooper())
    private val store by lazy { Store.of(this) }

    // Each start or stop gets a new number; older work sees it and gives up.
    @Volatile private var generation = 0
    private var network: ConnectivityManager.NetworkCallback? = null
    @Volatile private var openVpn: OpenVpnProcess? = null

    override fun onStartCommand(intent: Intent?, flags: Int, startId: Int): Int {
        // No intent: the system restarted the service. An always-on start has
        // the VpnService action. Neither is a user turning the VPN on from here.
        if (intent?.action == ACTION_STOP) {
            store.wantOn = false
            shutdown()
            return START_NOT_STICKY
        }
        if (intent == null && !store.wantOn) {
            stopSelf()
            return START_NOT_STICKY
        }
        if (!enterForeground()) {
            VpnState.set(VpnState.Phase.ERROR, T("VPN permission is missing. Open the app and connect once."), tunUp = false)
            stopSelf()
            return START_NOT_STICKY
        }
        store.wantOn = true
        val g = ++generation
        worker.execute { run(g) }
        return START_STICKY
    }

    private fun run(g: Int) {
        val config = store.active
        stopBackends()
        val text = try {
            if (config == null) throw Native.GoError(T("Import a VPN first."))
            store.read(config)
        } catch (e: Exception) {
            return fail(g, e.message ?: e.toString())
        }

        VpnState.set(VpnState.Phase.CONNECTING, T("Connecting…"), config = config, tunUp = false, apps = emptySet())
        updateNotification()

        if (config != null && store.isOpenVpn(config)) return runOpenVpn(g, config, text)

        try {
            val plan = Native.plan(text)
            // The server name resolves through the normal network, which may
            // not exist yet (right after the phone starts, for example).
            while (true) {
                if (g != generation) return
                try {
                    Native.resolve(text)
                    break
                } catch (e: Native.GoError) {
                    VpnState.set(VpnState.Phase.CONNECTING, T("Waiting for the network…"))
                    Thread.sleep(3000)
                }
            }
            if (g != generation) return

            val b = Builder().setSession(config!!).setMtu(plan.mtu).setBlocking(true)
            for (a in plan.addresses) {
                val (ip, bits) = a.split("/")
                b.addAddress(ip, bits.toInt())
            }
            for (r in plan.routes) {
                val (ip, bits) = r.split("/")
                b.addRoute(ip, bits.toInt())
            }
            routeEverything(b, plan.addresses.map { it.substringBefore('/') })
            plan.dns.forEach { b.addDnsServer(it) }
            if (Build.VERSION.SDK_INT >= 29) b.setMetered(false)

            val used = allowApps(b)
            if (used.isEmpty()) return fail(g, T("Choose at least one app first."))

            val pfd = b.establish() ?: return fail(g, T("VPN permission is missing. Open the app and connect once."))
            if (g != generation) {
                pfd.close()
                return
            }
            Native.start(pfd.detachFd(), text)
            VpnState.set(VpnState.Phase.CONNECTING, T("Waiting for the server…"), tunUp = true, apps = used)
            watchNetwork()
            monitor(g, System.currentTimeMillis())
        } catch (e: InterruptedException) {
            return
        } catch (e: Exception) {
            Log.w(TAG, "start failed", e)
            fail(g, e.message ?: e.toString())
        }
    }

    /**
     * A chosen app must never leave outside the VPN, whatever routes the
     * configuration or the server name, as in the desktop versions. A destination
     * the VPN cannot reach fails there instead of quietly using the normal network.
     * A family without an address stays blocked: Android does that by itself.
     */
    private fun routeEverything(b: Builder, addresses: List<String>) {
        if (addresses.any { ':' !in it }) b.addRoute("0.0.0.0", 0)
        if (addresses.any { ':' in it }) b.addRoute("::", 0)
    }

    /**
     * Never establish with an empty list: for Android that means every app on
     * the phone, the opposite of what this app promises.
     */
    private fun allowApps(b: Builder): Set<String> {
        val used = mutableSetOf<String>()
        for (pkg in store.apps) {
            if (pkg == packageName) continue
            try {
                b.addAllowedApplication(pkg)
                used += pkg
            } catch (e: PackageManager.NameNotFoundException) {
                Log.i(TAG, "skipping $pkg: not installed")
            }
        }
        return used
    }

    private fun stopBackends() {
        runCatching { Native.stop() }
        openVpn?.stop()
    }

    /** Blocks on this worker thread until openvpn exits, like monitor() does. */
    private fun runOpenVpn(g: Int, config: String, text: String) {
        val plan = try {
            Native.planOpenVpn(text)
        } catch (e: Native.GoError) {
            return fail(g, e.message ?: e.toString())
        }
        val login = if (plan.needsAuth) {
            store.login(config) ?: return fail(g, T("This VPN asks for a user and password. Open the app and connect."))
        } else {
            null
        }
        var buildFailure: String? = null
        val started = System.currentTimeMillis()
        val process = OpenVpnProcess(
            nativeDir = applicationInfo.nativeLibraryDir,
            runDir = java.io.File(filesDir, "openvpn-run"),
            config = plan.config,
            user = login?.first,
            password = login?.second,
            openTun = { settings ->
                val b = Builder().setSession(config).setMtu(settings.mtu)
                settings.addresses.forEach { (ip, bits) -> b.addAddress(ip, bits) }
                settings.routes.forEach { (ip, bits) -> b.addRoute(ip, bits) }
                routeEverything(b, settings.addresses.map { it.first })
                settings.dns.forEach { b.addDnsServer(it) }
                settings.domains.forEach { b.addSearchDomain(it) }
                if (Build.VERSION.SDK_INT >= 29) b.setMetered(false)
                val used = allowApps(b)
                if (used.isEmpty()) {
                    buildFailure = T("Choose at least one app first.")
                    openVpn?.stop()
                    null
                } else {
                    b.establish().also {
                        if (it == null) {
                            buildFailure = T("VPN permission is missing. Open the app and connect once.")
                            openVpn?.stop()
                        } else {
                            VpnState.set(VpnState.Phase.CONNECTING, T("Waiting for the server…"), tunUp = true, apps = used)
                        }
                    }
                }
            },
            onState = { state, _ ->
                if (g == generation) {
                    when (state) {
                        OpenVpnProcess.State.CONNECTED -> VpnState.set(VpnState.Phase.CONNECTED)
                        OpenVpnProcess.State.CONNECTING -> VpnState.set(
                            VpnState.Phase.CONNECTING,
                            if (System.currentTimeMillis() - started > NO_ANSWER_MS) {
                                T("No answer from the server yet. Check the configuration and the network.")
                            } else {
                                T("Connecting…")
                            },
                        )
                    }
                    updateNotification()
                }
            },
            onBytes = { rx, tx ->
                VpnState.stats = Native.Stats(rx, tx, 0)
                VpnState.notifyListeners()
            },
            onLog = { line -> runCatching { Native.appendLog("[openvpn] $line") } },
        )
        openVpn = process
        watchNetwork()
        // openvpn keeps retrying quietly when nothing answers, so no state change
        // would ever tell the user that.
        val watcher = thread(name = "openvpn-watcher") {
            try {
                while (g == generation && openVpn === process) {
                    Thread.sleep(2000)
                    if (g == generation && VpnState.phase == VpnState.Phase.CONNECTING &&
                        System.currentTimeMillis() - started > NO_ANSWER_MS
                    ) {
                        VpnState.set(VpnState.Phase.CONNECTING, T("No answer from the server yet. Check the configuration and the network."))
                    }
                }
            } catch (e: InterruptedException) {
                return@thread
            }
        }
        try {
            process.run()
        } catch (e: Exception) {
            Log.w(TAG, "openvpn", e)
            buildFailure = buildFailure ?: (e.message ?: e.toString())
        }
        if (openVpn === process) openVpn = null
        watcher.interrupt()
        if (g != generation) return
        when (process.failure) {
            OpenVpnProcess.Failure.AUTH -> {
                store.forgetPassword(config)
                fail(g, T("The server refused the user or password."))
            }
            OpenVpnProcess.Failure.KEY_PASSWORD ->
                fail(g, T("This VPN's private key has a password, which Android does not support yet."))
            null -> fail(g, buildFailure ?: process.lastError ?: T("OpenVPN stopped. See the log for the reason."))
        }
    }

    /** Follows the handshake and the counters until a newer start or stop. */
    private fun monitor(g: Int, started: Long) {
        var lastKick = started
        var shown: VpnState.Phase? = null
        // The peer answers data within seconds, so data going out with nothing
        // coming back means the connection is gone, whatever the last handshake.
        var lastRx = 0L
        var lastRxAt = started
        var txAtLastRx = 0L
        while (g == generation) {
            try {
                val st = Native.stats()
                VpnState.stats = st
                val now = System.currentTimeMillis()
                if (st.rx != lastRx) {
                    lastRx = st.rx
                    lastRxAt = now
                    txAtLastRx = st.tx
                }
                val silent = st.tx > txAtLastRx && now - lastRxAt > SILENT_MS
                if (st.lastHandshake > 0 && !silent) {
                    if (VpnState.phase != VpnState.Phase.CONNECTED) VpnState.set(VpnState.Phase.CONNECTED)
                    else VpnState.notifyListeners()
                } else {
                    if (now - lastKick > KICK_MS) {
                        lastKick = now
                        Native.kick()
                    }
                    if (silent || now - started > NO_ANSWER_MS) {
                        VpnState.set(VpnState.Phase.CONNECTING, T("No answer from the server yet. Check the configuration and the network."))
                    }
                }
                if (VpnState.phase != shown) {
                    shown = VpnState.phase
                    updateNotification()
                }
                Thread.sleep(1000)
            } catch (e: InterruptedException) {
                return
            } catch (e: Exception) {
                Log.w(TAG, "monitor", e)
                return
            }
        }
    }

    private fun fail(g: Int, message: String) {
        if (g != generation) return
        generation++
        store.wantOn = false
        stopBackends()
        VpnState.set(VpnState.Phase.ERROR, message, tunUp = false, apps = emptySet())
        main.post { leave() }
    }

    private fun shutdown() {
        generation++
        stopBackends()
        worker.execute {
            VpnState.set(VpnState.Phase.OFF, tunUp = false, apps = emptySet(), config = null)
        }
        main.post { leave() }
    }

    private fun leave() {
        stopWatchingNetwork()
        stopForeground(STOP_FOREGROUND_REMOVE)
        stopSelf()
    }

    /**
     * Sockets opened on the old network do not move by themselves. This cannot
     * use registerDefaultNetworkCallback: for the app that created a VPN, that
     * reports the VPN itself, so Wi-Fi and mobile data changes would go unseen.
     * Any change is enough reason to rebind, and doing it needlessly is harmless;
     * the delay turns the burst of events of one change into one rebind.
     */
    private fun watchNetwork() {
        main.post {
            if (network != null) return@post
            val validated = HashMap<Network, Boolean>()
            val registered = System.currentTimeMillis()
            val rebind = Runnable {
                networkWorker.execute {
                    val r = runCatching {
                        Native.rebind()
                        openVpn?.networkChanged()
                    }
                    Log.i(TAG, "rebind: ${r.exceptionOrNull() ?: "ok"}")
                }
            }
            fun changed(why: String, n: Network) {
                // Registering reports the networks that already exist.
                if (System.currentTimeMillis() - registered < SETTLE_MS) return
                Log.i(TAG, "network $n: $why")
                main.removeCallbacks(rebind)
                main.postDelayed(rebind, REBIND_DELAY_MS)
            }
            val cb = object : ConnectivityManager.NetworkCallback() {
                override fun onAvailable(n: Network) = changed("available", n)
                override fun onLost(n: Network) {
                    validated.remove(n)
                    changed("lost", n)
                }
                override fun onLinkPropertiesChanged(n: Network, lp: LinkProperties) = changed("addresses", n)
                // A network usually becomes the default only after being validated.
                override fun onCapabilitiesChanged(n: Network, caps: NetworkCapabilities) {
                    val v = caps.hasCapability(NetworkCapabilities.NET_CAPABILITY_VALIDATED)
                    if (validated.put(n, v) != v) changed("validated=$v", n)
                }
            }
            network = cb
            val request = NetworkRequest.Builder()
                .addCapability(NetworkCapabilities.NET_CAPABILITY_INTERNET)
                .addCapability(NetworkCapabilities.NET_CAPABILITY_NOT_VPN)
                .build()
            // Callbacks arrive on the main thread, like the rest of this code.
            getSystemService(ConnectivityManager::class.java).registerNetworkCallback(request, cb, main)
        }
    }

    private fun stopWatchingNetwork() {
        network?.let { getSystemService(ConnectivityManager::class.java).unregisterNetworkCallback(it) }
        network = null
    }

    private fun notification(): Notification {
        val open = PendingIntent.getActivity(
            this, 0, Intent(this, MainActivity::class.java), PendingIntent.FLAG_IMMUTABLE,
        )
        val stop = PendingIntent.getService(
            this, 1, Intent(this, TunnelService::class.java).setAction(ACTION_STOP), PendingIntent.FLAG_IMMUTABLE,
        )
        val text = when (VpnState.phase) {
            VpnState.Phase.CONNECTED -> T("Connected: {0}", VpnState.config ?: "")
            else -> T("Connecting…")
        }
        return Notification.Builder(this, App.CHANNEL)
            .setSmallIcon(R.drawable.ic_notification)
            .setContentTitle(getString(R.string.app_name))
            .setContentText(text)
            .setContentIntent(open)
            .setOngoing(true)
            .addAction(Notification.Action.Builder(null, T("Disconnect"), stop).build())
            .build()
    }

    private fun updateNotification() {
        main.post {
            if (generation == 0) return@post
            getSystemService(android.app.NotificationManager::class.java).notify(NOTIFICATION, notification())
        }
    }

    private fun enterForeground(): Boolean = try {
        if (Build.VERSION.SDK_INT >= 34) {
            startForeground(NOTIFICATION, notification(), ServiceInfo.FOREGROUND_SERVICE_TYPE_SYSTEM_EXEMPTED)
        } else {
            startForeground(NOTIFICATION, notification())
        }
        true
    } catch (e: Exception) {
        // Android 14+ only lets a prepared VPN app use this foreground type.
        Log.w(TAG, "startForeground", e)
        false
    }

    /** Another VPN app took over, or the user removed the permission. */
    override fun onRevoke() {
        store.wantOn = false
        generation++
        stopBackends()
        VpnState.set(VpnState.Phase.ERROR, T("Android turned this VPN off: another VPN was started, or the permission was removed."), tunUp = false, apps = emptySet())
        main.post { leave() }
    }

    override fun onDestroy() {
        generation++
        stopWatchingNetwork()
        stopBackends()
        worker.shutdown()
        networkWorker.shutdown()
        if (VpnState.phase != VpnState.Phase.ERROR) VpnState.set(VpnState.Phase.OFF, tunUp = false, apps = emptySet(), config = null)
        super.onDestroy()
    }

    companion object {
        private const val TAG = "vpn_specify"
        private const val NOTIFICATION = 1
        private const val ACTION_STOP = "io.github.d3t3f.vpnspecify.STOP"
        private const val KICK_MS = 10_000L
        private const val NO_ANSWER_MS = 20_000L
        private const val SILENT_MS = 30_000L
        private const val SETTLE_MS = 1500L
        private const val REBIND_DELAY_MS = 1000L

        /** The caller makes sure VpnService.prepare() returned null. */
        fun start(ctx: Context) {
            ctx.startForegroundService(Intent(ctx, TunnelService::class.java))
        }

        fun stop(ctx: Context) {
            ctx.startService(Intent(ctx, TunnelService::class.java).setAction(ACTION_STOP))
        }
    }
}
