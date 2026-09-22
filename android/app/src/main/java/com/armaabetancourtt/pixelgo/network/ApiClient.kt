package com.armaabetancourtt.pixelgo.network

import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.withContext
import java.net.HttpURLConnection
import java.net.URL

class ApiClient(private val baseUrl: String) {
    suspend fun health(): String = withContext(Dispatchers.IO) {
        val connection = URL(baseUrl.trimEnd('/') + "/health").openConnection() as HttpURLConnection
        try {
            connection.requestMethod = "GET"
            connection.setRequestProperty("Accept", "application/json")
            connection.connectTimeout = 3_000
            connection.readTimeout = 3_000
            val status = connection.responseCode
            require(status in 200..299) { "HTTP $status" }
            connection.inputStream.bufferedReader().use { it.readText() }
        } finally {
            connection.disconnect()
        }
    }
}
