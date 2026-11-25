package br.com.encomendo.core.security

/**
 * SessionTokens is the signed-in user's token pair (docs/architecture.md 5.1).
 * [toString] never prints them, so a log line or crash report cannot leak a session.
 */
data class SessionTokens(
    /** The 15-minute JWT sent as `Authorization: Bearer`. */
    val accessToken: String,
    /** The opaque refresh token; valid once, rotated on every refresh. */
    val refreshToken: String,
) {
    override fun toString(): String = "SessionTokens(accessToken=██, refreshToken=██)"
}
