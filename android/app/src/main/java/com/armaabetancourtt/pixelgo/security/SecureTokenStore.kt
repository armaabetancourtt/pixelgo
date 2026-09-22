package com.armaabetancourtt.pixelgo.security

import android.content.Context
import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import android.util.Base64
import com.armaabetancourtt.pixelgo.model.TokenPair
import org.json.JSONObject
import java.security.KeyStore
import javax.crypto.Cipher
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

class SecureTokenStore(context: Context) {
    private val alias = "pixelgo.session"
    private val keyStore = KeyStore.getInstance("AndroidKeyStore").apply { load(null) }
    private val preferences = context.getSharedPreferences(
        "pixelgo.secure.session",
        Context.MODE_PRIVATE
    )

    @Synchronized
    fun save(pair: TokenPair) {
        val json = JSONObject()
            .put("accessToken", pair.accessToken)
            .put("refreshToken", pair.refreshToken)
            .put("tokenType", pair.tokenType)
            .put("expiresInSeconds", pair.expiresInSeconds)
            .toString()
            .encodeToByteArray()

        val encrypted = encrypt(json)
        preferences.edit()
            .putString("iv", Base64.encodeToString(encrypted.iv, Base64.NO_WRAP))
            .putString(
                "ciphertext",
                Base64.encodeToString(encrypted.ciphertext, Base64.NO_WRAP)
            )
            .apply()
    }

    @Synchronized
    fun load(): TokenPair? {
        val ivEncoded = preferences.getString("iv", null) ?: return null
        val ciphertextEncoded = preferences.getString("ciphertext", null) ?: return null

        return runCatching {
            val plaintext = decrypt(
                EncryptedValue(
                    iv = Base64.decode(ivEncoded, Base64.NO_WRAP),
                    ciphertext = Base64.decode(ciphertextEncoded, Base64.NO_WRAP)
                )
            )
            val json = JSONObject(plaintext.decodeToString())
            TokenPair(
                accessToken = json.getString("accessToken"),
                refreshToken = json.getString("refreshToken"),
                tokenType = json.getString("tokenType"),
                expiresInSeconds = json.getLong("expiresInSeconds")
            )
        }.getOrElse {
            clear()
            null
        }
    }

    @Synchronized
    fun clear() {
        preferences.edit().clear().apply()
    }

    private fun encrypt(value: ByteArray): EncryptedValue {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(Cipher.ENCRYPT_MODE, getOrCreateKey())
        return EncryptedValue(cipher.iv, cipher.doFinal(value))
    }

    private fun decrypt(value: EncryptedValue): ByteArray {
        val cipher = Cipher.getInstance("AES/GCM/NoPadding")
        cipher.init(
            Cipher.DECRYPT_MODE,
            getOrCreateKey(),
            GCMParameterSpec(128, value.iv)
        )
        return cipher.doFinal(value.ciphertext)
    }

    private fun getOrCreateKey(): SecretKey {
        (keyStore.getKey(alias, null) as? SecretKey)?.let { return it }

        val generator = KeyGenerator.getInstance(
            KeyProperties.KEY_ALGORITHM_AES,
            "AndroidKeyStore"
        )
        generator.init(
            KeyGenParameterSpec.Builder(
                alias,
                KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT
            )
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .build()
        )
        return generator.generateKey()
    }
}

private data class EncryptedValue(
    val iv: ByteArray,
    val ciphertext: ByteArray
)
