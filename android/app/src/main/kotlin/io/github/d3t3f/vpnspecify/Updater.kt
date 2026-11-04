package io.github.d3t3f.vpnspecify

import android.app.PendingIntent
import android.content.Context
import android.content.Intent
import android.content.pm.PackageInstaller
import android.os.Build
import org.json.JSONObject
import java.io.File
import java.io.IOException
import java.net.HttpURLConnection
import java.net.URL
import java.security.MessageDigest

/**
 * Updates from the GitHub releases, like the desktop versions. Android itself
 * refuses an update not signed with the key of the installed app; the SHA-256
 * published by GitHub also catches a damaged download.
 */
object Updater {
    private const val DOWNLOAD_PREFIX = "https://github.com/D3T3F/vpn_specify/releases/download/"

    data class Release(val version: String, val notes: String, val url: String, val sha256: String)

    /** "1.2.3" or "v1.2.3" as numbers, or null. */
    private fun parse(v: String): List<Int>? {
        val parts = v.removePrefix("v").split(".")
        if (parts.size != 3) return null
        return parts.map { it.toIntOrNull() ?: return null }
    }

    fun newer(a: String, b: String): Boolean {
        val x = parse(a) ?: return false
        val y = parse(b) ?: return false
        for (i in 0..2) if (x[i] != y[i]) return x[i] > y[i]
        return false
    }

    private fun assetName(version: String): String {
        val arch = when (Build.SUPPORTED_ABIS.firstOrNull()) {
            "arm64-v8a" -> "arm64"
            else -> Build.SUPPORTED_ABIS.firstOrNull() ?: "unknown"
        }
        return "vpn_specify-$version-android-$arch.apk"
    }

    private fun open(url: String): HttpURLConnection {
        val c = URL(url).openConnection() as HttpURLConnection
        c.connectTimeout = 15_000
        c.readTimeout = 30_000
        c.setRequestProperty("User-Agent", "vpn_specify/${BuildConfig.VERSION_NAME}")
        return c
    }

    /** A newer release with a package for this phone, or null. Blocks. */
    fun check(): Release? {
        val c = open(BuildConfig.UPDATE_API)
        c.setRequestProperty("Accept", "application/vnd.github+json")
        try {
            if (c.responseCode != 200) throw IOException(T("GitHub answered {0}", c.responseCode))
            val o = JSONObject(c.inputStream.bufferedReader().use { it.readText() })
            val tag = o.getString("tag_name")
            if (!newer(tag, BuildConfig.VERSION_NAME)) return null
            val version = tag.removePrefix("v")
            val want = assetName(version)
            val assets = o.optJSONArray("assets") ?: return null
            for (i in 0 until assets.length()) {
                val a = assets.getJSONObject(i)
                if (a.getString("name") != want) continue
                val url = a.getString("browser_download_url")
                if (!BuildConfig.DEBUG && !url.startsWith(DOWNLOAD_PREFIX)) return null
                return Release(version, o.optString("body"), url, a.optString("digest").removePrefix("sha256:"))
            }
            return null
        } finally {
            c.disconnect()
        }
    }

    /** Downloads and verifies the APK. [progress] gets 0..100. Blocks. */
    fun download(ctx: Context, r: Release, progress: (Int) -> Unit): File {
        val dir = File(ctx.cacheDir, "update").apply {
            deleteRecursively()
            mkdirs()
        }
        val out = File(dir, "update.apk")
        val c = open(r.url)
        try {
            if (c.responseCode != 200) throw IOException(T("GitHub answered {0}", c.responseCode))
            val total = c.contentLengthLong
            val digest = MessageDigest.getInstance("SHA-256")
            c.inputStream.use { input ->
                out.outputStream().use { output ->
                    val buf = ByteArray(64 * 1024)
                    var done = 0L
                    while (true) {
                        val n = input.read(buf)
                        if (n < 0) break
                        output.write(buf, 0, n)
                        digest.update(buf, 0, n)
                        done += n
                        if (total > 0) progress((done * 100 / total).toInt())
                    }
                }
            }
            val got = digest.digest().joinToString("") { "%02x".format(it) }
            if (r.sha256.isNotEmpty() && !got.equals(r.sha256, ignoreCase = true)) {
                out.delete()
                throw IOException(T("the download is corrupted (the SHA-256 does not match)"))
            }
        } finally {
            c.disconnect()
        }
        return out
    }

    /** Hands the APK to Android, which asks the user to confirm. */
    fun install(ctx: Context, apk: File) {
        val installer = ctx.packageManager.packageInstaller
        val params = PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL)
        params.setAppPackageName(ctx.packageName)
        val id = installer.createSession(params)
        installer.openSession(id).use { session ->
            session.openWrite("update.apk", 0, apk.length()).use { out ->
                apk.inputStream().use { it.copyTo(out) }
                session.fsync(out)
            }
            val status = PendingIntent.getBroadcast(
                ctx, id, Intent(ctx, InstallReceiver::class.java),
                PendingIntent.FLAG_UPDATE_CURRENT or PendingIntent.FLAG_MUTABLE,
            )
            session.commit(status.intentSender)
        }
    }
}
