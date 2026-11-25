package br.com.encomendo.core.navigation

import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.ui.Modifier
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.LocalViewModelStoreOwner
import androidx.navigation.NavGraphBuilder
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.navigation
import androidx.navigation.compose.rememberNavController
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.ui.LocalGrantedPermissions
import br.com.encomendo.feature.auth.LoginScreen
import br.com.encomendo.feature.hoje.HojeScreen
import org.koin.compose.viewmodel.koinViewModel

/** RootRoute connects [RootViewModel] to [RootNavHost]; the activity shows only this. */
@Composable
fun RootRoute(
    modifier: Modifier = Modifier,
    viewModel: RootViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    RootNavHost(state = state, modifier = modifier)
}

/**
 * RootNavHost hosts every graph of the management app: the auth graph while
 * signed out, the management graph while signed in. Screen ViewModels live in
 * the session's own store ([RootUiState.Ready.screens]), and every
 * `PermissionGate` below reads the session's permissions; signed out, nothing
 * is granted.
 */
@Composable
fun RootNavHost(
    state: RootUiState,
    modifier: Modifier = Modifier,
) {
    val session =
        when (state) {
            RootUiState.Loading -> return
            is RootUiState.Ready -> state
        }
    val signedIn = session is RootUiState.SignedIn
    val permissions = (session as? RootUiState.SignedIn)?.permissions ?: GrantedPermissions.None
    // A fresh back stack per session: Back never leads from the login into
    // management screens after a logout, nor from Hoje back to the login.
    key(session.screens) {
        CompositionLocalProvider(
            LocalViewModelStoreOwner provides session.screens,
            LocalGrantedPermissions provides permissions,
        ) {
            NavHost(
                navController = rememberNavController(),
                startDestination = if (signedIn) BackofficeGraph else AuthGraph,
                modifier = modifier,
            ) {
                authNavGraph()
                backofficeNavGraph()
            }
        }
    }
}

/** Login, and later sign-up, tenant picker and forgot password (`feature/auth`). */
fun NavGraphBuilder.authNavGraph() {
    navigation<AuthGraph>(startDestination = LoginRoute) {
        composable<LoginRoute> { LoginScreen() }
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
