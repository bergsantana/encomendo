package br.com.encomendo.core.security

import android.security.keystore.KeyGenParameterSpec
import android.security.keystore.KeyProperties
import java.security.GeneralSecurityException
import java.security.KeyStore
import java.security.ProviderException
import javax.crypto.KeyGenerator
import javax.crypto.SecretKey

/**
 * KeystoreCipher seals secrets with an AES-256-GCM key that lives in the
 * Android Keystore and never leaves it (framing in AesGcm.android.kt).
 * [open] never throws: a key the Keystore can't load means an unreadable
 * session, never a crash on every launch.
 */
class KeystoreCipher(
    private val alias: String = DEFAULT_ALIAS,
) : SecretCipher {
    private val keyLock = Any()

    override fun seal(plain: ByteArray): ByteArray = sealWith(key(), plain)

    override fun open(sealed: ByteArray): ByteArray? =
        try {
            existingKey()?.let { openWith(it, sealed) }
        } catch (_: GeneralSecurityException) {
            // KeyStoreException, UnrecoverableKeyException: known on some devices after updates.
            null
        } catch (_: ProviderException) {
            // The Keystore daemon can fail with this RuntimeException; same outcome.
            null
        }

    // Two first-time seals at once must not each create a key under the same alias.
    private fun key(): SecretKey = synchronized(keyLock) { existingKey() ?: newKey() }

    private fun existingKey(): SecretKey? {
        val keyStore = KeyStore.getInstance(ANDROID_KEYSTORE).apply { load(null) }
        return (keyStore.getEntry(alias, null) as? KeyStore.SecretKeyEntry)?.secretKey
    }

    private fun newKey(): SecretKey {
        val spec =
            KeyGenParameterSpec
                .Builder(alias, KeyProperties.PURPOSE_ENCRYPT or KeyProperties.PURPOSE_DECRYPT)
                .setBlockModes(KeyProperties.BLOCK_MODE_GCM)
                .setEncryptionPaddings(KeyProperties.ENCRYPTION_PADDING_NONE)
                .setKeySize(KEY_BITS)
                .build()
        return KeyGenerator
            .getInstance(KeyProperties.KEY_ALGORITHM_AES, ANDROID_KEYSTORE)
            .apply { init(spec) }
            .generateKey()
    }

    private companion object {
        const val DEFAULT_ALIAS = "encomendo.session"
        const val ANDROID_KEYSTORE = "AndroidKeyStore"
        const val KEY_BITS = 256
    }
}
