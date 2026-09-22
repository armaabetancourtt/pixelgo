package com.armaabetancourtt.pixelgo.network

import com.armaabetancourtt.pixelgo.model.PixelDevice
import com.armaabetancourtt.pixelgo.model.Transfer
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import org.json.JSONArray
import java.net.HttpURLConnection
import java.net.URL

class ApiClient(private val baseUrl: String) {
    suspend fun health() {
        get("/health")
    }

    suspend fun listDevices(): List<PixelDevice> {
        val array = JSONArray(get("/v1/devices"))
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

    suspend fun listTransfers(): List<Transfer> {
        val array = JSONArray(get("/v1/transfers"))
        return buildList {
            for (index in 0 until array.length()) {
                val item = array.getJSONObject(index)
                add(
                    Transfer(
                        id = item.getString("id"),
                        kind = item.getString("kind"),
                        status = item.getString("status"),
                        displayName = item.optString("displayName").takeIf { it.isNotBlank() },
                        sizeBytes = item.getLong("sizeBytes")
                    )
                )
            }
        }
    }

    private suspend fun get(path: String): String = withContext(Dispatchers.IO) {
        val connection = URL(baseUrl.trimEnd('/') + path).openConnection() as HttpURLConnection
        try {
            connection.requestMethod = "GET"
            connection.setRequestProperty("Accept", "application/json")
            connection.connectTimeout = 3_000
            connection.readTimeout = 3_000

            val status = connection.responseCode
            if (status !in 200..299) {
                throw ApiException(status)
            }

            connection.inputStream.bufferedReader().use { it.readText() }
        } finally {
            connection.disconnect()
        }
    }
}

class ApiException(val statusCode: Int) : Exception("PIXEL GO API returned HTTP $statusCode")
