package com.armaabetancourtt.pixelgo.network

import com.armaabetancourtt.pixelgo.model.PixelDevice
import com.armaabetancourtt.pixelgo.model.TokenPair
import com.armaabetancourtt.pixelgo.model.Transfer
import com.armaabetancourtt.pixelgo.security.SessionStore
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.sync.Mutex
import kotlinx.coroutines.sync.withLock
import kotlinx.coroutines.withContext
import org.json.JSONArray
import org.json.JSONObject
import java.net.HttpURLConnection
import java.net.URL

class ApiClient(
    private val baseUrl: String,
    private val sessionStore: SessionStore
) {
    private val refreshMutex = Mutex()

    fun hasStoredSession(): Boolean = sessionStore.load() != null

    fun currentSession(): TokenPair? = sessionStore.load()

    suspend fun register(email: String, password: String) {
        val pair = publicAuthPost(
            "/v1/auth/register",
            JSONObject().put("email", email).put("password", password)
        )
        sessionStore.save(pair)
    }

    suspend fun login(email: String, password: String) {
        val pair = publicAuthPost(
            "/v1/auth/login",
            JSONObject().put("email", email).put("password", password)
        )
        sessionStore.save(pair)
    }

    fun signOut() {
        sessionStore.clear()
    }

    suspend fun health() {
        execute("GET", "/health", accessToken = null)
            .requireSuccess()
    }

    suspend fun ensureCurrentDevice(
        name: String,
        platform: String
    ): PixelDevice {
        val session = sessionStore.load() ?: throw NoSessionException()

        sessionStore.deviceId(session.userId)?.let { storedId ->
            val devices = listDevices()
            devices.firstOrNull { it.id == storedId }?.let { return it }
        }

        val idempotencyKey = sessionStore.registrationKey(session.userId)
        val response = authenticatedRequest(
            method = "POST",
            path = "/v1/devices",
            body = JSONObject()
                .put("name", name.take(120))
                .put("platform", platform)
                .toString(),
            headers = mapOf("Idempotency-Key" to idempotencyKey)
        )
        val item = JSONObject(response)
        val device = PixelDevice(
            id = item.getString("id"),
            name = item.getString("name"),
            platform = item.getString("platform")
        )
        sessionStore.saveDeviceId(session.userId, device.id)
        return device
    }

    suspend fun listDevices(): List<PixelDevice> {
        val array = JSONArray(authenticatedGet("/v1/devices"))
        return buildList {
            for (index in 0 until array.length()) {
                val item = array.getJSONObject(index)
                add(
                    PixelDevice(
                        id = item.getString("id"),
                        name = item.getString("name"),
                        platform = item.getString("platform")
                    )
                )
            }
        }
    }

    suspend fun isDeviceOnline(deviceId: String): Boolean {
        val item = JSONObject(authenticatedGet("/v1/presence/$deviceId"))
        return item.getBoolean("online")
    }

    suspend fun listTransfers(): List<Transfer> {
        val array = JSONArray(authenticatedGet("/v1/transfers"))
        return buildList {
            for (index in 0 until array.length()) {
                val item = array.getJSONObject(index)
                add(
                    Transfer(
                        id = item.getString("id"),
                        kind = item.getString("kind"),
                        status = item.getString("status"),
                        displayName = item
                            .optString("displayName")
                            .takeIf { it.isNotBlank() },
                        sizeBytes = item.getLong("sizeBytes")
                    )
                )
            }
        }
    }

    private suspend fun authenticatedGet(path: String): String {
        return authenticatedRequest("GET", path)
    }

    private suspend fun authenticatedRequest(
        method: String,
        path: String,
        body: String? = null,
        headers: Map<String, String> = emptyMap()
    ): String {
        val initial = sessionStore.load() ?: throw NoSessionException()
        val first = execute(
            method = method,
            path = path,
            body = body,
            accessToken = initial.accessToken,
            headers = headers
        )

        if (first.status != HttpURLConnection.HTTP_UNAUTHORIZED) {
            return first.requireSuccess()
        }

        try {
            refreshSingleFlight(initial.accessToken)
        } catch (error: Exception) {
            sessionStore.clear()
            throw SessionExpiredException(error)
        }

        val refreshed = sessionStore.load() ?: throw SessionExpiredException()
        return execute(
            method = method,
            path = path,
            body = body,
            accessToken = refreshed.accessToken,
            headers = headers
        ).requireSuccess()
    }

    /**
     * Refresh tokens rotate on every use. If several API calls receive 401 at
     * once, only the first coroutine is allowed to rotate the token. Later
     * callers observe that the stored access token already changed and reuse
     * the new session instead of replaying the old refresh token.
     */
    private suspend fun refreshSingleFlight(failedAccessToken: String) {
        refreshMutex.withLock {
            val current = sessionStore.load() ?: throw NoSessionException()
            if (current.accessToken != failedAccessToken) {
                return
            }

            val replacement = publicAuthPost(
                "/v1/auth/refresh",
                JSONObject().put("refreshToken", current.refreshToken)
            )
            sessionStore.save(replacement)
        }
    }

    private suspend fun publicAuthPost(
        path: String,
        body: JSONObject
    ): TokenPair {
        val response = execute(
            method = "POST",
            path = path,
            body = body.toString(),
            accessToken = null
        )
        val payload = JSONObject(response.requireSuccess())
        return TokenPair(
            userId = payload.getString("userId"),
            accessToken = payload.getString("accessToken"),
            refreshToken = payload.getString("refreshToken"),
            tokenType = payload.getString("tokenType"),
            expiresInSeconds = payload.getLong("expiresInSeconds")
        )
    }

    private suspend fun execute(
        method: String,
        path: String,
        body: String? = null,
        accessToken: String?,
        headers: Map<String, String> = emptyMap()
    ): HttpResult = withContext(Dispatchers.IO) {
        val connection = URL(
            baseUrl.trimEnd('/') + path
        ).openConnection() as HttpURLConnection

        try {
            connection.requestMethod = method
            connection.setRequestProperty("Accept", "application/json")
            connection.connectTimeout = 5_000
            connection.readTimeout = 5_000

            if (accessToken != null) {
                connection.setRequestProperty(
                    "Authorization",
                    "Bearer $accessToken"
                )
            }
            for ((key, value) in headers) {
                connection.setRequestProperty(key, value)
            }

            if (body != null) {
                val bytes = body.toByteArray(Charsets.UTF_8)
                connection.doOutput = true
                connection.setRequestProperty(
                    "Content-Type",
                    "application/json"
                )
                connection.setFixedLengthStreamingMode(bytes.size)
                connection.outputStream.use { it.write(bytes) }
            }

            val status = connection.responseCode
            val stream = if (status in 200..299) {
                connection.inputStream
            } else {
                connection.errorStream
            }
            val responseBody = stream
                ?.bufferedReader()
                ?.use { it.readText() }
                .orEmpty()

            HttpResult(status, responseBody)
        } finally {
            connection.disconnect()
        }
    }
}

private data class HttpResult(
    val status: Int,
    val body: String
) {
    fun requireSuccess(): String {
        if (status !in 200..299) {
            throw ApiException(status, body)
        }
        return body
    }
}

class ApiException(
    val statusCode: Int,
    responseBody: String = ""
) : Exception(
    if (responseBody.isBlank()) {
        "PIXEL GO API returned HTTP $statusCode"
    } else {
        "PIXEL GO API returned HTTP $statusCode: $responseBody"
    }
)

class NoSessionException : Exception("Sign in to continue.")

class SessionExpiredException(
    cause: Throwable? = null
) : Exception("Your session expired. Sign in again.", cause)
