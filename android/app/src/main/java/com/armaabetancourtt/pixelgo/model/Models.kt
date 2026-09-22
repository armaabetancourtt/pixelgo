package com.armaabetancourtt.pixelgo.model

data class TokenPair(
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
    val kind: String,
    val status: String,
    val displayName: String?,
    val sizeBytes: Long
)
