package com.armaabetancourtt.pixelgo.model

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
