package com.armaabetancourtt.pixelgo.network

import com.armaabetancourtt.pixelgo.model.PixelDevice
import com.armaabetancourtt.pixelgo.model.TokenPair
import com.armaabetancourtt.pixelgo.model.Transfer
import com.armaabetancourtt.pixelgo.security.SecureTokenStore
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
    private val tokenStore: SecureTokenStore
) {
    private val refreshMutex = Mutex()

    fun hasStoredSession(): Boolean = tokenStore.load() != null

    fun signOut() {
        tokenStore.clear()
    }

    suspend fun health() {
        val result = execute(method = "GET", path = "/health")
        requireSuccess(result)
    }

    suspend fun register(email: String, password: String) {
        authenticate("/v1/auth/register", email, password)
    }

    suspend fun login(email: String, password: String) {
        authenticate("/v1/auth/login", email, password)
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

    private suspend fun authenticate(
        path: String,
        email: String,
        password: String
    ) {
        val body = JSONObject()
            .put("email", email)
            .put("password", password)
            .toString()

        val result = execute(
            method = "POST",
            path = path,
            body = body
        )
        requireSuccess(result)
        tokenStore.save(parseTokenPair(result.body))
    }

    private suspend fun authenticatedGet(path: String): String {
        val initial = tokenStore.load() ?: throw SessionRequiredException()
        var result = execute(
            method = "GET",
            path = path,
            accessToken = initial.accessToken,
            tokenType = initial.tokenType
        )

        if (result.status == HttpURLConnection.HTTP_UNAUTHORIZED) {
            val refreshed = refreshSession(initial.accessToken)
            result = execute(
                method = "GET",
                path = path,
                accessToken = refreshed.accessToken,
                tokenType = refreshed.tokenType
            )
        }

        requireSuccess(result)
        return result.body
    }

    private suspend fun refreshSession(accessTokenUsed: String): TokenPair {
        return refreshMutex.withLock {
            val current = tokenStore.load() ?: throw SessionRequiredException()

            // Another request may have refreshed while this caller waited.
            if (current.accessToken != accessTokenUsed) {
                return@withLock current
            }

            val body = JSONObject()
                .put("refreshToken", current.refreshToken)
                .toString()
            val result = execute(
                method = "POST",
                path = "/v1/auth/refresh",
                body = body
            )

            if (result.status !in 200..299) {
                tokenStore.clear()
                throw SessionExpiredException()
            }

            val rotated = parseTokenPair(result.body)
            tokenStore.save(rotated)
            rotated
        }
    }

    private fun parseTokenPair(raw: String): TokenPair {
        val json = JSONObject(raw)
        return TokenPair(
            accessToken = json.getString("accessToken"),
            refreshToken = json.getString("refreshToken"),
            tokenType = json.getString("tokenType"),
            expiresInSeconds = json.getLong("expiresInSeconds")
        )
    }

    private fun requireSuccess(result: HttpResult) {
        if (result.status !in 200..299) {
            throw ApiException(result.status, result.body)
        }
    }

    private suspend fun execute(
        method: String,
        path: String,
        body: String? = null,
        accessToken: String? = null,
        tokenType: String = "Bearer"
    ): HttpResult = withContext(Dispatchers.IO) {
        val connection = URL(
            baseUrl.trimEnd('/') + path
        ).openConnection() as HttpURLConnection

        try {
            connection.requestMethod = method
            connection.setRequestProperty("Accept", "application/json")
            connection.connectTimeout = 3_000
            connection.readTimeout = 3_000

            if (accessToken != null) {
                connection.setRequestProperty(
                    "Authorization",
                    "$tokenType $accessToken"
                )
            }

            if (body != null) {
                connection.doOutput = true
                connection.setRequestProperty(
                    "Content-Type",
                    "application/json"
                )
                connection.outputStream.use {
                    it.write(body.encodeToByteArray())
                }
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
)

class ApiException(
    val statusCode: Int,
    val responseBody: String = ""
) : Exception("PIXEL GO API returned HTTP $statusCode")

class SessionRequiredException : Exception("Sign in to continue.")
class SessionExpiredException : Exception("Your session expired. Sign in again.")
