package br.com.encomendo.core.security

import java.security.GeneralSecurityException
import javax.crypto.Cipher
import javax.crypto.SecretKey
import javax.crypto.spec.GCMParameterSpec

/** The IV length GCM uses; sealed bytes start with it. */
internal const val GCM_IV_BYTES = 12

private const val GCM_TAG_BITS = 128
private const val TRANSFORMATION = "AES/GCM/NoPadding"

/** Returns a fresh random IV followed by [plain] encrypted and authenticated with [key]. */
internal fun sealWith(
    key: SecretKey,
    plain: ByteArray,
): ByteArray {
    val cipher = Cipher.getInstance(TRANSFORMATION)
    cipher.init(Cipher.ENCRYPT_MODE, key)
    return cipher.iv + cipher.doFinal(plain)
}

/** Returns the plain bytes, or null when [sealed] is too short, tampered with or sealed by another key. */
internal fun openWith(
    key: SecretKey,
    sealed: ByteArray,
): ByteArray? {
    if (sealed.size <= GCM_IV_BYTES) return null
    return try {
        val cipher = Cipher.getInstance(TRANSFORMATION)
        cipher.init(Cipher.DECRYPT_MODE, key, GCMParameterSpec(GCM_TAG_BITS, sealed, 0, GCM_IV_BYTES))
        cipher.doFinal(sealed, GCM_IV_BYTES, sealed.size - GCM_IV_BYTES)
    } catch (_: GeneralSecurityException) {
        // AEADBadTagException and friends: these bytes are not a session this key sealed.
        null
    }
}
