package br.com.encomendo.core.navigation

import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.navigation.NavGraphBuilder
import androidx.navigation.NavHostController
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.navigation
import androidx.navigation.compose.rememberNavController
import br.com.encomendo.feature.hoje.HojeScreen

/** RootNavHost hosts every graph of the management app. */
@Composable
fun RootNavHost(
    modifier: Modifier = Modifier,
    navController: NavHostController = rememberNavController(),
) {
    NavHost(navController = navController, startDestination = BackofficeGraph, modifier = modifier) {
        backofficeNavGraph()
    }
}

/**
 * The management screens, in menu order: Hoje · Pedidos · Compras · Produção ·
 * Estoque · Catálogo · Clientes · Financeiro. Each feature adds its route here
 * when it ships, matching the web admin features one to one.
 */
fun NavGraphBuilder.backofficeNavGraph() {
    navigation<BackofficeGraph>(startDestination = HojeRoute) {
        composable<HojeRoute> { HojeScreen() }
    }
}
