package br.com.encomendo.core.network

/** ApiClientConfig holds the settings of the Encomendo API client. */
data class ApiClientConfig(
    /** The API origin, for example `https://api.encomendo.com.br`; request paths start with `/`. */
    val baseUrl: String,
    /** How long to wait for a connection before reporting [ApiErrorCode.Network]. */
    val connectTimeoutMillis: Long = 15_000,
    /** How long a whole request may take, so a weak signal fails visibly instead of hanging. */
    val requestTimeoutMillis: Long = 30_000,
)
