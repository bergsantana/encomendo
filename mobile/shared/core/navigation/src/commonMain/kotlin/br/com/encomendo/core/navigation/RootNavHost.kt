package br.com.encomendo.core.navigation

import androidx.compose.foundation.layout.padding
import androidx.compose.material3.Scaffold
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.ui.Modifier
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.LocalViewModelStoreOwner
import androidx.navigation.NavController
import androidx.navigation.NavGraphBuilder
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.navigation
import androidx.navigation.compose.rememberNavController
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.ui.LocalGrantedPermissions
import br.com.encomendo.feature.auth.AccountBarRoute
import br.com.encomendo.feature.auth.LoginRoute
import br.com.encomendo.feature.auth.StorePickerRoute
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
 * signed out, the store picker while signed in without a store, and the
 * management graph under the account bar once a store is open. Screen ViewModels live in
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
    val permissions = (session as? RootUiState.SignedIn)?.permissions ?: GrantedPermissions.None
    // A fresh back stack per session: Back never leads from the login into
    // management screens after a logout, nor from Hoje back to the login.
    key(session.screens) {
        CompositionLocalProvider(
            LocalViewModelStoreOwner provides session.screens,
            LocalGrantedPermissions provides permissions,
        ) {
            when (session) {
                is RootUiState.SignedOut -> AuthHost(modifier)
                is RootUiState.ChoosingStore -> StorePickerRoute(modifier)
                is RootUiState.SignedIn -> ManagementShell(modifier)
            }
        }
    }
}

@Composable
private fun AuthHost(modifier: Modifier) {
    NavHost(navController = rememberNavController(), startDestination = AuthGraph, modifier = modifier) {
        authNavGraph()
    }
}

/** The management screens under the account bar: the store's name, the person's, Trocar de loja and Sair. */
@Composable
private fun ManagementShell(modifier: Modifier) {
    val navController = rememberNavController()
    Scaffold(
        topBar = {
            AccountBarRoute(
                // Single top: the bar stays above the picker, and a second tap must not stack another.
                onSwitchStore = { navController.navigate(StorePickerDestination) { launchSingleTop = true } },
            )
        },
        modifier = modifier,
    ) { padding ->
        NavHost(
            navController = navController,
            startDestination = BackofficeGraph,
            modifier = Modifier.padding(padding),
        ) {
            backofficeNavGraph(navController)
        }
    }
}

/** Login, and later sign-up and forgot password (`feature/auth`); the store picker has its own state. */
fun NavGraphBuilder.authNavGraph() {
    navigation<AuthGraph>(startDestination = LoginDestination) {
        composable<LoginDestination> { LoginRoute() }
    }
}

/**
 * The management screens, in menu order: Hoje · Pedidos · Compras · Produção ·
 * Estoque · Catálogo · Clientes · Financeiro. Each feature adds its route here
 * when it ships, matching the web admin features one to one.
 */
fun NavGraphBuilder.backofficeNavGraph(navController: NavController) {
    navigation<BackofficeGraph>(startDestination = HojeDestination) {
        composable<HojeDestination> { HojeScreen() }
        // Choosing a store moves the session; the root then starts that store's screens afresh.
        composable<StorePickerDestination> {
            // Pops the picker itself, so a second tap on the open store, still in flight, pops nothing more.
            StorePickerRoute(onClose = { navController.popBackStack(StorePickerDestination, inclusive = true) })
        }
    }
}
