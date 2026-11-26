package br.com.encomendo.core.navigation

import kotlinx.serialization.Serializable

// Navigation keys are named XDestination, so they never clash with a feature's
// XRoute composable (mobile/CLAUDE.md, Layers).

/** The auth graph, shown while nobody is signed in. */
@Serializable
data object AuthGraph

/** Login: email and password. */
@Serializable
data object LoginDestination

/** Criar conta: a new staff account, which then creates its first store. */
@Serializable
data object SignUpDestination

/** The management graph, opened after a staff login. */
@Serializable
data object BackofficeGraph

/** Hoje: what needs the owner today. */
@Serializable
data object HojeDestination

/** The store picker, reached from "Trocar de loja" inside the management graph. */
@Serializable
data object StorePickerDestination
