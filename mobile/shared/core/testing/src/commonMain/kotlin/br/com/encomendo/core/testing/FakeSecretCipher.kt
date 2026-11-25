package br.com.encomendo.core.testing

import br.com.encomendo.core.security.SecretCipher

/**
 * FakeSecretCipher stands in for the Keystore cipher in JVM tests. It scrambles
 * the bytes behind a marker, so a test can check that no plain token reaches
 * storage. With [canOpen] false it behaves like a device whose key is gone.
 */
class FakeSecretCipher(
    private val canOpen: Boolean = true,
) : SecretCipher {
    override fun seal(plain: ByteArray): ByteArray = MARKER + plain.scrambled()

    override fun open(sealed: ByteArray): ByteArray? {
        val ours = sealed.size >= MARKER.size && sealed.copyOfRange(0, MARKER.size).contentEquals(MARKER)
        return if (canOpen && ours) sealed.copyOfRange(MARKER.size, sealed.size).scrambled() else null
    }

    private fun ByteArray.scrambled(): ByteArray = ByteArray(size) { (this[it].toInt() xor MASK).toByte() }

    private companion object {
        val MARKER = "sealed:".encodeToByteArray()
        const val MASK = 0x5A
    }
}
