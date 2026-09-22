package com.armaabetancourtt.pixelgo.model

data class TokenPair(
    val userId: String,
    val accessToken: String,
    val refreshToken: String,
    val tokenType: String,
    val expiresInSeconds: Long
)

data class PixelDevice(
    val id: String,
    val name: String,
    val platform: String
)

data class Transfer(
    val id: String,
    val sourceDeviceId: String,
    val destinationDeviceId: String,
    val kind: String,
    val status: String,
    val displayName: String?,
    val contentType: String?,
    val sizeBytes: Long,
    val sha256: String,
    val uploadUrl: String?,
    val downloadUrl: String?
)

data class ReceivedTextItem(
    val id: String,
    val kind: String,
    val text: String
)
