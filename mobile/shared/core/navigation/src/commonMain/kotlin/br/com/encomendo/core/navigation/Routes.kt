package br.com.encomendo.core.navigation

import kotlinx.serialization.Serializable

/** The management graph, opened after a staff login (milestone 2 adds the auth graph before it). */
@Serializable
data object BackofficeGraph

/** Hoje: what needs the owner today. */
@Serializable
data object HojeRoute
