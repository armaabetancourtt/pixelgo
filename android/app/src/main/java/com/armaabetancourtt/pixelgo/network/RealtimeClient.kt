package com.armaabetancourtt.pixelgo.network

import com.armaabetancourtt.pixelgo.security.SessionStore
import okhttp3.OkHttpClient
import okhttp3.Request
import okhttp3.Response
import okhttp3.WebSocket
import okhttp3.WebSocketListener
import org.json.JSONObject
import java.net.URLEncoder
import java.nio.charset.StandardCharsets

class RealtimeClient(
    private val baseUrl: String,
    private val sessionStore: SessionStore,
    private val client: OkHttpClient = OkHttpClient()
) {
    private var socket: WebSocket? = null
    private var generation: Long = 0

    @Synchronized
    fun connect(
        deviceId: String,
        onEvent: (String) -> Unit,
        onDisconnected: () -> Unit
    ) {
        val session = sessionStore.load() ?: run {
            onDisconnected()
            return
        }

        generation += 1
        val currentGeneration = generation
        socket?.cancel()

        val encodedDevice = URLEncoder.encode(
            deviceId,
            StandardCharsets.UTF_8.toString()
        )
        val websocketBase = when {
            baseUrl.startsWith("https://") -> {
                "wss://" + baseUrl.removePrefix("https://")
            }
            baseUrl.startsWith("http://") -> {
                "ws://" + baseUrl.removePrefix("http://")
            }
            else -> baseUrl
        }.trimEnd('/')

        val request = Request.Builder()
            .url("${websocketBase}/v1/events?deviceId=${encodedDevice}")
            .header(
                "Authorization",
                "${session.tokenType} ${session.accessToken}"
            )
            .build()

        socket = client.newWebSocket(
            request,
            object : WebSocketListener() {
                override fun onMessage(webSocket: WebSocket, text: String) {
                    if (!isCurrent(currentGeneration)) return
                    val type = runCatching {
                        JSONObject(text).getString("type")
                    }.getOrNull() ?: return
                    onEvent(type)
                }

                override fun onFailure(
                    webSocket: WebSocket,
                    t: Throwable,
                    response: Response?
                ) {
                    if (!markDisconnected(currentGeneration)) return
                    onDisconnected()
                }

                override fun onClosed(
                    webSocket: WebSocket,
                    code: Int,
                    reason: String
                ) {
                    if (!markDisconnected(currentGeneration)) return
                    onDisconnected()
                }
            }
        )
    }

    @Synchronized
    fun disconnect() {
        generation += 1
        socket?.close(1000, "client sign out")
        socket = null
    }

    @Synchronized
    private fun isCurrent(candidate: Long): Boolean {
        return generation == candidate
    }

    @Synchronized
    private fun markDisconnected(candidate: Long): Boolean {
        if (generation != candidate) return false
        socket = null
        return true
    }
}
