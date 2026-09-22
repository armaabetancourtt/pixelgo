package com.armaabetancourtt.pixelgo

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.Spacer
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.height
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.lazy.LazyColumn
import androidx.compose.foundation.lazy.items
import androidx.compose.material3.Button
import androidx.compose.material3.HorizontalDivider
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.unit.dp
import com.armaabetancourtt.pixelgo.model.PixelDevice
import com.armaabetancourtt.pixelgo.model.Transfer
import com.armaabetancourtt.pixelgo.network.ApiClient
import kotlin.math.roundToInt

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    PixelGoHome()
                }
            }
        }
    }
}

@Composable
private fun PixelGoHome() {
    val api = remember { ApiClient(BuildConfig.API_BASE_URL) }
    var status by remember { mutableStateOf("Connecting…") }
    var devices by remember { mutableStateOf<List<PixelDevice>>(emptyList()) }
    var transfers by remember { mutableStateOf<List<Transfer>>(emptyList()) }

    LaunchedEffect(Unit) {
        runCatching {
            api.health()
            devices = api.listDevices()
            transfers = api.listTransfers()
        }.fold(
            onSuccess = { status = "API online" },
            onFailure = { status = "Local API unavailable" }
        )
    }

    LazyColumn(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 24.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        item {
            Spacer(Modifier.height(28.dp))
            Text(
                "PIXEL GO",
                style = MaterialTheme.typography.headlineLarge,
                fontWeight = FontWeight.Black
            )
            Text(
                "Native cross-device sharing.",
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
            Spacer(Modifier.height(8.dp))
            Text(
                status,
                style = MaterialTheme.typography.labelMedium,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }

        item {
            Spacer(Modifier.height(14.dp))
            Text("YOUR DEVICES", style = MaterialTheme.typography.labelLarge)
        }

        if (devices.isEmpty()) {
            item {
                Text(
                    "No devices registered yet",
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        } else {
            items(devices, key = { it.id }) { device ->
                Row(
                    Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Column {
                        Text(device.name, fontWeight = FontWeight.SemiBold)
                        Text(
                            device.platform.uppercase(),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                    Text(
                        if (device.online) "Online" else "Offline",
                        style = MaterialTheme.typography.labelMedium,
                        color = if (device.online) {
                            MaterialTheme.colorScheme.primary
                        } else {
                            MaterialTheme.colorScheme.onSurfaceVariant
                        }
                    )
                }
            }
        }

        item {
            HorizontalDivider()
            Text("RECENT", style = MaterialTheme.typography.labelLarge)
        }

        if (transfers.isEmpty()) {
            item {
                Text(
                    "Nothing sent yet",
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }
        } else {
            items(transfers, key = { it.id }) { transfer ->
                TransferRow(
                    title = transfer.displayName ?: transfer.kind.replaceFirstChar { it.uppercase() },
                    detail = "${formatBytes(transfer.sizeBytes)} · ${transfer.status}"
                )
            }
        }

        item {
            Spacer(Modifier.height(16.dp))
            Button(onClick = {}, modifier = Modifier.fillMaxWidth()) {
                Text("+   SEND")
            }
            Spacer(Modifier.height(28.dp))
        }
    }
}

@Composable
private fun TransferRow(title: String, detail: String) {
    Column(
        Modifier
            .fillMaxWidth()
            .padding(vertical = 8.dp)
    ) {
        Text(title, fontWeight = FontWeight.SemiBold)
        Text(
            detail,
            style = MaterialTheme.typography.bodySmall,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )
    }
}

private fun formatBytes(bytes: Long): String {
    if (bytes < 1_024) return "$bytes B"
    val kilobytes = bytes / 1_024.0
    if (kilobytes < 1_024) return "${(kilobytes * 10).roundToInt() / 10.0} KB"
    val megabytes = kilobytes / 1_024.0
    return "${(megabytes * 10).roundToInt() / 10.0} MB"
}
