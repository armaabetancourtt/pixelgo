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
import com.armaabetancourtt.pixelgo.network.ApiClient

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

    LaunchedEffect(Unit) {
        status = runCatching { api.health() }.fold(
            onSuccess = { "API online" },
            onFailure = { "Local API unavailable" }
        )
    }

    LazyColumn(
        modifier = Modifier.fillMaxSize().padding(horizontal = 24.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        item {
            Spacer(Modifier.height(28.dp))
            Text("PIXEL GO", style = MaterialTheme.typography.headlineLarge, fontWeight = FontWeight.Black)
            Text("Native cross-device sharing.", color = MaterialTheme.colorScheme.onSurfaceVariant)
            Spacer(Modifier.height(8.dp))
            Text(status, style = MaterialTheme.typography.labelMedium, color = MaterialTheme.colorScheme.onSurfaceVariant)
        }

        item {
            Spacer(Modifier.height(14.dp))
            Text("YOUR DEVICES", style = MaterialTheme.typography.labelLarge)
        }

        items(listOf("Armando's iPhone" to "iOS", "Pixel 10" to "ANDROID")) { (name, platform) ->
            Row(Modifier.fillMaxWidth(), horizontalArrangement = Arrangement.SpaceBetween) {
                Column {
                    Text(name, fontWeight = FontWeight.SemiBold)
                    Text(platform, style = MaterialTheme.typography.labelSmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
                }
                Text("Online", style = MaterialTheme.typography.labelMedium)
            }
        }

        item {
            HorizontalDivider()
            Text("RECENT", style = MaterialTheme.typography.labelLarge)
            TransferRow("IMG_2048.jpg", "12.4 MB · Delivered")
            TransferRow("github.com/...", "Link · Delivered")
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
    Column(Modifier.fillMaxWidth().padding(vertical = 8.dp)) {
        Text(title, fontWeight = FontWeight.SemiBold)
        Text(detail, style = MaterialTheme.typography.bodySmall, color = MaterialTheme.colorScheme.onSurfaceVariant)
    }
}
