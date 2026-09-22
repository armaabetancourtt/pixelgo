package com.armaabetancourtt.pixelgo.network

import com.armaabetancourtt.pixelgo.model.PixelDevice
import com.armaabetancourtt.pixelgo.model.ReceivedTextItem
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
import java.security.MessageDigest
import java.util.UUID

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

    suspend fun sendText(
        text: String,
        sourceDeviceId: String,
        destinationDeviceId: String
    ): Transfer {
        val kind = inferTextKind(text)
        return sendPayload(
            payload = text.toByteArray(Charsets.UTF_8),
            kind = kind,
            displayName = if (kind == "link") "Link" else "Text",
            contentType = "text/plain; charset=utf-8",
            sourceDeviceId = sourceDeviceId,
            destinationDeviceId = destinationDeviceId
        )
    }

    suspend fun sendPayload(
        payload: ByteArray,
        kind: String,
        displayName: String,
        contentType: String,
        sourceDeviceId: String,
        destinationDeviceId: String
    ): Transfer {
        require(kind in setOf("file", "photo", "link", "text", "clipboard")) {
            "Unsupported transfer kind."
        }

        val checksum = sha256Hex(payload)
        val createdBody = JSONObject()
            .put("sourceDeviceId", sourceDeviceId)
            .put("destinationDeviceId", destinationDeviceId)
            .put("kind", kind)
            .put("displayName", displayName.take(255))
            .put("contentType", contentType.take(255))
            .put("sizeBytes", payload.size)
            .put("sha256", checksum)
            .toString()

        val createKey = "transfer-create-" +
            UUID.randomUUID().toString().lowercase()

        val created = parseTransfer(
            JSONObject(
                authenticatedRequest(
                    method = "POST",
                    path = "/v1/transfers",
                    body = createdBody,
                    headers = mapOf("Idempotency-Key" to createKey)
                )
            )
        )

        val uploadUrl = created.uploadUrl ?: throw InvalidTransferException(
            "Server did not return an upload URL."
        )
        uploadSigned(
            url = uploadUrl,
            payload = payload,
            contentType = contentType,
            checksum = checksum
        )

        return parseTransfer(
            JSONObject(
                authenticatedRequest(
                    method = "POST",
                    path = "/v1/transfers/" + created.id + "/uploaded",
                    headers = mapOf(
                        "Idempotency-Key" to
                            "transfer-uploaded-" + created.id
                    )
                )
            )
        )
    }

    suspend fun receiveReadyTextItems(
        destinationDeviceId: String
    ): List<ReceivedTextItem> {
        val received = mutableListOf<ReceivedTextItem>()

        for (transfer in listTransfers()) {
            if (
                transfer.destinationDeviceId != destinationDeviceId ||
                transfer.status != "ready" ||
                transfer.kind !in setOf("text", "link", "clipboard")
            ) {
                continue
            }

            val payload = downloadPayload(transfer)
            val text = payload.toString(Charsets.UTF_8)
            completeTransfer(transfer.id)

            received += ReceivedTextItem(
                id = transfer.id,
                kind = transfer.kind,
                text = text
            )
        }

        return received
    }

    suspend fun downloadPayload(transfer: Transfer): ByteArray {
        val downloadUrl = transfer.downloadUrl ?: throw InvalidTransferException(
            "Server did not return a download URL."
        )
        val payload = downloadSigned(downloadUrl)

        if (payload.size.toLong() != transfer.sizeBytes) {
            throw ChecksumMismatchException()
        }
        if (!sha256Hex(payload).equals(transfer.sha256, ignoreCase = true)) {
            throw ChecksumMismatchException()
        }
        return payload
    }

    suspend fun completeTransfer(transferId: String): Transfer {
        return parseTransfer(
            JSONObject(
                authenticatedRequest(
                    method = "POST",
                    path = "/v1/transfers/" + transferId + "/complete",
                    headers = mapOf(
                        "Idempotency-Key" to
                            "transfer-complete-" + transferId
                    )
                )
            )
        )
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
        val item = JSONObject(authenticatedGet("/v1/presence/" + deviceId))
        return item.getBoolean("online")
    }

    suspend fun listTransfers(): List<Transfer> {
        val array = JSONArray(authenticatedGet("/v1/transfers"))
        return buildList {
            for (index in 0 until array.length()) {
                add(parseTransfer(array.getJSONObject(index)))
            }
        }
    }

    private fun parseTransfer(item: JSONObject): Transfer {
        return Transfer(
            id = item.getString("id"),
            sourceDeviceId = item.getString("sourceDeviceId"),
            destinationDeviceId = item.getString("destinationDeviceId"),
            kind = item.getString("kind"),
            status = item.getString("status"),
            displayName = item.optString("displayName")
                .takeIf { it.isNotBlank() },
            contentType = item.optString("contentType")
                .takeIf { it.isNotBlank() },
            sizeBytes = item.getLong("sizeBytes"),
            sha256 = item.getString("sha256"),
            uploadUrl = item.optString("uploadUrl")
                .takeIf { it.isNotBlank() },
            downloadUrl = item.optString("downloadUrl")
                .takeIf { it.isNotBlank() }
        )
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

    private suspend fun uploadSigned(
        url: String,
        payload: ByteArray,
        contentType: String,
        checksum: String
    ) = withContext(Dispatchers.IO) {
        val connection = URL(url).openConnection() as HttpURLConnection
        try {
            connection.requestMethod = "PUT"
            connection.doOutput = true
            connection.connectTimeout = 5_000
            connection.readTimeout = 5_000
            connection.setRequestProperty("Content-Type", contentType)
            connection.setRequestProperty("X-Amz-Meta-Sha256", checksum)
            connection.setFixedLengthStreamingMode(payload.size)
            connection.outputStream.use { it.write(payload) }

            val status = connection.responseCode
            if (status !in 200..299) {
                throw ApiException(status, readResponse(connection, status))
            }
        } finally {
            connection.disconnect()
        }
    }

    private suspend fun downloadSigned(url: String): ByteArray =
        withContext(Dispatchers.IO) {
            val connection = URL(url).openConnection() as HttpURLConnection
            try {
                connection.requestMethod = "GET"
                connection.connectTimeout = 5_000
                connection.readTimeout = 5_000
                val status = connection.responseCode
                if (status !in 200..299) {
                    throw ApiException(status, readResponse(connection, status))
                }
                connection.inputStream.use { it.readBytes() }
            } finally {
                connection.disconnect()
            }
        }

    private fun inferTextKind(value: String): String {
        val normalized = value.trim()
        return if (
            normalized.startsWith("https://", ignoreCase = true) ||
            normalized.startsWith("http://", ignoreCase = true)
        ) {
            "link"
        } else {
            "text"
        }
    }

    private fun sha256Hex(payload: ByteArray): String {
        return MessageDigest.getInstance("SHA-256")
            .digest(payload)
            .joinToString("") { byte -> "%02x".format(byte) }
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
                    "Bearer " + accessToken
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
            val responseBody = readResponse(connection, status)
            HttpResult(status, responseBody)
        } finally {
            connection.disconnect()
        }
    }

    private fun readResponse(
        connection: HttpURLConnection,
        status: Int
    ): String {
        val stream = if (status in 200..299) {
            connection.inputStream
        } else {
            connection.errorStream
        }
        return stream
            ?.bufferedReader()
            ?.use { it.readText() }
            .orEmpty()
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
        "PIXEL GO API returned HTTP " + statusCode
    } else {
        "PIXEL GO API returned HTTP " + statusCode + ": " + responseBody
    }
)

class NoSessionException : Exception("Sign in to continue.")

class SessionExpiredException(
    cause: Throwable? = null
) : Exception("Your session expired. Sign in again.", cause)

class ChecksumMismatchException :
    Exception("The received payload failed SHA-256 verification.")

class InvalidTransferException(message: String) : Exception(message)
