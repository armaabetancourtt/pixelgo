package com.armaabetancourtt.pixelgo.storage

import androidx.room.Entity
import androidx.room.PrimaryKey

@Entity(tableName = "transfers")
data class TransferEntity(
    @PrimaryKey val id: String,
    val displayName: String?,
    val kind: String,
    val status: String,
    val sizeBytes: Long,
    val sha256: String,
    val updatedAtEpochMs: Long
)
