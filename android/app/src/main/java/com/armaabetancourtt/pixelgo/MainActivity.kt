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
import androidx.compose.material3.OutlinedTextField
import androidx.compose.material3.Surface
import androidx.compose.material3.Text
import androidx.compose.material3.TextButton
import androidx.compose.runtime.Composable
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.mutableStateOf
import androidx.compose.runtime.remember
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.runtime.setValue
import androidx.compose.ui.Modifier
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import com.armaabetancourtt.pixelgo.model.PixelDevice
import com.armaabetancourtt.pixelgo.model.Transfer
import com.armaabetancourtt.pixelgo.network.ApiClient
import com.armaabetancourtt.pixelgo.network.SessionExpiredException
import com.armaabetancourtt.pixelgo.network.SessionRequiredException
import com.armaabetancourtt.pixelgo.security.SecureTokenStore
import kotlinx.coroutines.launch
import kotlin.math.roundToInt

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)
        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    PixelGoRoot()
                }
            }
        }
    }
}

@Composable
private fun PixelGoRoot() {
    val context = LocalContext.current.applicationContext
    val tokenStore = remember { SecureTokenStore(context) }
    val api = remember { ApiClient(BuildConfig.API_BASE_URL, tokenStore) }
    var authenticated by remember { mutableStateOf(api.hasStoredSession()) }

    if (authenticated) {
        PixelGoHome(
            api = api,
            onSignOut = {
                api.signOut()
                authenticated = false
            },
            onSessionExpired = {
                api.signOut()
                authenticated = false
            }
        )
    } else {
        AuthScreen(
            api = api,
            onAuthenticated = { authenticated = true }
        )
    }
}

@Composable
private fun AuthScreen(
    api: ApiClient,
    onAuthenticated: () -> Unit
) {
    val scope = rememberCoroutineScope()
    var email by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var createAccount by remember { mutableStateOf(false) }
    var loading by remember { mutableStateOf(false) }
    var error by remember { mutableStateOf<String?>(null) }

    Column(
        modifier = Modifier
            .fillMaxSize()
            .padding(28.dp),
        verticalArrangement = Arrangement.Center
    ) {
        Text(
            "PIXEL GO",
            style = MaterialTheme.typography.headlineLarge,
            fontWeight = FontWeight.Black
        )
        Text(
            "Your devices. One private transfer space.",
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )

        Spacer(Modifier.height(24.dp))

        OutlinedTextField(
            value = email,
            onValueChange = { email = it },
            label = { Text("Email") },
            singleLine = true,
            modifier = Modifier.fillMaxWidth()
        )

        Spacer(Modifier.height(12.dp))

        OutlinedTextField(
            value = password,
            onValueChange = { password = it },
            label = { Text("Password") },
            singleLine = true,
            visualTransformation = PasswordVisualTransformation(),
            modifier = Modifier.fillMaxWidth()
        )

        Spacer(Modifier.height(18.dp))

        Button(
            onClick = {
                scope.launch {
                    loading = true
                    error = null

                    runCatching {
                        if (createAccount) {
                            api.register(email, password)
                        } else {
                            api.login(email, password)
                        }
                    }.fold(
                        onSuccess = { onAuthenticated() },
                        onFailure = {
                            error = it.message ?: "Authentication failed."
                        }
                    )
                    loading = false
                }
            },
            enabled = email.isNotBlank() &&
                password.isNotBlank() &&
                !loading,
            modifier = Modifier.fillMaxWidth()
        ) {
            Text(
                when {
                    loading -> "CONNECTING…"
                    createAccount -> "CREATE ACCOUNT"
                    else -> "SIGN IN"
                }
            )
        }

        TextButton(
            onClick = {
                createAccount = !createAccount
                error = null
            },
            modifier = Modifier.fillMaxWidth()
        ) {
            Text(
                if (createAccount) {
                    "Already have an account? Sign in"
                } else {
                    "New to PIXEL GO? Create account"
                }
            )
        }

        error?.let {
            Spacer(Modifier.height(8.dp))
            Text(
                it,
                style = MaterialTheme.typography.bodySmall,
                color = MaterialTheme.colorScheme.onSurfaceVariant
            )
        }
    }
}

@Composable
private fun PixelGoHome(
    api: ApiClient,
    onSignOut: () -> Unit,
    onSessionExpired: () -> Unit
) {
    var status by remember { mutableStateOf("Connecting…") }
    var devices by remember { mutableStateOf<List<PixelDevice>>(emptyList()) }
    var transfers by remember { mutableStateOf<List<Transfer>>(emptyList()) }
    var onlineDeviceIds by remember { mutableStateOf<Set<String>>(emptySet()) }

    LaunchedEffect(Unit) {
        runCatching {
            api.health()
            val loadedDevices = api.listDevices()
            devices = loadedDevices
            transfers = api.listTransfers()

            val online = mutableSetOf<String>()
            for (device in loadedDevices) {
                try {
                    if (api.isDeviceOnline(device.id)) {
                        online += device.id
                    }
                } catch (_: Exception) {
                    // Presence is ephemeral. Durable data should still render.
                }
            }
            onlineDeviceIds = online
        }.fold(
            onSuccess = { status = "API online" },
            onFailure = { error ->
                when (error) {
                    is SessionExpiredException,
                    is SessionRequiredException -> onSessionExpired()
                    else -> status = "API unavailable"
                }
            }
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
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Column {
                    Text(
                        "PIXEL GO",
                        style = MaterialTheme.typography.headlineLarge,
                        fontWeight = FontWeight.Black
                    )
                    Text(
                        "Native cross-device sharing.",
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }

                TextButton(onClick = onSignOut) {
                    Text("Sign out")
                }
            }

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
                        if (onlineDeviceIds.contains(device.id)) "Online" else "Offline",
                        style = MaterialTheme.typography.labelMedium,
                        color = if (onlineDeviceIds.contains(device.id)) {
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
                    title = transfer.displayName
                        ?: transfer.kind.replaceFirstChar { it.uppercase() },
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
    if (kilobytes < 1_024) {
        return "${(kilobytes * 10).roundToInt() / 10.0} KB"
    }
    val megabytes = kilobytes / 1_024.0
    return "${(megabytes * 10).roundToInt() / 10.0} MB"
}
