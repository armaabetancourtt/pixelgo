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
import androidx.compose.material3.CircularProgressIndicator
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
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import com.armaabetancourtt.pixelgo.model.PixelDevice
import com.armaabetancourtt.pixelgo.model.Transfer
import com.armaabetancourtt.pixelgo.network.ApiClient
import com.armaabetancourtt.pixelgo.network.SessionExpiredException
import com.armaabetancourtt.pixelgo.security.SessionStore
import kotlinx.coroutines.launch
import kotlin.math.roundToInt

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val sessionStore = SessionStore(applicationContext)
        val api = ApiClient(BuildConfig.API_BASE_URL, sessionStore)

        setContent {
            MaterialTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    PixelGoApp(api)
                }
            }
        }
    }
}

@Composable
private fun PixelGoApp(api: ApiClient) {
    val scope = rememberCoroutineScope()

    var didBootstrap by remember { mutableStateOf(false) }
    var isAuthenticated by remember { mutableStateOf(false) }
    var isLoading by remember { mutableStateOf(false) }
    var errorMessage by remember { mutableStateOf<String?>(null) }

    var devices by remember { mutableStateOf<List<PixelDevice>>(emptyList()) }
    var transfers by remember { mutableStateOf<List<Transfer>>(emptyList()) }
    var onlineDeviceIds by remember { mutableStateOf<Set<String>>(emptySet()) }

    suspend fun reload() {
        if (!isAuthenticated) return

        isLoading = true
        try {
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
                    // Presence is ephemeral. A temporary lookup failure should
                    // not hide the durable device list.
                }
            }
            onlineDeviceIds = online
            errorMessage = null
        } catch (error: SessionExpiredException) {
            api.signOut()
            isAuthenticated = false
            devices = emptyList()
            transfers = emptyList()
            onlineDeviceIds = emptySet()
            errorMessage = error.message
        } catch (error: Exception) {
            errorMessage = error.message ?: "Could not load PIXEL GO."
        } finally {
            isLoading = false
        }
    }

    LaunchedEffect(Unit) {
        isAuthenticated = api.hasStoredSession()
        didBootstrap = true
        if (isAuthenticated) {
            reload()
        }
    }

    when {
        !didBootstrap -> {
            Column(
                modifier = Modifier
                    .fillMaxSize()
                    .padding(28.dp),
                verticalArrangement = Arrangement.Center
            ) {
                CircularProgressIndicator()
                Spacer(Modifier.height(16.dp))
                Text("Opening PIXEL GO…")
            }
        }

        !isAuthenticated -> {
            AuthScreen(
                isLoading = isLoading,
                errorMessage = errorMessage,
                onSubmit = { email, password, createAccount ->
                    scope.launch {
                        isLoading = true
                        errorMessage = null
                        try {
                            if (createAccount) {
                                api.register(email, password)
                            } else {
                                api.login(email, password)
                            }
                            isAuthenticated = true
                            reload()
                        } catch (error: Exception) {
                            errorMessage = error.message ?: "Authentication failed."
                            isLoading = false
                        }
                    }
                }
            )
        }

        else -> {
            HomeScreen(
                devices = devices,
                transfers = transfers,
                onlineDeviceIds = onlineDeviceIds,
                errorMessage = errorMessage,
                isLoading = isLoading,
                onRefresh = { scope.launch { reload() } },
                onSignOut = {
                    api.signOut()
                    isAuthenticated = false
                    devices = emptyList()
                    transfers = emptyList()
                    onlineDeviceIds = emptySet()
                    errorMessage = null
                }
            )
        }
    }
}

@Composable
private fun AuthScreen(
    isLoading: Boolean,
    errorMessage: String?,
    onSubmit: (email: String, password: String, createAccount: Boolean) -> Unit
) {
    var email by remember { mutableStateOf("") }
    var password by remember { mutableStateOf("") }
    var createAccount by remember { mutableStateOf(false) }

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
            style = MaterialTheme.typography.titleMedium,
            color = MaterialTheme.colorScheme.onSurfaceVariant
        )

        Spacer(Modifier.height(28.dp))

        OutlinedTextField(
            value = email,
            onValueChange = { email = it },
            modifier = Modifier.fillMaxWidth(),
            label = { Text("Email") },
            singleLine = true,
            enabled = !isLoading
        )

        Spacer(Modifier.height(12.dp))

        OutlinedTextField(
            value = password,
            onValueChange = { password = it },
            modifier = Modifier.fillMaxWidth(),
            label = { Text("Password") },
            singleLine = true,
            enabled = !isLoading,
            visualTransformation = PasswordVisualTransformation()
        )

        Spacer(Modifier.height(18.dp))

        Button(
            onClick = { onSubmit(email.trim(), password, createAccount) },
            modifier = Modifier.fillMaxWidth(),
            enabled = email.isNotBlank() && password.isNotBlank() && !isLoading
        ) {
            if (isLoading) {
                CircularProgressIndicator()
            } else {
                Text(if (createAccount) "CREATE ACCOUNT" else "SIGN IN")
            }
        }

        TextButton(
            onClick = { createAccount = !createAccount },
            modifier = Modifier.fillMaxWidth(),
            enabled = !isLoading
        ) {
            Text(
                if (createAccount) {
                    "Already have an account? Sign in"
                } else {
                    "New to PIXEL GO? Create account"
                }
            )
        }

        if (errorMessage != null) {
            Spacer(Modifier.height(8.dp))
            Text(
                errorMessage,
                color = MaterialTheme.colorScheme.onSurfaceVariant,
                style = MaterialTheme.typography.bodySmall
            )
        }
    }
}

@Composable
private fun HomeScreen(
    devices: List<PixelDevice>,
    transfers: List<Transfer>,
    onlineDeviceIds: Set<String>,
    errorMessage: String?,
    isLoading: Boolean,
    onRefresh: () -> Unit,
    onSignOut: () -> Unit
) {
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

            if (errorMessage != null) {
                Spacer(Modifier.height(8.dp))
                Text(
                    errorMessage,
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )
            }

            TextButton(
                onClick = onRefresh,
                enabled = !isLoading
            ) {
                Text(if (isLoading) "Refreshing…" else "Refresh")
            }
        }

        item {
            Spacer(Modifier.height(8.dp))
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
