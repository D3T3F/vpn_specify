package io.github.d3t3f.vpnspecify

import android.content.Context
import android.telephony.TelephonyManager
import java.io.ByteArrayOutputStream
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.text.Normalizer
import java.util.Locale

/**
 * The public servers of VPN Gate. This app stays outside its own VPN, so these
 * requests use the normal network and tell the user's real country. Reading
 * the answers is done in Go (internal/vpngate), shared with the desktop.
 */
object VpnGate {
    private const val API = "https://www.vpngate.net/api/iphone/"
    private const val HOME = "https://www.vpngate.net/en/"
    private const val MAX_LIST = 32 shl 20
    private const val MAX_HOME = 4 shl 20

    private fun get(url: String, limit: Int): String {
        val c = URL(url).openConnection() as HttpURLConnection
        c.connectTimeout = 20_000
        c.readTimeout = 60_000
        c.setRequestProperty("User-Agent", "vpn_specify/${BuildConfig.VERSION_NAME}")
        try {
            if (c.responseCode != 200) throw IOException(T("VPN Gate answered {0}", c.responseCode))
            val out = ByteArrayOutputStream()
            c.inputStream.use { input ->
                val buf = ByteArray(64 * 1024)
                while (out.size() < limit) {
                    val n = input.read(buf)
                    if (n < 0) break
                    out.write(buf, 0, n)
                }
            }
            return out.toString("UTF-8")
        } finally {
            c.disconnect()
        }
    }

    /** The phone's country, for when the VPN Gate page does not tell it. */
    private fun systemRegion(ctx: Context): String {
        val tm = ctx.getSystemService(TelephonyManager::class.java)
        val country = listOf(tm?.networkCountryIso, tm?.simCountryIso, Locale.getDefault().country)
            .firstOrNull { !it.isNullOrEmpty() }.orEmpty()
        return "und-${country.uppercase()}"
    }

    /** Blocks. Only the list is required: the page just tells the country. */
    fun search(ctx: Context): Native.GateList {
        val list = try {
            get(API, MAX_LIST)
        } catch (e: IOException) {
            throw IOException(T("could not reach VPN Gate: {0}", e.message ?: e.toString()), e)
        }
        val home = runCatching { get(HOME, MAX_HOME) }.getOrNull()
        return Native.vpnGateList(list, home, systemRegion(ctx))
    }

    /** Lowercase without accents, so "Croacia" finds "Croácia". */
    private val accents = Regex("\\p{Mn}+")

    private fun fold(s: String): String =
        Normalizer.normalize(s, Normalizer.Form.NFD).replace(accents, "").lowercase(Locale.ROOT)

    /**
     * The filters of the list. [proto] is "", "tcp" or "udp". A country is found
     * by its name in the app's language, its English name or its exact code.
     */
    fun matches(s: Native.GateServer, query: String, proto: String): Boolean {
        if (proto.isNotEmpty() && s.proto != proto) return false
        val q = fold(query.trim())
        if (q.isEmpty()) return true
        return fold(s.country) == q || fold(countryName(s.country, s.countryName)).contains(q) ||
            fold(s.countryName).contains(q)
    }

    /** The country in the language of the app. */
    fun countryName(code: String, fallback: String): String =
        Locale("", code).getDisplayCountry(Locale.forLanguageTag(I18n.code)).ifEmpty { fallback }
}
