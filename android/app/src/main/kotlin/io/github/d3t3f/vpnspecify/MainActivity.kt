package io.github.d3t3f.vpnspecify

import android.Manifest
import android.app.Activity
import android.app.AlertDialog
import android.app.ProgressDialog
import android.content.Intent
import android.content.pm.PackageManager
import android.content.pm.ShortcutInfo
import android.content.pm.ShortcutManager
import android.content.res.Configuration
import android.graphics.Bitmap
import android.graphics.Canvas
import android.graphics.Color
import android.graphics.Paint
import android.graphics.Typeface
import android.graphics.drawable.Drawable
import android.graphics.drawable.GradientDrawable
import android.graphics.drawable.Icon
import android.net.Uri
import android.net.VpnService
import android.os.Bundle
import android.os.Handler
import android.os.Looper
import android.provider.OpenableColumns
import android.provider.Settings
import android.text.Editable
import android.text.TextWatcher
import android.text.format.Formatter
import android.view.Gravity
import android.view.View
import android.view.ViewGroup
import android.widget.BaseAdapter
import android.widget.Button
import android.widget.CheckBox
import android.widget.EditText
import android.widget.ImageView
import android.widget.LinearLayout
import android.widget.ListView
import android.widget.ScrollView
import android.widget.Switch
import android.widget.TextView
import android.widget.Toast
import kotlin.concurrent.thread

/** The whole interface: status, VPNs, apps and settings on one screen. */
class MainActivity : Activity() {
    private val store by lazy { Store.of(this) }
    private val main = Handler(Looper.getMainLooper())

    private lateinit var content: LinearLayout
    private lateinit var dot: View
    private lateinit var stateText: TextView
    private lateinit var detailText: TextView
    private lateinit var connectButton: Button

    private val onState: () -> Unit = { showState() }
    private var checkedUpdates = false

    // Colors, the same as the desktop interface.
    private val dark by lazy {
        resources.configuration.uiMode and Configuration.UI_MODE_NIGHT_MASK == Configuration.UI_MODE_NIGHT_YES
    }
    private val bg get() = color("#F4F5F7", "#15171C")
    private val cardBg get() = color("#FFFFFF", "#1E2128")
    private val textColor get() = color("#1D2129", "#E7E9EE")
    private val muted get() = color("#677085", "#9AA2B4")
    private val line get() = color("#E2E5EB", "#2D313B")
    private val accent get() = color("#2F6FED", "#5B8CFF")
    private val accentText get() = color("#FFFFFF", "#0D1220")
    private val okColor get() = color("#1F9D55", "#3CCF7A")
    private val errColor get() = color("#D03B3B", "#FF6B6B")
    private fun color(light: String, night: String) = Color.parseColor(if (dark) night else light)
    private fun dp(n: Int) = (n * resources.displayMetrics.density).toInt()

    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        val scroll = ScrollView(this).apply {
            setBackgroundColor(bg)
            isFillViewport = true
            clipToPadding = false
        }
        content = LinearLayout(this).apply { orientation = LinearLayout.VERTICAL }
        scroll.addView(content)
        // The app draws behind the system bars; keep the content out of them.
        scroll.setOnApplyWindowInsetsListener { v, insets ->
            @Suppress("DEPRECATION")
            v.setPadding(
                insets.systemWindowInsetLeft + dp(14), insets.systemWindowInsetTop + dp(10),
                insets.systemWindowInsetRight + dp(14), insets.systemWindowInsetBottom + dp(14),
            )
            insets
        }
        setContentView(scroll)
        render()
    }

    override fun onStart() {
        super.onStart()
        VpnState.listen(onState)
        render()
        if (!checkedUpdates && store.updateCheck) {
            checkedUpdates = true
            checkForUpdate(quiet = true)
        }
    }

    override fun onStop() {
        VpnState.unlisten(onState)
        super.onStop()
    }

    // ---- building blocks ----------------------------------------------------

    private fun text(s: String, size: Float = 14f, c: Int = textColor, bold: Boolean = false) = TextView(this).apply {
        text = s
        textSize = size
        setTextColor(c)
        textAlignment = View.TEXT_ALIGNMENT_VIEW_START
        if (bold) typeface = Typeface.DEFAULT_BOLD
    }

    private fun rounded(fill: Int, stroke: Int? = null, radius: Int = 12) = GradientDrawable().apply {
        setColor(fill)
        cornerRadius = dp(radius).toFloat()
        if (stroke != null) setStroke(dp(1), stroke)
    }

    private fun button(s: String, primary: Boolean = false, click: () -> Unit) = Button(this).apply {
        text = s
        isAllCaps = false
        stateListAnimator = null
        minHeight = dp(44)
        minimumHeight = dp(44)
        setPadding(dp(14), 0, dp(14), 0)
        background = if (primary) rounded(accent, radius = 9) else rounded(cardBg, line, 9)
        setTextColor(if (primary) accentText else textColor)
        setOnClickListener { click() }
    }

    /** A small button on a row; its text keeps it from squeezing the row's name. */
    private fun rowButton(s: String, click: () -> Unit) = button(s, click = click).apply {
        minHeight = dp(36)
        minimumHeight = dp(36)
        minWidth = 0
        minimumWidth = 0
        textSize = 13f
        setPadding(dp(10), 0, dp(10), 0)
    }

    private fun lp(w: Int = ViewGroup.LayoutParams.MATCH_PARENT, h: Int = ViewGroup.LayoutParams.WRAP_CONTENT, top: Int = 0, start: Int = 0, weight: Float = 0f) =
        LinearLayout.LayoutParams(w, h, weight).apply {
            topMargin = dp(top)
            marginStart = dp(start)
        }

    private fun card(title: String? = null, block: LinearLayout.() -> Unit) {
        val c = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            background = rounded(cardBg, line)
            setPadding(dp(14), dp(12), dp(14), dp(14))
        }
        if (title != null) c.addView(text(title, 15f, bold = true))
        c.block()
        content.addView(c, lp(top = 10))
    }

    private fun LinearLayout.hint(s: String) = addView(text(s, 12.5f, muted), lp(top = 4))

    private fun row(block: LinearLayout.() -> Unit) = LinearLayout(this).apply {
        orientation = LinearLayout.HORIZONTAL
        gravity = Gravity.CENTER_VERTICAL
        block()
    }

    private fun dialog(): AlertDialog.Builder = AlertDialog.Builder(this)

    private fun AlertDialog.showDirected(): AlertDialog {
        show()
        window?.decorView?.layoutDirection = direction()
        return this
    }

    private fun direction() = if (I18n.rtl) View.LAYOUT_DIRECTION_RTL else View.LAYOUT_DIRECTION_LTR

    private fun message(s: String) {
        dialog().setMessage(s).setPositiveButton(T("Close"), null).create().showDirected()
    }

    // ---- the screen ---------------------------------------------------------

    private fun render() {
        window.decorView.layoutDirection = direction()
        content.removeAllViews()
        header()
        statusCard()
        vpnsCard()
        appsCard()
        alwaysCard()
        settingsCard()
        showState()
    }

    private fun header() {
        content.addView(row {
            addView(ImageView(context).apply { setImageResource(R.mipmap.ic_launcher) }, lp(dp(44), dp(44)))
            val titles = LinearLayout(context).apply { orientation = LinearLayout.VERTICAL }
            titles.addView(text("vpn_specify", 19f, bold = true))
            titles.addView(text(T("VPN only for the apps you choose"), 12.5f, muted))
            addView(titles, lp(0, start = 12, weight = 1f))
        })
    }

    private fun statusCard() = card {
        addView(row {
            dot = View(context)
            addView(dot, lp(dp(10), dp(10)))
            stateText = text("", 16f, bold = true)
            addView(stateText, lp(0, start = 8, weight = 1f))
        })
        detailText = text("", 13f, muted)
        addView(detailText, lp(top = 4))
        connectButton = button("", primary = true) { toggle() }
        addView(connectButton, lp(top = 10))
    }

    private fun showState() {
        if (!::stateText.isInitialized) return
        val on = VpnState.phase == VpnState.Phase.CONNECTED || VpnState.phase == VpnState.Phase.CONNECTING
        val (label, c) = when (VpnState.phase) {
            VpnState.Phase.CONNECTED -> T("Connected") to okColor
            VpnState.Phase.CONNECTING -> T("Connecting…") to accent
            VpnState.Phase.ERROR -> T("Error") to errColor
            VpnState.Phase.OFF -> T("Disconnected") to muted
        }
        stateText.text = label
        dot.background = GradientDrawable().apply {
            shape = GradientDrawable.OVAL
            setColor(c)
        }
        detailText.setTextColor(if (VpnState.phase == VpnState.Phase.ERROR) errColor else muted)
        detailText.text = when (VpnState.phase) {
            VpnState.Phase.CONNECTED -> {
                val s = VpnState.stats
                T(
                    "{0} · received {1}, sent {2}",
                    VpnState.config ?: "",
                    Formatter.formatShortFileSize(this, s.rx),
                    Formatter.formatShortFileSize(this, s.tx),
                )
            }
            VpnState.Phase.OFF -> T("Only the apps below use the VPN. The rest of the phone stays outside it.")
            else -> VpnState.detail
        }
        connectButton.text = if (on) T("Disconnect") else T("Connect")
    }

    private fun toggle() {
        val on = VpnState.phase == VpnState.Phase.CONNECTED || VpnState.phase == VpnState.Phase.CONNECTING
        if (on) TunnelService.stop(this) else connect()
    }

    private fun connect() {
        val active = store.active
        when {
            active == null -> return message(T("Import a VPN first."))
            store.apps.isEmpty() -> return message(T("Choose at least one app first."))
        }
        if (needsLogin(active!!)) return askLogin(active) { connect() }
        if (sdkAtLeast33 && checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
            requestPermissions(arrayOf(Manifest.permission.POST_NOTIFICATIONS), REQUEST_NOTIFICATIONS)
        }
        val consent = VpnService.prepare(this)
        if (consent != null) startActivityForResult(consent, REQUEST_VPN) else startTunnel()
    }

    private fun needsLogin(name: String): Boolean = store.isOpenVpn(name) && store.login(name) == null &&
        runCatching { Native.planOpenVpn(store.read(name)).needsAuth }.getOrDefault(false)

    private fun askLogin(name: String, then: () -> Unit) {
        val user = EditText(this).apply {
            hint = T("User")
            setSingleLine()
            setText(store.savedUser(name).orEmpty())
        }
        val password = EditText(this).apply {
            hint = T("Password")
            setSingleLine()
            inputType = android.text.InputType.TYPE_CLASS_TEXT or android.text.InputType.TYPE_TEXT_VARIATION_PASSWORD
        }
        val box = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(20), dp(8), dp(20), 0)
            addView(user, lp())
            addView(password, lp(top = 6))
        }
        dialog().setTitle(name).setView(box)
            .setPositiveButton(T("Connect")) { _, _ ->
                if (user.text.isNotEmpty()) {
                    store.saveLogin(name, user.text.toString(), password.text.toString())
                    then()
                }
            }
            .setNegativeButton(T("Cancel"), null).create().showDirected()
    }

    private fun startTunnel() {
        VpnState.set(VpnState.Phase.CONNECTING, T("Connecting…"), config = store.active, tunUp = false, apps = emptySet())
        TunnelService.start(this)
    }

    /** After changing the VPN or the apps: Android needs a new tunnel for that. */
    private fun restartIfRunning() {
        val on = VpnState.phase == VpnState.Phase.CONNECTED || VpnState.phase == VpnState.Phase.CONNECTING
        if (!on) return
        if (store.apps.isEmpty() || store.active == null) TunnelService.stop(this) else startTunnel()
    }

    private fun vpnsCard() = card(T("VPNs")) {
        val configs = store.configs()
        val active = store.active
        if (configs.isEmpty()) hint(T("No VPN yet. Import a .conf or .ovpn file."))
        for (name in configs) {
            addView(row {
                val selected = name == active
                addView(android.widget.RadioButton(context).apply {
                    isChecked = selected
                    isClickable = false
                    isFocusable = false
                })
                addView(text(name), lp(0, start = 4, weight = 1f))
                addView(rowButton(T("Remove")) { confirmRemoveConfig(name) })
                setOnClickListener {
                    if (!selected) {
                        store.active = name
                        render()
                        restartIfRunning()
                    }
                }
            }, lp(top = 6))
        }
        addView(button(T("Import file…")) { pickFile() }, lp(top = 10))
        addView(button(T("Search VPN Gate…")) { searchVpnGate() }, lp(top = 6))
        hint(T("WireGuard (.conf) and OpenVPN (.ovpn) files are supported."))
    }

    private fun confirmRemoveConfig(name: String) {
        dialog().setMessage(T("Remove {0}?", name))
            .setPositiveButton(T("Remove")) { _, _ ->
                val wasActive = store.active == name
                store.delete(name)
                render()
                if (wasActive) restartIfRunning()
            }
            .setNegativeButton(T("Cancel"), null).create().showDirected()
    }

    private fun appsCard() = card(T("Apps in the VPN")) {
        val pm = packageManager
        val apps = store.apps.mapNotNull { pkg ->
            runCatching { pm.getApplicationInfo(pkg, 0) }.getOrNull()?.let { pkg to it }
        }.sortedBy { pm.getApplicationLabel(it.second).toString().lowercase() }
        if (apps.isEmpty()) hint(T("No app yet. Add the apps that should use the VPN, Discord for example."))
        for ((pkg, info) in apps) {
            val label = pm.getApplicationLabel(info).toString()
            addView(row {
                addView(ImageView(context).apply { setImageDrawable(pm.getApplicationIcon(info)) }, lp(dp(34), dp(34)))
                addView(text(label), lp(0, start = 10, weight = 1f))
                addView(rowButton(T("Open")) { startActivity(LaunchActivity.intent(this@MainActivity, pkg)) })
                addView(rowButton(T("Shortcut")) { pinShortcut(pkg, label, pm.getApplicationIcon(info)) }, lp(ViewGroup.LayoutParams.WRAP_CONTENT, start = 6))
                addView(rowButton("✕") {
                    store.apps = store.apps - pkg
                    render()
                    restartIfRunning()
                }, lp(ViewGroup.LayoutParams.WRAP_CONTENT, start = 6))
            }, lp(top = 8))
        }
        addView(button(T("Choose apps…")) { pickApps() }, lp(top = 10))
    }

    private fun alwaysCard() = card(T("Keep the apps in the VPN")) {
        hint(T("Android cannot notice an app being opened. To have an app always in the VPN, leave the VPN on: only the chosen apps use it, and the rest of the phone is not affected."))
        addView(switchRow(T("Connect when the phone starts"), store.connectOnBoot) { on ->
            store.connectOnBoot = on
            if (on && VpnService.prepare(this@MainActivity) != null) {
                message(T("Connect once with the button above, so that Android gives the VPN permission."))
            }
        }, lp(top = 6))
        addView(button(T("Open the system's VPN settings")) {
            startActivity(Intent(Settings.ACTION_VPN_SETTINGS))
        }, lp(top = 6))
        hint(T("There, in the gear next to vpn_specify, \"Always-on VPN\" makes Android restart it by itself, and \"Block connections without VPN\" stops the chosen apps from ever leaving outside it. That second option also blocks the rest of the phone."))
    }

    private fun settingsCard() = card(T("Settings")) {
        val current = store.language
        val names = listOf(T("System default")) + I18n.languages.map { it.name }
        val shown = I18n.languages.firstOrNull { it.code == current }?.name ?: names[0]
        addView(button(T("Language: {0}", shown)) {
            dialog().setItems(names.toTypedArray()) { _, i ->
                I18n.choose(this@MainActivity, if (i == 0) null else I18n.languages[i - 1].code)
                render()
            }.create().showDirected()
        }, lp(top = 8))
        addView(switchRow(T("Check for updates when opening"), store.updateCheck) { store.updateCheck = it }, lp(top = 6))
        addView(button(T("Check for updates now")) { checkForUpdate(quiet = false) }, lp(top = 6))
        addView(row {
            addView(button(T("Log")) {
                message(Native.log().ifEmpty { T("Nothing in the log yet.") })
            }, lp(0, weight = 1f))
            addView(button(T("Licenses")) { showLicenses() }, lp(0, start = 8, weight = 1f))
        }, lp(top = 6))
        hint(T("Version {0}", BuildConfig.VERSION_NAME))
    }

    private fun switchRow(label: String, value: Boolean, change: (Boolean) -> Unit) = row {
        addView(text(label), lp(0, weight = 1f))
        addView(Switch(context).apply {
            isChecked = value
            setOnCheckedChangeListener { _, on -> change(on) }
        })
    }

    private fun showLicenses() {
        val names = assets.list("licenses").orEmpty().sorted()
        dialog().setItems(names.map { it.removeSuffix(".txt") }.toTypedArray()) { _, i ->
            message(assets.open("licenses/" + names[i]).bufferedReader().use { it.readText() })
        }.setNegativeButton(T("Close"), null).create().showDirected()
    }

    // ---- importing a configuration ------------------------------------------

    private fun pickFile() {
        val i = Intent(Intent.ACTION_OPEN_DOCUMENT).addCategory(Intent.CATEGORY_OPENABLE).setType("*/*")
        startActivityForResult(i, REQUEST_FILE)
    }

    private fun displayName(uri: Uri): String {
        contentResolver.query(uri, arrayOf(OpenableColumns.DISPLAY_NAME), null, null, null)?.use { c ->
            if (c.moveToFirst()) c.getString(0)?.let { return it }
        }
        return uri.lastPathSegment ?: "vpn"
    }

    private fun importFile(uri: Uri) {
        val file = displayName(uri)
        val body = try {
            contentResolver.openInputStream(uri)!!.use { String(it.readNBytes(MAX_CONFIG), Charsets.UTF_8) }
        } catch (e: Exception) {
            return message(e.message ?: e.toString())
        }
        // The same test as Open in internal/tunnel/tunnel.go
        val wireGuard = body.contains("[interface]", ignoreCase = true)
        try {
            if (wireGuard) Native.plan(body) else Native.planOpenVpn(body)
        } catch (e: Native.GoError) {
            return message(e.message ?: "")
        }
        val name = store.save(file.substringBeforeLast('.'), body, if (wireGuard) "conf" else "ovpn")
        if (store.configs().size == 1) store.active = name
        render()
    }

    // ---- VPN Gate ------------------------------------------------------------

    private fun searchVpnGate() {
        var all = listOf<Native.GateServer>()
        var servers = listOf<Native.GateServer>()
        var proto = ""
        val status = text(T("Searching for servers…"), 13f, muted)
        val none = text(T("No server matches this filter."), 13f, muted).apply { visibility = View.GONE }
        val adapter = object : BaseAdapter() {
            override fun getCount() = servers.size
            override fun getItem(i: Int) = servers[i]
            override fun getItemId(i: Int) = i.toLong()
            override fun getView(i: Int, reuse: View?, parent: ViewGroup): View = gateRow(servers[i]) {
                notifyDataSetChanged()
            }
        }
        val query = EditText(this).apply {
            hint = T("Search country…")
            setSingleLine()
            visibility = View.GONE
        }
        val protocols = android.widget.RadioGroup(this).apply {
            orientation = android.widget.RadioGroup.HORIZONTAL
            visibility = View.GONE
        }
        fun applyFilters() {
            servers = all.filter { VpnGate.matches(it, query.text.toString(), proto) }
            none.visibility = if (all.isNotEmpty() && servers.isEmpty()) View.VISIBLE else View.GONE
            adapter.notifyDataSetChanged()
        }
        for ((label, value) in listOf(T("All protocols") to "", "TCP" to "tcp", "UDP" to "udp")) {
            protocols.addView(android.widget.RadioButton(this).apply {
                text = label
                id = View.generateViewId()
                isChecked = value.isEmpty()
                setOnClickListener {
                    proto = value
                    applyFilters()
                }
            })
        }
        query.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, a: Int, b: Int, c: Int) = Unit
            override fun onTextChanged(s: CharSequence?, a: Int, b: Int, c: Int) = Unit
            override fun afterTextChanged(s: Editable?) = applyFilters()
        })
        val box = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(16), dp(4), dp(16), 0)
            addView(text(T("VPN Gate servers are run by volunteers. An operator can see traffic that is not encrypted, and servers may keep logs, so avoid them for banking and important accounts. Some may also be offline: if one does not connect, try another."), 12.5f, muted))
            addView(status, lp(top = 8))
            addView(query, lp(top = 4))
            addView(protocols, lp())
            addView(none, lp(top = 4))
            addView(ListView(context).apply { this.adapter = adapter }, lp(h = dp(320), top = 4))
        }
        dialog().setTitle(T("VPN Gate servers")).setView(box)
            .setPositiveButton(T("Close"), null).create().showDirected()

        thread {
            val result = runCatching { VpnGate.search(this) }
            main.post {
                result.onSuccess { r ->
                    all = r.servers
                    query.visibility = if (all.isEmpty()) View.GONE else View.VISIBLE
                    protocols.visibility = query.visibility
                    applyFilters()
                    status.text = when {
                        all.isEmpty() -> T("VPN Gate has no server available right now.")
                        r.near.isNotEmpty() -> T("Closest to {0} first.", VpnGate.countryName(r.near, r.near))
                        else -> T("Your country was not detected: the best rated servers come first.")
                    }
                }.onFailure { e ->
                    status.text = e.message ?: e.toString()
                }
            }
        }
    }

    private fun gateRow(s: Native.GateServer, changed: () -> Unit): View = row {
        setPadding(0, dp(6), 0, dp(6))
        val info = LinearLayout(context).apply {
            orientation = LinearLayout.VERTICAL
            addView(text("${VpnGate.countryName(s.country, s.countryName)} · ${s.proto.uppercase()}", bold = true))
            addView(text(s.host, 12.5f, muted))
            addView(text(T("Port {0} · {1} Mbps · {2} sessions", s.port, "%.1f".format(s.speed / 1e6), s.sessions), 12.5f, muted))
        }
        addView(info, lp(0, weight = 1f))
        val added = s.name in store.configs()
        addView(rowButton(if (added) T("Added") else T("Add")) {
            try {
                val config = Native.vpnGateConfig(s.id)
                Native.planOpenVpn(config)
                val name = store.save(s.name, config, "ovpn")
                if (store.configs().size == 1) store.active = name
                render()
                changed()
            } catch (e: Native.GoError) {
                message(e.message ?: "")
            }
        }.apply { isEnabled = !added })
    }

    // ---- choosing apps ------------------------------------------------------

    private class AppEntry(val pkg: String, val label: String, val icon: Drawable)

    private fun pickApps() {
        val chosen = store.apps.toMutableSet()
        val all = ArrayList<AppEntry>()
        var shown = listOf<AppEntry>()
        val adapter = object : BaseAdapter() {
            override fun getCount() = shown.size
            override fun getItem(i: Int) = shown[i]
            override fun getItemId(i: Int) = i.toLong()
            override fun getView(i: Int, reuse: View?, parent: ViewGroup): View {
                val a = shown[i]
                val r = (reuse as? LinearLayout) ?: row {
                    setPadding(dp(16), dp(8), dp(16), dp(8))
                    addView(ImageView(context), lp(dp(36), dp(36)))
                    addView(text(""), lp(0, start = 12, weight = 1f))
                    addView(CheckBox(context).apply {
                        isClickable = false
                        isFocusable = false
                    })
                }
                (r.getChildAt(0) as ImageView).setImageDrawable(a.icon)
                (r.getChildAt(1) as TextView).text = a.label
                (r.getChildAt(2) as CheckBox).isChecked = a.pkg in chosen
                return r
            }
        }
        val list = ListView(this).apply { this.adapter = adapter }
        val search = EditText(this).apply {
            hint = T("Search")
            setSingleLine()
        }
        fun filter() {
            val q = search.text.toString().trim().lowercase()
            shown = all.filter { q.isEmpty() || it.label.lowercase().contains(q) || it.pkg.contains(q) }
            adapter.notifyDataSetChanged()
        }
        search.addTextChangedListener(object : TextWatcher {
            override fun beforeTextChanged(s: CharSequence?, a: Int, b: Int, c: Int) = Unit
            override fun onTextChanged(s: CharSequence?, a: Int, b: Int, c: Int) = Unit
            override fun afterTextChanged(s: Editable?) = filter()
        })
        list.setOnItemClickListener { _, _, i, _ ->
            val p = shown[i].pkg
            if (!chosen.add(p)) chosen.remove(p)
            adapter.notifyDataSetChanged()
        }
        val box = LinearLayout(this).apply {
            orientation = LinearLayout.VERTICAL
            setPadding(dp(16), dp(4), dp(16), 0)
            addView(search, lp())
            addView(list, lp(h = dp(420)))
        }
        dialog().setTitle(T("Choose apps…")).setView(box)
            .setPositiveButton(T("Save")) { _, _ ->
                if (chosen != store.apps) {
                    store.apps = chosen
                    render()
                    restartIfRunning()
                }
            }
            .setNegativeButton(T("Cancel"), null).create().showDirected()

        thread {
            val pm = packageManager
            val launcher = Intent(Intent.ACTION_MAIN).addCategory(Intent.CATEGORY_LAUNCHER)
            val found = pm.queryIntentActivities(launcher, 0)
                .map { it.activityInfo.applicationInfo }
                .distinctBy { it.packageName }
                .filter { it.packageName != packageName }
                .map { AppEntry(it.packageName, pm.getApplicationLabel(it).toString(), pm.getApplicationIcon(it)) }
                .sortedBy { it.label.lowercase() }
            main.post {
                all.addAll(found)
                filter()
            }
        }
    }

    // ---- shortcuts ----------------------------------------------------------

    private fun pinShortcut(pkg: String, label: String, icon: Drawable) {
        val sm = getSystemService(ShortcutManager::class.java)
        if (!sm.isRequestPinShortcutSupported) return message(T("This phone's home screen does not accept shortcuts."))
        val info = ShortcutInfo.Builder(this, "open-$pkg")
            .setShortLabel(T("{0} (VPN)", label))
            .setIcon(Icon.createWithBitmap(badged(icon)))
            .setIntent(LaunchActivity.intent(this, pkg))
            .build()
        sm.requestPinShortcut(info, null)
    }

    /** The app's icon with a dot, to tell it apart from the normal one. */
    private fun badged(icon: Drawable): Bitmap {
        val size = dp(48)
        val bmp = Bitmap.createBitmap(size, size, Bitmap.Config.ARGB_8888)
        val c = Canvas(bmp)
        icon.setBounds(0, 0, size, size)
        icon.draw(c)
        val p = Paint(Paint.ANTI_ALIAS_FLAG)
        val r = size * 0.17f
        p.color = Color.WHITE
        c.drawCircle(size - r, size - r, r, p)
        p.color = Color.parseColor("#2F6FED")
        c.drawCircle(size - r, size - r, r * 0.72f, p)
        return bmp
    }

    // ---- updates ------------------------------------------------------------

    private fun checkForUpdate(quiet: Boolean) {
        thread {
            try {
                val r = Updater.check()
                main.post {
                    if (isDestroyed) return@post
                    when {
                        r != null -> offerUpdate(r)
                        !quiet -> message(T("You already have the latest version."))
                    }
                }
            } catch (e: Exception) {
                if (!quiet) main.post { if (!isDestroyed) message(T("could not reach GitHub") + ": " + e.message) }
            }
        }
    }

    private fun offerUpdate(r: Updater.Release) {
        dialog().setTitle(T("Version {0} is available", r.version))
            .setMessage(r.notes.take(1500).ifEmpty { T("(this version has no description)") })
            .setPositiveButton(T("Update")) { _, _ -> installUpdate(r) }
            .setNegativeButton(T("Not now"), null).create().showDirected()
    }

    @Suppress("DEPRECATION")
    private fun installUpdate(r: Updater.Release) {
        if (!packageManager.canRequestPackageInstalls()) {
            Toast.makeText(this, T("Allow vpn_specify to install updates, then come back and try again."), Toast.LENGTH_LONG).show()
            startActivity(Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:$packageName")))
            return
        }
        val progress = ProgressDialog(this).apply {
            setMessage(T("Downloading…"))
            setProgressStyle(ProgressDialog.STYLE_HORIZONTAL)
            max = 100
            setCancelable(false)
            show()
        }
        thread {
            try {
                val apk = Updater.download(this, r) { p -> main.post { progress.progress = p } }
                Updater.install(this, apk)
                main.post { progress.dismiss() }
            } catch (e: Exception) {
                android.util.Log.w("vpn_specify", "update failed", e)
                main.post {
                    progress.dismiss()
                    if (!isDestroyed) message(T("The update failed: {0}", e.message ?: e.toString()))
                }
            }
        }
    }

    // ---- results ------------------------------------------------------------

    @Deprecated("Deprecated in Java")
    override fun onActivityResult(requestCode: Int, resultCode: Int, data: Intent?) {
        @Suppress("DEPRECATION") super.onActivityResult(requestCode, resultCode, data)
        if (resultCode != RESULT_OK) return
        when (requestCode) {
            REQUEST_VPN -> startTunnel()
            REQUEST_FILE -> data?.data?.let { importFile(it) }
        }
    }

    companion object {
        private const val REQUEST_VPN = 1
        private const val REQUEST_FILE = 2
        private const val REQUEST_NOTIFICATIONS = 3
        private const val MAX_CONFIG = 1 shl 20
    }
}
