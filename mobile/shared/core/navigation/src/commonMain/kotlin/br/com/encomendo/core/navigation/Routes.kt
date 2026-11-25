package br.com.encomendo.core.navigation

import kotlinx.serialization.Serializable

/** The auth graph, shown while nobody is signed in. */
@Serializable
data object AuthGraph

/** Login: email and password (placeholder until the /auth contract). */
@Serializable
data object LoginRoute

/** The management graph, opened after a staff login. */
@Serializable
data object BackofficeGraph

/** Hoje: what needs the owner today. */
@Serializable
data object HojeRoute
