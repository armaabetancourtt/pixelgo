package com.armaabetancourtt.pixelgo.push

import android.app.Notification
import android.app.NotificationChannel
import android.app.NotificationManager
import com.armaabetancourtt.pixelgo.BuildConfig
import com.armaabetancourtt.pixelgo.network.ApiClient
import com.armaabetancourtt.pixelgo.security.SessionStore
import com.google.firebase.messaging.FirebaseMessagingService
import com.google.firebase.messaging.RemoteMessage
import kotlinx.coroutines.CoroutineScope
import kotlinx.coroutines.Dispatchers
import kotlinx.coroutines.SupervisorJob
import kotlinx.coroutines.launch

class PixelGoMessagingService : FirebaseMessagingService() {
    private val scope = CoroutineScope(SupervisorJob() + Dispatchers.IO)

    override fun onNewToken(token: String) {
        super.onNewToken(token)

        scope.launch {
            val store = SessionStore(applicationContext)
            val session = store.load() ?: return@launch
            val deviceId = store.deviceId(session.userId) ?: return@launch

            runCatching {
                ApiClient(BuildConfig.API_BASE_URL, store)
                    .updatePushToken(deviceId, token)
            }
        }
    }

    override fun onMessageReceived(message: RemoteMessage) {
        super.onMessageReceived(message)

        val title = message.notification?.title ?: "PIXEL GO"
        val body = message.notification?.body ?: "Something is ready on this device."

        val manager = getSystemService(NotificationManager::class.java)
        val channelId = "pixelgo.transfers"
        manager.createNotificationChannel(
            NotificationChannel(
                channelId,
                "Transfers",
                NotificationManager.IMPORTANCE_DEFAULT
            )
        )

        val notification = Notification.Builder(this, channelId)
            .setSmallIcon(android.R.drawable.stat_sys_download_done)
            .setContentTitle(title)
            .setContentText(body)
            .setAutoCancel(true)
            .build()

        manager.notify(message.messageId?.hashCode() ?: body.hashCode(), notification)
    }
}
