package br.com.encomendo.core.security

/**
 * SecretCipher encrypts small secrets with a key the platform keeps outside
 * app storage: the Android Keystore on Android, the Keychain on iOS later.
 * Calls may block, so callers run them on an IO dispatcher.
 */
interface SecretCipher {
    /**
     * Returns [plain] encrypted and authenticated, with everything needed to
     * open it. Throws `IOException` when the platform cannot seal, as storage
     * would, so callers handle one kind of failure.
     */
    fun seal(plain: ByteArray): ByteArray

    /** Returns the plain bytes, or null when they cannot be decrypted (key gone, data tampered). */
    fun open(sealed: ByteArray): ByteArray?
}
