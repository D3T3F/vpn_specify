package io.github.d3t3f.vpnspecify

import android.net.LocalSocket
import android.net.LocalSocketAddress
import android.os.ParcelFileDescriptor
import android.util.Log
import java.io.File
import java.io.FileDescriptor
import java.io.IOException
import kotlin.concurrent.thread

/**
 * Runs the openvpn program (libovpnexec.so, see android/openvpn/build.sh) and
 * talks to it through its management interface. openvpn built for Android
 * creates no interface itself: it describes what it needs (addresses, routes,
 * DNS) and asks for the tun descriptor, which comes from [openTun].
 *
 * Android only lets an app run programs from its native library folder, which
 * is why the program has a library's name and is not compressed in the APK.
 */
class OpenVpnProcess(
    private val nativeDir: String,
    private val runDir: File,
    private val config: String,
    private val user: String?,
    private val password: String?,
    /** Creates the interface and returns its descriptor, or null if it could not. */
    private val openTun: (TunSettings) -> ParcelFileDescriptor?,
    private val onState: (State, String) -> Unit,
    private val onBytes: (Long, Long) -> Unit,
    private val onLog: (String) -> Unit,
) {
    enum class State { CONNECTING, CONNECTED }
    enum class Failure { AUTH, KEY_PASSWORD }

    /** What openvpn asked for, gathered until it requests the interface. */
    class TunSettings {
        val addresses = ArrayList<Pair<String, Int>>()
        val routes = ArrayList<Pair<String, Int>>()
        val dns = ArrayList<String>()
        val domains = ArrayList<String>()
        var mtu = 1500

        fun same(o: TunSettings?) = o != null && addresses == o.addresses && routes == o.routes &&
            dns == o.dns && domains == o.domains && mtu == o.mtu
    }

    private var process: Process? = null
    private var socket: LocalSocket? = null
    private val writeLock = Any()
    @Volatile private var stopping = false
    @Volatile var failure: Failure? = null
        private set
    @Volatile var lastError: String? = null
        private set

    private var pending = TunSettings()
    private var current: TunSettings? = null

    /** Runs until openvpn exits. Blocks. */
    fun run() {
        runDir.deleteRecursively()
        runDir.mkdirs()
        val conf = File(runDir, "vpn.conf").apply {
            writeText(config)
            setReadable(false, false)
            setReadable(true, true)
        }
        val sock = File(runDir, "m")
        val p = ProcessBuilder(
            "$nativeDir/libovpnexec.so",
            "--config", conf.path,
            "--management", sock.path, "unix",
            "--management-hold",
            "--management-query-passwords",
            "--script-security", "0",
            "--tmp-dir", runDir.path,
            "--suppress-timestamps",
            "--verb", "3",
        ).directory(runDir).redirectErrorStream(true).start()
        process = p
        // The password must not stay on disk any longer than needed: it never
        // goes there, management passes it. The configuration is deleted below.
        val output = thread(name = "openvpn-output") {
            runCatching {
                // Its dialogue with this app is noise in the log the user reads.
                p.inputStream.bufferedReader().forEachLine { if (!it.startsWith("MANAGEMENT:")) onLog(it) }
            }
        }
        try {
            manage(sock)
        } catch (e: IOException) {
            if (!stopping) Log.w(TAG, "management", e)
        } finally {
            runCatching { socket?.close() }
            if (!p.waitFor(EXIT_WAIT_MS)) p.destroyForcibly()
            output.join(1000)
            runDir.deleteRecursively()
        }
    }

    private fun Process.waitFor(ms: Long): Boolean {
        val end = System.currentTimeMillis() + ms
        while (System.currentTimeMillis() < end) {
            if (!isAlive) return true
            Thread.sleep(50)
        }
        return !isAlive
    }

    /** Asks openvpn to exit, from any thread. */
    fun stop() {
        stopping = true
        thread(name = "openvpn-stop") {
            runCatching { send("signal SIGINT") }
            val p = process ?: return@thread
            if (!p.waitFor(EXIT_WAIT_MS)) p.destroyForcibly()
        }
    }

    /** The network changed: openvpn moves its socket or reconnects. */
    fun networkChanged() {
        runCatching { send("network-change") }
    }

    private fun connect(path: File): LocalSocket {
        val end = System.currentTimeMillis() + CONNECT_WAIT_MS
        while (true) {
            val s = LocalSocket()
            try {
                s.connect(LocalSocketAddress(path.path, LocalSocketAddress.Namespace.FILESYSTEM))
                return s
            } catch (e: IOException) {
                s.close()
                if (stopping || process?.isAlive != true || System.currentTimeMillis() > end) throw e
                Thread.sleep(100)
            }
        }
    }

    private fun manage(path: File) {
        val s = connect(path)
        socket = s
        // Blocks until openvpn exits, which closes the socket. No read timeout:
        // on a LocalSocket it looks the same as a broken connection.
        val input = s.inputStream
        val buf = ByteArray(4096)
        val line = StringBuilder()
        while (true) {
            val n = input.read(buf)
            if (n < 0) return
            // Descriptors openvpn passes, so they can be protected. This app is
            // outside its own VPN, so there is nothing to protect them from.
            s.ancillaryFileDescriptors?.forEach { closeQuietly(it) }
            for (i in 0 until n) {
                val c = buf[i].toInt().toChar()
                if (c == '\n') {
                    handle(line.toString().trimEnd('\r'))
                    line.setLength(0)
                } else {
                    line.append(c)
                }
            }
        }
    }

    private fun closeQuietly(fd: FileDescriptor) {
        runCatching { ParcelFileDescriptor.dup(fd).close() }
        runCatching { android.system.Os.close(fd) }
    }

    private fun handle(l: String) {
        when {
            l.startsWith(">HOLD:") -> {
                send("state on")
                send("bytecount 2")
                // The number is how long openvpn wants to wait before trying
                // again, so a server that refuses us is not hammered.
                val wait = l.substringAfterLast(':').trim().toLongOrNull() ?: 0
                if (wait <= 0) {
                    send("hold release")
                } else {
                    thread(name = "openvpn-hold") {
                        Thread.sleep(wait * 1000)
                        if (!stopping) runCatching { send("hold release") }
                    }
                }
            }
            l.startsWith(">STATE:") -> {
                val f = l.removePrefix(">STATE:").split(",")
                if (f.size >= 2) when (f[1]) {
                    "CONNECTED" -> onState(State.CONNECTED, "")
                    "EXITING" -> Unit
                    else -> onState(State.CONNECTING, "")
                }
            }
            l.startsWith(">BYTECOUNT:") -> {
                val f = l.removePrefix(">BYTECOUNT:").split(",")
                if (f.size == 2) onBytes(f[0].toLongOrNull() ?: 0, f[1].toLongOrNull() ?: 0)
            }
            l.startsWith(">FATAL:") -> lastError = l.removePrefix(">FATAL:")
            l.startsWith(">PASSWORD:") -> password(l.removePrefix(">PASSWORD:"))
            l.startsWith(">NEED-OK:") -> needOk(l.removePrefix(">NEED-OK:"))
        }
    }

    private fun quote(s: String) = "\"" + s.replace("\\", "\\\\").replace("\"", "\\\"") + "\""

    private fun password(l: String) {
        when {
            l.startsWith("Verification Failed") -> {
                failure = if (l.contains("'Private Key'")) Failure.KEY_PASSWORD else Failure.AUTH
                stop()
            }
            l.startsWith("Need 'Private Key'") -> {
                failure = Failure.KEY_PASSWORD
                stop()
            }
            l.startsWith("Need 'Auth'") -> {
                if (user == null || password == null) {
                    failure = Failure.AUTH
                    stop()
                    return
                }
                send("username \"Auth\" " + quote(user))
                send("password \"Auth\" " + quote(password))
            }
        }
    }

    private fun needOk(l: String) {
        // Need 'NAME' confirmation MSG:detail
        val name = l.substringAfter('\'').substringBefore('\'')
        val detail = l.substringAfter("MSG:", "")
        when (name) {
            "IFCONFIG" -> {
                // local mask-or-peer mtu topology
                val f = detail.split(" ")
                if (f.size >= 4) {
                    val bits = if (f[3] == "subnet") prefixOf(f[1]) else 32
                    pending.addresses += f[0] to bits
                    pending.mtu = f[2].toIntOrNull() ?: pending.mtu
                    if (bits < 32) pending.routes += maskedRoute(f[0], bits)
                }
            }
            "IFCONFIG6" -> {
                val f = detail.split(" ")
                val (ip, bits) = f[0].split("/").let { it[0] to (it.getOrNull(1)?.toIntOrNull() ?: 128) }
                pending.addresses += ip to bits
            }
            "ROUTE" -> {
                // network mask gateway [dev iface]. With a device the route leaves
                // through the normal network, which is what not routing it does.
                val f = detail.split(" ")
                if (f.size >= 2 && "dev" !in f) pending.routes += maskedRoute(f[0], prefixOf(f[1]))
            }
            "ROUTE6" -> {
                val f = detail.split(" ")
                val parts = f[0].split("/")
                val isTun = f.size < 2 || f[1].startsWith("tun")
                if (parts.size == 2 && isTun) parts[1].toIntOrNull()?.let { pending.routes += parts[0] to it }
            }
            "DNSSERVER", "DNS6SERVER" -> if (detail.isNotBlank()) pending.dns += detail.trim()
            "DNSDOMAIN" -> if (detail.isNotBlank()) pending.domains += detail.trim()
            "PERSIST_TUN_ACTION" -> {
                val keep = pending.same(current)
                return send("needok '$name' " + if (keep) "NOACTION" else "OPEN_BEFORE_CLOSE").also {
                    if (keep) pending = TunSettings()
                }
            }
            "OPENTUN" -> return handOverTun()
        }
        send("needok '$name' ok")
    }

    private fun handOverTun() {
        val settings = pending
        pending = TunSettings()
        val tun = openTun(settings)
        if (tun == null) {
            send("needok 'OPENTUN' cancel")
            return
        }
        current = settings
        try {
            synchronized(writeLock) {
                val s = socket ?: throw IOException("no management connection")
                s.setFileDescriptorsForSend(arrayOf(tun.fileDescriptor))
                s.outputStream.write("needok 'OPENTUN' ok\n".toByteArray())
                s.outputStream.flush()
                s.setFileDescriptorsForSend(null)
            }
        } finally {
            // openvpn received its own copy of the descriptor
            tun.close()
        }
    }

    private fun send(command: String) {
        synchronized(writeLock) {
            val s = socket ?: return
            s.outputStream.write((command + "\n").toByteArray())
            s.outputStream.flush()
        }
    }

    companion object {
        private const val TAG = "vpn_specify"
        private const val CONNECT_WAIT_MS = 10_000L
        private const val EXIT_WAIT_MS = 3_000L

        internal fun prefixOf(mask: String): Int =
            mask.split(".").sumOf { Integer.bitCount(it.toIntOrNull() ?: 0) }

        /** Android refuses a route with host bits set. */
        internal fun maskedRoute(ip: String, bits: Int): Pair<String, Int> {
            val parts = ip.split(".").map { it.toIntOrNull() ?: 0 }
            if (parts.size != 4) return ip to bits
            val n = parts.fold(0L) { acc, p -> (acc shl 8) or p.toLong() }
            val mask = if (bits == 0) 0L else (0xFFFFFFFFL shl (32 - bits)) and 0xFFFFFFFFL
            val m = n and mask
            return "${(m shr 24) and 255}.${(m shr 16) and 255}.${(m shr 8) and 255}.${m and 255}" to bits
        }
    }
}
