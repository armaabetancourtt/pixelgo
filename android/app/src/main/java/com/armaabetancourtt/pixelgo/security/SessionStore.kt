package com.armaabetancourtt.pixelgo.security

import android.content.Context
import android.util.Base64
import com.armaabetancourtt.pixelgo.model.TokenPair
import org.json.JSONObject

class SessionStore(
    context: Context,
    private val crypto: SecureTokenStore = SecureTokenStore()
) {
    private val preferences = context.getSharedPreferences(
        "pixelgo.session.v1",
        Context.MODE_PRIVATE
    )

    @Synchronized
    fun load(): TokenPair? {
        val ivEncoded = preferences.getString("iv", null) ?: return null
        val ciphertextEncoded = preferences.getString("ciphertext", null) ?: return null

        return try {
            val encrypted = EncryptedValue(
                iv = Base64.decode(ivEncoded, Base64.NO_WRAP),
                ciphertext = Base64.decode(ciphertextEncoded, Base64.NO_WRAP)
            )
            val plaintext = crypto.decrypt(encrypted)
            val json = JSONObject(plaintext.toString(Charsets.UTF_8))
            TokenPair(
                accessToken = json.getString("accessToken"),
                refreshToken = json.getString("refreshToken"),
                tokenType = json.getString("tokenType"),
                expiresInSeconds = json.getLong("expiresInSeconds")
            )
        } catch (_: Exception) {
            clear()
            null
        }
    }

    @Synchronized
    fun save(pair: TokenPair) {
        val payload = JSONObject()
            .put("accessToken", pair.accessToken)
            .put("refreshToken", pair.refreshToken)
            .put("tokenType", pair.tokenType)
            .put("expiresInSeconds", pair.expiresInSeconds)
            .toString()
            .toByteArray(Charsets.UTF_8)

        val encrypted = crypto.encrypt(payload)
        preferences.edit()
            .putString("iv", Base64.encodeToString(encrypted.iv, Base64.NO_WRAP))
            .putString(
                "ciphertext",
                Base64.encodeToString(encrypted.ciphertext, Base64.NO_WRAP)
            )
            .apply()
    }

    @Synchronized
    fun clear() {
        preferences.edit().clear().apply()
    }
}
