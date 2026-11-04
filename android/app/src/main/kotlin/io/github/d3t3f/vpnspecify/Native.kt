package io.github.d3t3f.vpnspecify

import org.json.JSONArray
import org.json.JSONObject

/** The Go side (internal/mobile): one JNI function that exchanges JSON. */
object Native {
    init {
        System.loadLibrary("vpn_specify")
    }

    @JvmStatic
    private external fun call(request: ByteArray): ByteArray

    class GoError(message: String) : Exception(message)

    private fun request(method: String, config: String? = null, fd: Int? = null, text: String? = null): Any? {
        val req = JSONObject().put("method", method)
        if (config != null) req.put("config", config)
        if (fd != null) req.put("fd", fd)
        if (text != null) req.put("text", text)
        val reply = JSONObject(String(call(req.toString().toByteArray()), Charsets.UTF_8))
        if (reply.has("error")) throw GoError(reply.getString("error"))
        return reply.opt("result")
    }

    data class Language(val code: String, val name: String, val rtl: Boolean)

    fun languages(): List<Language> {
        val a = request("languages") as JSONArray
        return List(a.length()) {
            val o = a.getJSONObject(it)
            Language(o.getString("code"), o.getString("name"), o.optBoolean("rtl"))
        }
    }

    fun matchLanguage(locale: String): String = request("matchLanguage", text = locale) as? String ?: ""

    /** Makes [code] the language of Go's messages and returns its translations. */
    fun setLanguage(code: String): Map<String, String> {
        val o = request("setLanguage", text = code) as? JSONObject ?: return emptyMap()
        return o.keys().asSequence().associateWith { o.getString(it) }
    }

    data class Plan(
        val addresses: List<String>,
        val dns: List<String>,
        val mtu: Int,
        val routes: List<String>,
    )

    private fun strings(a: JSONArray?): List<String> =
        if (a == null) emptyList() else List(a.length()) { a.getString(it) }

    /** Validates a configuration; throws [GoError] with a translated message. */
    fun plan(config: String): Plan {
        val o = request("plan", config = config) as JSONObject
        return Plan(
            strings(o.optJSONArray("addresses")),
            strings(o.optJSONArray("dns")),
            o.getInt("mtu"),
            strings(o.optJSONArray("routes")),
        )
    }

    data class OvpnPlan(val config: String, val needsAuth: Boolean)

    /** Validates a .ovpn and removes what must not run; throws [GoError]. */
    fun planOpenVpn(text: String): OvpnPlan {
        val o = request("planOpenVPN", config = text) as JSONObject
        return OvpnPlan(o.getString("config"), o.optBoolean("needsAuth"))
    }

    fun appendLog(line: String) {
        request("appendLog", text = line)
    }

    fun resolve(config: String) {
        request("resolve", config = config)
    }

    /** Starts WireGuard on [fd], which now belongs to Go. */
    fun start(fd: Int, config: String) {
        request("start", config = config, fd = fd)
    }

    fun stop() {
        request("stop")
    }

    fun rebind() {
        request("rebind")
    }

    fun kick() {
        request("kick")
    }

    data class Stats(val rx: Long, val tx: Long, val lastHandshake: Long)

    fun stats(): Stats {
        val o = request("stats") as JSONObject
        return Stats(o.getLong("rxBytes"), o.getLong("txBytes"), o.getLong("lastHandshake"))
    }

    fun log(): String = request("log") as? String ?: ""
}
