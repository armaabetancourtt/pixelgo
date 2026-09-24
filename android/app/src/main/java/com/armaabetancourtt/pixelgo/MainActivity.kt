package com.armaabetancourtt.pixelgo

import android.net.Uri
import android.os.Build
import android.os.Bundle
import android.provider.OpenableColumns
import androidx.activity.ComponentActivity
import androidx.activity.compose.rememberLauncherForActivityResult
import androidx.activity.compose.setContent
import androidx.activity.result.PickVisualMediaRequest
import androidx.activity.result.contract.ActivityResultContracts
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
import androidx.compose.material3.AlertDialog
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
import androidx.compose.ui.platform.LocalContext
import androidx.compose.ui.text.font.FontWeight
import androidx.compose.ui.text.input.PasswordVisualTransformation
import androidx.compose.ui.unit.dp
import com.armaabetancourtt.pixelgo.model.PixelDevice
import com.armaabetancourtt.pixelgo.model.ReceivedTextItem
import com.armaabetancourtt.pixelgo.model.Transfer
import com.armaabetancourtt.pixelgo.network.ApiClient
import com.armaabetancourtt.pixelgo.network.RealtimeClient
import com.armaabetancourtt.pixelgo.network.SessionExpiredException
import com.armaabetancourtt.pixelgo.security.SessionStore
import com.google.firebase.FirebaseApp
import com.google.firebase.messaging.FirebaseMessaging
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.delay
import kotlinx.coroutines.launch
import kotlinx.coroutines.withContext
import java.io.File
import kotlin.math.roundToInt

class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        super.onCreate(savedInstanceState)

        val sessionStore = SessionStore(applicationContext)
        val api = ApiClient(BuildConfig.API_BASE_URL, sessionStore)
        val realtime = RealtimeClient(BuildConfig.API_BASE_URL, sessionStore)

        setContent {
            MaterialTheme(colorScheme = pixelGoColorScheme) {
                Surface(
                    modifier = Modifier.fillMaxSize(),
                    color = MaterialTheme.colorScheme.background
                ) {
                    PixelGoApp(api, realtime)
                }
            }
        }
    }
}

@Composable
private fun PixelGoApp(
    api: ApiClient,
    realtime: RealtimeClient
) {
    val scope = rememberCoroutineScope()
    val context = LocalContext.current

    var didBootstrap by remember { mutableStateOf(false) }
    var isAuthenticated by remember { mutableStateOf(false) }
    var isLoading by remember { mutableStateOf(false) }
    var errorMessage by remember { mutableStateOf<String?>(null) }

    var devices by remember { mutableStateOf<List<PixelDevice>>(emptyList()) }
    var transfers by remember { mutableStateOf<List<Transfer>>(emptyList()) }
    var receivedItems by remember {
        mutableStateOf<List<ReceivedTextItem>>(emptyList())
    }
    var onlineDeviceIds by remember { mutableStateOf<Set<String>>(emptySet()) }
    var localDeviceId by remember { mutableStateOf<String?>(null) }

    suspend fun clearAuthenticatedState(message: String? = null) {
        realtime.disconnect()
        api.signOut()
        localDeviceId = null
        isAuthenticated = false
        devices = emptyList()
        transfers = emptyList()
        receivedItems = emptyList()
        onlineDeviceIds = emptySet()
        errorMessage = message
    }

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
                    // Presence is ephemeral. Durable data should still render.
                }
            }
            onlineDeviceIds = online
            errorMessage = null
        } catch (error: SessionExpiredException) {
            clearAuthenticatedState(error.message)
        } catch (error: Exception) {
            errorMessage = error.message ?: "Could not load PIXEL GO."
        } finally {
            isLoading = false
        }
    }

    suspend fun receivePendingItems() {
        val destinationId = localDeviceId ?: return

        try {
            val incoming = api.receiveReadyTextItems(destinationId)
            if (incoming.isNotEmpty()) {
                val known = receivedItems.map { it.id }.toMutableSet()
                receivedItems = (
                    incoming.filter { known.add(it.id) } + receivedItems
                )
            }
        } catch (error: SessionExpiredException) {
            clearAuthenticatedState(error.message)
        } catch (error: Exception) {
            errorMessage = error.message ?: "Could not receive transfer."
        }
    }

    fun connectRealtime(deviceId: String) {
        realtime.connect(
            deviceId = deviceId,
            onEvent = { eventType ->
                scope.launch {
                    if (!isAuthenticated) return@launch

                    if (eventType == "transfer.ready") {
                        receivePendingItems()
                    }
                    reload()
                }
            },
            onDisconnected = {
                scope.launch {
                    delay(1_000)
                    if (!isAuthenticated || localDeviceId != deviceId) {
                        return@launch
                    }

                    // REST refreshes the access token before the next
                    // authenticated WebSocket handshake when necessary.
                    runCatching { api.listDevices() }

                    if (isAuthenticated && localDeviceId == deviceId) {
                        connectRealtime(deviceId)
                    }
                }
            }
        )
    }

    suspend fun prepareAuthenticatedSession() {
        isLoading = true
        try {
            val deviceName = listOf(Build.MANUFACTURER, Build.MODEL)
                .filter { it.isNotBlank() }
                .joinToString(" ")
                .ifBlank { "Android device" }

            val device = api.ensureCurrentDevice(
                name = deviceName,
                platform = "android"
            )
            localDeviceId = device.id

            if (FirebaseApp.getApps(context).isNotEmpty()) {
                FirebaseMessaging.getInstance().token
                    .addOnSuccessListener { token ->
                        if (token.isNotBlank()) {
                            scope.launch {
                                runCatching {
                                    api.updatePushToken(device.id, token)
                                }
                            }
                        }
                    }
            }

            // Durable state repairs anything realtime may have missed while
            // the OS suspended or killed the process.
            receivePendingItems()
            connectRealtime(device.id)
            reload()
        } catch (error: SessionExpiredException) {
            clearAuthenticatedState(error.message)
            isLoading = false
        } catch (error: Exception) {
            errorMessage = error.message ?: "Could not prepare this device."
            isLoading = false
        }
    }

    fun sendText(text: String, destinationDeviceId: String) {
        val sourceDeviceId = localDeviceId ?: return
        if (text.trim().isEmpty()) return

        scope.launch {
            isLoading = true
            try {
                api.sendText(
                    text = text,
                    sourceDeviceId = sourceDeviceId,
                    destinationDeviceId = destinationDeviceId
                )
                reload()
            } catch (error: SessionExpiredException) {
                clearAuthenticatedState(error.message)
            } catch (error: Exception) {
                errorMessage = error.message ?: "Could not send transfer."
            } finally {
                isLoading = false
            }
        }
    }

    fun sendPayload(
        payload: ByteArray,
        kind: String,
        displayName: String,
        contentType: String,
        destinationDeviceId: String
    ) {
        val sourceDeviceId = localDeviceId ?: return
        if (payload.isEmpty()) return

        scope.launch {
            isLoading = true
            try {
                api.sendPayload(
                    payload = payload,
                    kind = kind,
                    displayName = displayName,
                    contentType = contentType,
                    sourceDeviceId = sourceDeviceId,
                    destinationDeviceId = destinationDeviceId
                )
                reload()
            } catch (error: SessionExpiredException) {
                clearAuthenticatedState(error.message)
            } catch (error: Exception) {
                errorMessage = error.message ?: "Could not send transfer."
            } finally {
                isLoading = false
            }
        }
    }

    fun sendUri(
        uri: Uri,
        kind: String,
        displayName: String,
        contentType: String,
        destinationDeviceId: String
    ) {
        val sourceDeviceId = localDeviceId ?: return

        scope.launch {
            isLoading = true
            try {
                api.sendStream(
                    openStream = {
                        context.contentResolver.openInputStream(uri)
                            ?: error("Could not open selected item.")
                    },
                    kind = kind,
                    displayName = displayName,
                    contentType = contentType,
                    sourceDeviceId = sourceDeviceId,
                    destinationDeviceId = destinationDeviceId
                )
                reload()
            } catch (error: SessionExpiredException) {
                clearAuthenticatedState(error.message)
            } catch (error: Exception) {
                errorMessage = error.message ?: "Could not send transfer."
            } finally {
                isLoading = false
            }
        }
    }

    fun saveIncoming(transfer: Transfer, destinationUri: Uri) {
        scope.launch {
            isLoading = true
            var verifiedFile: File? = null

            try {
                verifiedFile = withContext(Dispatchers.IO) {
                    File.createTempFile(
                        "pixelgo-verified-",
                        ".download",
                        context.cacheDir
                    )
                }

                api.downloadPayloadTo(transfer) {
                    verifiedFile.outputStream()
                }

                withContext(Dispatchers.IO) {
                    verifiedFile.inputStream().use { input ->
                        val output = context.contentResolver
                            .openOutputStream(destinationUri)
                            ?: error("Could not open destination file.")
                        output.use {
                            input.copyTo(it, 64 * 1024)
                            it.flush()
                        }
                    }
                }

                api.completeTransfer(transfer.id)
                reload()
            } catch (error: SessionExpiredException) {
                clearAuthenticatedState(error.message)
            } catch (error: Exception) {
                errorMessage = error.message ?: "Could not save transfer."
            } finally {
                withContext(Dispatchers.IO) {
                    verifiedFile?.delete()
                }
                isLoading = false
            }
        }
    }

    LaunchedEffect(Unit) {
        isAuthenticated = api.hasStoredSession()
        didBootstrap = true
        if (isAuthenticated) {
            prepareAuthenticatedSession()
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
                            prepareAuthenticatedSession()
                        } catch (error: Exception) {
                            errorMessage =
                                error.message ?: "Authentication failed."
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
                receivedItems = receivedItems,
                onlineDeviceIds = onlineDeviceIds,
                localDeviceId = localDeviceId,
                errorMessage = errorMessage,
                isLoading = isLoading,
                onSend = ::sendText,
                onSendPayload = ::sendPayload,
                onSendUri = ::sendUri,
                onSaveIncoming = ::saveIncoming,
                onRefresh = {
                    scope.launch {
                        receivePendingItems()
                        reload()
                    }
                },
                onSignOut = {
                    scope.launch {
                        clearAuthenticatedState()
                    }
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
        PixelGoBrandHeader()
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
    receivedItems: List<ReceivedTextItem>,
    onlineDeviceIds: Set<String>,
    localDeviceId: String?,
    errorMessage: String?,
    isLoading: Boolean,
    onSend: (text: String, destinationDeviceId: String) -> Unit,
    onSendPayload: (
        payload: ByteArray,
        kind: String,
        displayName: String,
        contentType: String,
        destinationDeviceId: String
    ) -> Unit,
    onSendUri: (
        uri: Uri,
        kind: String,
        displayName: String,
        contentType: String,
        destinationDeviceId: String
    ) -> Unit,
    onSaveIncoming: (Transfer, Uri) -> Unit,
    onRefresh: () -> Unit,
    onSignOut: () -> Unit
) {
    var showingSend by remember { mutableStateOf(false) }
    var pendingSave by remember { mutableStateOf<Transfer?>(null) }
    val destinations = devices.filter { it.id != localDeviceId }
    val readyBinaryTransfers = transfers.filter {
        it.destinationDeviceId == localDeviceId &&
            it.status == "ready" &&
            it.kind in setOf("file", "photo")
    }

    val saveLauncher = rememberLauncherForActivityResult(
        ActivityResultContracts.CreateDocument("*/*")
    ) { uri ->
        val transfer = pendingSave
        pendingSave = null
        if (uri != null && transfer != null) {
            onSaveIncoming(transfer, uri)
        }
    }

    LazyColumn(
        modifier = Modifier
            .fillMaxSize()
            .padding(horizontal = 24.dp),
        verticalArrangement = Arrangement.spacedBy(14.dp)
    ) {
        item {
            Spacer(Modifier.height(28.dp))
            PixelGoBrandHeader(compact = true)
            Spacer(Modifier.height(14.dp))
            Row(
                modifier = Modifier.fillMaxWidth(),
                horizontalArrangement = Arrangement.SpaceBetween
            ) {
                Column {
                    Text(
                        "Native cross-device sharing.",
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    if (destinations.isEmpty()) {
                        Text(
                            "Sign in on another device to start sending.",
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
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
                        Row {
                            Text(device.name, fontWeight = FontWeight.SemiBold)
                            if (device.id == localDeviceId) {
                                Text(
                                    "  THIS DEVICE",
                                    style = MaterialTheme.typography.labelSmall,
                                    color = MaterialTheme.colorScheme.onSurfaceVariant
                                )
                            }
                        }
                        Text(
                            device.platform.uppercase(),
                            style = MaterialTheme.typography.labelSmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }
                    Text(
                        if (onlineDeviceIds.contains(device.id)) {
                            "Online"
                        } else {
                            "Offline"
                        },
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

        if (receivedItems.isNotEmpty()) {
            item {
                HorizontalDivider()
                Text("INBOX", style = MaterialTheme.typography.labelLarge)
            }

            items(receivedItems, key = { it.id }) { item ->
                Column(
                    Modifier
                        .fillMaxWidth()
                        .padding(vertical = 6.dp)
                ) {
                    Text(
                        when (item.kind) {
                            "link" -> "LINK"
                            "clipboard" -> "CLIPBOARD"
                            else -> "TEXT"
                        },
                        style = MaterialTheme.typography.labelSmall,
                        fontWeight = FontWeight.Bold,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                    Text(item.text)
                }
            }
        }

        if (readyBinaryTransfers.isNotEmpty()) {
            item {
                HorizontalDivider()
                Text("READY TO SAVE", style = MaterialTheme.typography.labelLarge)
            }

            items(readyBinaryTransfers, key = { "ready-" + it.id }) { transfer ->
                Row(
                    Modifier.fillMaxWidth(),
                    horizontalArrangement = Arrangement.SpaceBetween
                ) {
                    Column(
                        modifier = Modifier.weight(1f)
                    ) {
                        Text(
                            transfer.displayName
                                ?: if (transfer.kind == "photo") "Photo" else "File",
                            fontWeight = FontWeight.SemiBold
                        )
                        Text(
                            formatBytes(transfer.sizeBytes),
                            style = MaterialTheme.typography.bodySmall,
                            color = MaterialTheme.colorScheme.onSurfaceVariant
                        )
                    }

                    TextButton(
                        onClick = {
                            pendingSave = transfer
                            saveLauncher.launch(
                                transfer.displayName
                                    ?: if (transfer.kind == "photo") {
                                        "PIXEL-GO-Photo"
                                    } else {
                                        "PIXEL-GO-File"
                                    }
                            )
                        },
                        enabled = !isLoading
                    ) {
                        Text("SAVE")
                    }
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
                    detail = formatBytes(transfer.sizeBytes) +
                        " · " + transfer.status
                )
            }
        }

        item {
            Spacer(Modifier.height(16.dp))
            Button(
                onClick = { showingSend = true },
                modifier = Modifier.fillMaxWidth(),
                enabled = destinations.isNotEmpty() && !isLoading
            ) {
                Text("+   SEND")
            }
            Spacer(Modifier.height(28.dp))
        }
    }

    if (showingSend && destinations.isNotEmpty()) {
        SendDialog(
            destinations = destinations,
            onDismiss = { showingSend = false },
            onSendText = { text, destination ->
                showingSend = false
                onSend(text, destination)
            },
            onSendPayload = { payload, kind, name, contentType, destination ->
                showingSend = false
                onSendPayload(payload, kind, name, contentType, destination)
            },
            onSendUri = { uri, kind, name, contentType, destination ->
                showingSend = false
                onSendUri(uri, kind, name, contentType, destination)
            }
        )
    }
}

@Composable
private fun SendDialog(
    destinations: List<PixelDevice>,
    onDismiss: () -> Unit,
    onSendText: (String, String) -> Unit,
    onSendPayload: (ByteArray, String, String, String, String) -> Unit,
    onSendUri: (Uri, String, String, String, String) -> Unit
) {
    val context = LocalContext.current

    var selectedDeviceId by remember {
        mutableStateOf(destinations.first().id)
    }
    var text by remember { mutableStateOf("") }
    var pickerError by remember { mutableStateOf<String?>(null) }

    fun dispatchUri(uri: Uri, kind: String) {
        try {
            val contentType = context.contentResolver.getType(uri)
                ?: "application/octet-stream"
            val displayName = queryDisplayName(
                context.contentResolver,
                uri
            ) ?: if (kind == "photo") "Photo" else "File"

            onSendUri(
                uri,
                kind,
                displayName,
                contentType,
                selectedDeviceId
            )
        } catch (error: Exception) {
            pickerError = error.message ?: "Could not inspect selected item."
        }
    }

    val photoPicker = rememberLauncherForActivityResult(
        ActivityResultContracts.PickVisualMedia()
    ) { uri ->
        if (uri != null) {
            dispatchUri(uri, "photo")
        }
    }

    val filePicker = rememberLauncherForActivityResult(
        ActivityResultContracts.GetContent()
    ) { uri ->
        if (uri != null) {
            dispatchUri(uri, "file")
        }
    }

    AlertDialog(
        onDismissRequest = onDismiss,
        title = { Text("Send") },
        text = {
            Column {
                Text(
                    "TO",
                    style = MaterialTheme.typography.labelSmall,
                    fontWeight = FontWeight.Bold
                )

                destinations.forEach { device ->
                    TextButton(
                        onClick = { selectedDeviceId = device.id },
                        modifier = Modifier.fillMaxWidth()
                    ) {
                        Text(
                            (if (selectedDeviceId == device.id) "✓  " else "") +
                                device.name
                        )
                    }
                }

                Spacer(Modifier.height(8.dp))

                OutlinedTextField(
                    value = text,
                    onValueChange = { text = it },
                    label = { Text("Text or link") },
                    minLines = 3,
                    modifier = Modifier.fillMaxWidth()
                )

                TextButton(
                    onClick = {
                        onSendText(text, selectedDeviceId)
                    },
                    enabled = text.trim().isNotEmpty(),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text("SEND TEXT / LINK", fontWeight = FontWeight.Bold)
                }

                TextButton(
                    onClick = {
                        val clipboard = context.getSystemService(
                            android.content.Context.CLIPBOARD_SERVICE
                        ) as android.content.ClipboardManager
                        val clip = clipboard.primaryClip
                        val value = if (clip != null && clip.itemCount > 0) {
                            clip.getItemAt(0)
                                .coerceToText(context)
                                ?.toString()
                                ?.trim()
                                .orEmpty()
                        } else {
                            ""
                        }

                        if (value.isBlank()) {
                            pickerError = "Clipboard does not contain text."
                        } else {
                            onSendPayload(
                                value.toByteArray(Charsets.UTF_8),
                                "clipboard",
                                "Clipboard",
                                "text/plain; charset=utf-8",
                                selectedDeviceId
                            )
                        }
                    },
                    enabled = selectedDeviceId.isNotEmpty(),
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text("PASTE & SEND CLIPBOARD")
                }

                HorizontalDivider()
                Spacer(Modifier.height(4.dp))

                TextButton(
                    onClick = {
                        photoPicker.launch(
                            PickVisualMediaRequest(
                                ActivityResultContracts.PickVisualMedia.ImageOnly
                            )
                        )
                    },
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text("CHOOSE PHOTO")
                }

                TextButton(
                    onClick = { filePicker.launch("*/*") },
                    modifier = Modifier.fillMaxWidth()
                ) {
                    Text("CHOOSE FILE")
                }

                Text(
                    "Selected files are hashed and streamed directly to object storage through a short-lived signed URL.",
                    style = MaterialTheme.typography.bodySmall,
                    color = MaterialTheme.colorScheme.onSurfaceVariant
                )

                pickerError?.let {
                    Spacer(Modifier.height(6.dp))
                    Text(
                        it,
                        style = MaterialTheme.typography.bodySmall,
                        color = MaterialTheme.colorScheme.onSurfaceVariant
                    )
                }
            }
        },
        confirmButton = {},
        dismissButton = {
            TextButton(onClick = onDismiss) {
                Text("Cancel")
            }
        }
    )
}

private fun queryDisplayName(
    resolver: android.content.ContentResolver,
    uri: Uri
): String? {
    return resolver.query(
        uri,
        arrayOf(OpenableColumns.DISPLAY_NAME),
        null,
        null,
        null
    )?.use { cursor ->
        val index = cursor.getColumnIndex(OpenableColumns.DISPLAY_NAME)
        if (index >= 0 && cursor.moveToFirst()) {
            cursor.getString(index)
        } else {
            null
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
    if (bytes < 1_024) return bytes.toString() + " B"

    val kilobytes = bytes / 1_024.0
    if (kilobytes < 1_024) {
        return ((kilobytes * 10).roundToInt() / 10.0).toString() + " KB"
    }

    val megabytes = kilobytes / 1_024.0
    return ((megabytes * 10).roundToInt() / 10.0).toString() + " MB"
}
