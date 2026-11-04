package io.github.d3t3f.vpnspecify

import android.app.Application
import android.app.NotificationChannel
import android.app.NotificationManager
import android.content.Context
import android.os.Build
import android.os.Handler
import android.os.Looper
import java.io.File
import java.util.Locale

class App : Application() {
    override fun onCreate() {
        super.onCreate()
        I18n.init(this)
        createChannel(this)
    }

    companion object {
        const val CHANNEL = "vpn"

        /** Also called when the language changes, to rename the channel. */
        fun createChannel(ctx: Context) {
            val ch = NotificationChannel(CHANNEL, T("VPN status"), NotificationManager.IMPORTANCE_LOW)
            ch.setShowBadge(false)
            ctx.getSystemService(NotificationManager::class.java).createNotificationChannel(ch)
        }
    }
}

/**
 * Translation, as in the desktop versions: the English text is the key, and
 * internal/i18n/catalog_test.go checks that the calls to this function have
 * translations. Catalogs come from the Go library, so there is only one copy.
 */
fun T(msg: String, vararg args: Any?): String {
    var s = I18n.catalog[msg]?.takeIf { it.isNotBlank() } ?: msg
    args.forEachIndexed { i, a -> s = s.replace("{$i}", a.toString()) }
    return s
}

object I18n {
    @Volatile var catalog: Map<String, String> = emptyMap()
        private set
    @Volatile var code = "en"
        private set
    var rtl = false
        private set

    val languages: List<Native.Language> by lazy { Native.languages() }

    fun init(ctx: Context) {
        val saved = Store.of(ctx).language
        apply(saved ?: detect())
    }

    /** The first system language the app supports, or English. */
    private fun detect(): String {
        val locales = android.content.res.Resources.getSystem().configuration.locales
        for (i in 0 until locales.size()) {
            val c = Native.matchLanguage(locales[i].toLanguageTag())
            if (c.isNotEmpty()) return c
        }
        return Native.matchLanguage(Locale.getDefault().toLanguageTag()).ifEmpty { "en" }
    }

    /** [choice] null: follow the system again. */
    fun choose(ctx: Context, choice: String?) {
        Store.of(ctx).language = choice
        apply(choice ?: detect())
        App.createChannel(ctx)
    }

    private fun apply(c: String) {
        catalog = Native.setLanguage(c)
        code = c
        rtl = languages.firstOrNull { it.code == c }?.rtl == true
    }
}

/** App settings and the imported configurations, all in private storage. */
class Store private constructor(ctx: Context) {
    private val prefs = ctx.getSharedPreferences("vpn_specify", Context.MODE_PRIVATE)
    private val dir = File(ctx.filesDir, "configs").apply { mkdirs() }

    var language: String?
        get() = prefs.getString("language", null)
        set(v) = prefs.edit().putString("language", v).apply()

    var active: String?
        get() = prefs.getString("active", null)?.takeIf { it in configs() } ?: configs().firstOrNull()
        set(v) = prefs.edit().putString("active", v).apply()

    var apps: Set<String>
        get() = prefs.getStringSet("apps", emptySet())!!.toSet()
        set(v) = prefs.edit().putStringSet("apps", v).apply()

    var connectOnBoot: Boolean
        get() = prefs.getBoolean("connectOnBoot", false)
        set(v) = prefs.edit().putBoolean("connectOnBoot", v).apply()

    /** The user wants the VPN on: reconnect if the system restarts the service. */
    var wantOn: Boolean
        get() = prefs.getBoolean("wantOn", false)
        set(v) = prefs.edit().putBoolean("wantOn", v).apply()

    var updateCheck: Boolean
        get() = prefs.getBoolean("updateCheck", true)
        set(v) = prefs.edit().putBoolean("updateCheck", v).apply()

    fun configs(): List<String> =
        dir.listFiles { f -> f.name.endsWith(".conf") || f.name.endsWith(".ovpn") }.orEmpty()
            .map { it.name.substringBeforeLast('.') }.sorted()

    private fun file(name: String): File {
        val ovpn = File(dir, "$name.ovpn")
        return if (ovpn.exists()) ovpn else File(dir, "$name.conf")
    }

    fun read(name: String): String = file(name).readText()

    fun isOpenVpn(name: String) = file(name).name.endsWith(".ovpn")

    /** The saved login of an OpenVPN configuration, if any. */
    fun login(name: String): Pair<String, String>? {
        val u = prefs.getString("user:$name", null) ?: return null
        val p = prefs.getString("password:$name", null) ?: return null
        return u to p
    }

    /** Kept when the password is forgotten, to fill in the next question. */
    fun savedUser(name: String): String? = prefs.getString("user:$name", null)

    fun saveLogin(name: String, user: String, password: String) {
        prefs.edit().putString("user:$name", user).putString("password:$name", password).apply()
    }

    fun forgetPassword(name: String) {
        prefs.edit().remove("password:$name").apply()
    }

    /** Saves under a free name based on [wanted] and returns that name. */
    fun save(wanted: String, text: String, extension: String = "conf"): String {
        val base = wanted.replace(Regex("[^\\p{L}\\p{N} ._-]"), "_").trim().trimStart('.').ifEmpty { "vpn" }.take(60)
        var name = base
        var n = 2
        while (name in configs()) name = "$base (${n++})"
        File(dir, "$name.$extension").writeText(text)
        return name
    }

    fun delete(name: String) {
        file(name).delete()
        prefs.edit().remove("user:$name").remove("password:$name").apply()
    }

    companion object {
        @Volatile private var instance: Store? = null
        fun of(ctx: Context): Store =
            instance ?: synchronized(this) { instance ?: Store(ctx.applicationContext).also { instance = it } }
    }
}

/** What the tunnel is doing, for the screens to show. Lives in the process. */
object VpnState {
    enum class Phase { OFF, CONNECTING, CONNECTED, ERROR }

    @Volatile var phase = Phase.OFF
        private set
    @Volatile var config: String? = null
        private set
    @Volatile var detail = ""
        private set
    /** The tun interface exists: the chosen apps' traffic can no longer leave outside it. */
    @Volatile var tunUp = false
        private set
    /** The apps the running tunnel was created with. */
    @Volatile var apps: Set<String> = emptySet()
        private set
    @Volatile var stats = Native.Stats(0, 0, 0)

    private val main = Handler(Looper.getMainLooper())
    private val listeners = mutableSetOf<() -> Unit>()

    fun set(
        phase: Phase,
        detail: String = "",
        config: String? = this.config,
        tunUp: Boolean = this.tunUp,
        apps: Set<String> = this.apps,
    ) {
        this.phase = phase
        this.detail = detail
        this.config = config
        this.tunUp = tunUp
        this.apps = apps
        notifyListeners()
    }

    fun notifyListeners() {
        main.post { listeners.toList().forEach { it() } }
    }

    fun listen(l: () -> Unit) {
        listeners += l
    }

    fun unlisten(l: () -> Unit) {
        listeners -= l
    }

    /** Running, or on its way, with exactly these apps. */
    fun runningWith(wanted: Set<String>): Boolean =
        (phase == Phase.CONNECTED || phase == Phase.CONNECTING) && apps == wanted
}

val Context.sdkAtLeast33: Boolean get() = Build.VERSION.SDK_INT >= 33
