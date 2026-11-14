package io.github.d3t3f.vpnspecify

import java.io.ByteArrayOutputStream
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.util.Locale

/**
 * The OpenVPN servers of RiseupVPN. This app stays outside its own VPN, so these
 * requests use the normal network. Reading the answers and building the
 * configuration is done in Go (internal/riseup), shared with the desktop.
 *
 * The client certificate is short lived and is downloaded only when the user
 * adds a server, never when the list is shown.
 */
object RiseUp {
    private const val GATEWAYS = "https://api.black.riseup.net/3/config/eip-service.json"
    private const val CERT = "https://api.black.riseup.net/3/cert"
    private const val CA = "https://black.riseup.net/ca.crt"
    private const val MAX = 1 shl 20

    /** Same rule as the desktop (internal/riseup): https only, on riseup.net or one of its subdomains. */
    private fun trusted(url: URL): Boolean =
        url.protocol == "https" && (url.host == "riseup.net" || url.host.endsWith(".riseup.net"))

    private fun get(url: String): String {
        var target = URL(url)
        require(trusted(target))
        // Redirects are followed by hand so each one is checked: the CA that comes
        // from here becomes the trust anchor of the saved configuration.
        for (hop in 0..5) {
            val c = target.openConnection() as HttpURLConnection
            c.instanceFollowRedirects = false
            c.connectTimeout = 20_000
            c.readTimeout = 20_000
            c.setRequestProperty("User-Agent", "vpn_specify/${BuildConfig.VERSION_NAME}")
            try {
                val code = c.responseCode
                if (code in intArrayOf(301, 302, 303, 307, 308)) {
                    val next = c.getHeaderField("Location")?.let { URL(target, it) }
                    if (next == null || !trusted(next) || hop == 5) {
                        throw IOException(T("RiseUp answered {0}", code))
                    }
                    target = next
                    continue
                }
                return read(c, code)
            } finally {
                c.disconnect()
            }
        }
        throw IOException(T("RiseUp answered {0}", 310))
    }

    private fun read(c: HttpURLConnection, code: Int): String {
        try {
            if (code != 200) throw IOException(T("RiseUp answered {0}", code))
            val out = ByteArrayOutputStream()
            c.inputStream.use { input ->
                val buf = ByteArray(64 * 1024)
                while (true) {
                    val n = input.read(buf)
                    if (n < 0) break
                    out.write(buf, 0, n)
                    if (out.size() > MAX) throw IOException(T("RiseUp sent more data than this version accepts"))
                }
            }
            return out.toString("UTF-8")
        } finally {
            c.disconnect()
        }
    }

    private fun download(url: String): String =
        try {
            get(url)
        } catch (e: IOException) {
            throw IOException(T("could not reach RiseUp: {0}", e.message ?: e.toString()), e)
        }

    /** The gateway list. Throws [Native.GoError] if RiseUp sent what this version cannot read. */
    fun search(): Native.RiseUpList = Native.riseupList(download(GATEWAYS))

    /** Downloads the client certificate and the CA, then builds the configuration of [id]. */
    fun config(id: String): Native.RiseUpConfig = Native.riseupConfig(id, download(CERT), download(CA))

    /** The country in the language of the app, or "" if the code is unknown. */
    fun countryName(code: String): String =
        Locale("", code).getDisplayCountry(Locale.forLanguageTag(I18n.code))

    /** The filter: a country by its name in the app's language, its English name or its exact code, or a city. */
    fun matches(s: Native.RiseUpServer, query: String): Boolean {
        val q = VpnGate.fold(query.trim())
        if (q.isEmpty()) return true
        return VpnGate.fold(s.country) == q || VpnGate.fold(countryName(s.country)).contains(q) ||
            VpnGate.fold(s.city).contains(q)
    }
}
