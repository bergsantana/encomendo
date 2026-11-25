package br.com.encomendo.core.security

import javax.crypto.KeyGenerator
import javax.crypto.SecretKey
import kotlin.test.Test
import kotlin.test.assertContentEquals
import kotlin.test.assertFalse
import kotlin.test.assertNull

/**
 * The framing KeystoreCipher uses, run on the JVM with a software key: the
 * Keystore only supplies the key, so everything else can be checked here.
 */
class AesGcmTest {
    private val key = newKey()
    private val plain = """{"access":"a","refresh":"r"}""".encodeToByteArray()

    @Test
    fun opensWhatItSealed() {
        assertContentEquals(plain, openWith(key, sealWith(key, plain)))
    }

    @Test
    fun sealsTheSameInputDifferentlyEachTime() {
        // A fresh IV per seal: equal sessions never produce equal bytes on disk.
        assertFalse(sealWith(key, plain).contentEquals(sealWith(key, plain)))
    }

    @Test
    fun refusesTamperedBytes() {
        val sealed = sealWith(key, plain)
        sealed[sealed.lastIndex] = (sealed.last().toInt() xor 1).toByte()

        assertNull(openWith(key, sealed))
    }

    @Test
    fun refusesInputTooShortToHoldAnIvAndATag() {
        assertNull(openWith(key, ByteArray(GCM_IV_BYTES)))
        assertNull(openWith(key, sealWith(key, plain).copyOf(GCM_IV_BYTES + 1)))
    }

    @Test
    fun refusesAnotherKey() {
        assertNull(openWith(newKey(), sealWith(key, plain)))
    }

    private fun newKey(): SecretKey = KeyGenerator.getInstance("AES").apply { init(AES_BITS) }.generateKey()

    private companion object {
        const val AES_BITS = 256
    }
}
