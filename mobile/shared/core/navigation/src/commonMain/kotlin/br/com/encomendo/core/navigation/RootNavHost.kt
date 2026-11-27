package br.com.encomendo.core.navigation

import androidx.compose.foundation.layout.consumeWindowInsets
import androidx.compose.foundation.layout.padding
import androidx.compose.material3.DrawerValue
import androidx.compose.material3.ModalNavigationDrawer
import androidx.compose.material3.Scaffold
import androidx.compose.material3.rememberDrawerState
import androidx.compose.runtime.Composable
import androidx.compose.runtime.CompositionLocalProvider
import androidx.compose.runtime.LaunchedEffect
import androidx.compose.runtime.getValue
import androidx.compose.runtime.key
import androidx.compose.runtime.rememberCoroutineScope
import androidx.compose.ui.Modifier
import androidx.lifecycle.compose.collectAsStateWithLifecycle
import androidx.lifecycle.viewmodel.compose.LocalViewModelStoreOwner
import androidx.navigation.NavController
import androidx.navigation.NavGraphBuilder
import androidx.navigation.compose.NavHost
import androidx.navigation.compose.composable
import androidx.navigation.compose.currentBackStackEntryAsState
import androidx.navigation.compose.navigation
import androidx.navigation.compose.rememberNavController
import androidx.navigation.toRoute
import br.com.encomendo.core.domain.session.GrantedPermissions
import br.com.encomendo.core.domain.session.Permission
import br.com.encomendo.core.ui.LocalGrantedPermissions
import br.com.encomendo.core.ui.LocalPhotoLoader
import br.com.encomendo.core.ui.PermissionGate
import br.com.encomendo.feature.auth.AccountBarRoute
import br.com.encomendo.feature.auth.ForgotPasswordRoute
import br.com.encomendo.feature.auth.LoginNavigation
import br.com.encomendo.feature.auth.LoginRoute
import br.com.encomendo.feature.auth.SignUpRoute
import br.com.encomendo.feature.auth.StorePickerRoute
import br.com.encomendo.feature.catalogo.AttributesRoute
import br.com.encomendo.feature.catalogo.CatalogoNavigation
import br.com.encomendo.feature.catalogo.CatalogoRoute
import br.com.encomendo.feature.catalogo.CategoriesRoute
import br.com.encomendo.feature.catalogo.ItemFormRoute
import br.com.encomendo.feature.catalogo.ItemsNavigation
import br.com.encomendo.feature.catalogo.KitFormRoute
import br.com.encomendo.feature.hoje.HojeScreen
import br.com.encomendo.feature.members.MembersRoute
import coil3.ImageLoader
import kotlinx.coroutines.launch
import org.koin.compose.koinInject
import org.koin.compose.viewmodel.koinViewModel

/** RootRoute connects [RootViewModel] to [RootNavHost]; the activity shows only this. */
@Composable
fun RootRoute(
    modifier: Modifier = Modifier,
    viewModel: RootViewModel = koinViewModel(),
) {
    val state by viewModel.uiState.collectAsStateWithLifecycle()
    // Photo thumbnails load through the loader with no bearer token (ADR 0026); screens only ask for an image.
    CompositionLocalProvider(LocalPhotoLoader provides koinInject<ImageLoader>()) {
        RootNavHost(state = state, modifier = modifier)
    }
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
    val navController = rememberNavController()
    NavHost(navController = navController, startDestination = AuthGraph, modifier = modifier) {
        authNavGraph(navController)
    }
}

/**
 * The management screens under the account bar (the store's name, the
 * person's, Trocar de loja and Sair), with the menu in a drawer the bar's
 * "Menu" opens (docs/adr/0020-android-management-menu.md).
 */
@Composable
private fun ManagementShell(modifier: Modifier) {
    val navController = rememberNavController()
    val drawer = rememberDrawerState(DrawerValue.Closed)
    // Only opens and closes the drawer: no request ever runs in it (ADR 0018).
    val drawerScope = rememberCoroutineScope()
    val entry by navController.currentBackStackEntryAsState()
    ModalNavigationDrawer(
        drawerState = drawer,
        drawerContent = {
            ManagementMenu(
                current = entry?.destination,
                onOpen = { destination ->
                    drawerScope.launch { drawer.close() }
                    navController.openFromMenu(destination)
                },
            )
        },
        modifier = modifier,
    ) {
        Scaffold(
            topBar = {
                AccountBarRoute(
                    // Single top: the bar stays above the picker, and a second tap must not stack another.
                    onSwitchStore = { navController.navigate(StorePickerDestination) { launchSingleTop = true } },
                    navigationIcon = { MenuButton(onClick = { drawerScope.launch { drawer.open() } }) },
                )
            },
        ) { padding ->
            NavHost(
                navController = navController,
                startDestination = BackofficeGraph,
                // Consumed, so a screen's imePadding() does not count the navigation bar twice.
                modifier = Modifier.padding(padding).consumeWindowInsets(padding),
            ) {
                backofficeNavGraph(navController)
            }
        }
    }
}

/**
 * Opens a menu entry on top of Hoje, the start: Back from any entry goes to
 * Hoje, and from Hoje leaves the app. Tapping the open entry again does nothing.
 */
private fun NavController.openFromMenu(destination: Any) =
    navigate(destination) {
        popUpTo<HojeDestination>()
        launchSingleTop = true
    }

/** Login, sign-up and Esqueci a senha (`feature/auth`); the store picker has its own state. */
fun NavGraphBuilder.authNavGraph(navController: NavController) {
    navigation<AuthGraph>(startDestination = LoginDestination) {
        composable<LoginDestination> { entry ->
            // Set by Esqueci a senha on its way back, so the login says the password changed.
            val passwordChanged by entry.savedStateHandle
                .getStateFlow(
                    PASSWORD_CHANGED,
                    false,
                ).collectAsStateWithLifecycle()
            LoginRoute(
                LoginNavigation(
                    onCreateAccount = { navController.navigate(SignUpDestination) { launchSingleTop = true } },
                    onForgotPassword = { navController.navigate(ForgotPasswordDestination) { launchSingleTop = true } },
                    passwordChanged = passwordChanged,
                    onNoticeShown = { entry.savedStateHandle[PASSWORD_CHANGED] = false },
                ),
            )
        }
        composable<ForgotPasswordDestination> {
            ForgotPasswordRoute(
                onBackToLogin = { navController.popBackStack() },
                onPasswordChanged = {
                    navController.previousBackStackEntry?.savedStateHandle?.set(PASSWORD_CHANGED, true)
                    navController.popBackStack()
                },
            )
        }
        composable<SignUpDestination> {
            SignUpRoute(onHaveAccount = { navController.popBackStack() })
        }
    }
}

/**
 * The management screens, in menu order: Hoje · Pedidos · Compras · Produção ·
 * Estoque · Catálogo · Clientes · Financeiro, then Equipe. Each feature adds
 * its route here, and its [MenuEntry], when it ships, matching the web admin
 * features one to one. A screen that needs a permission leaves for Hoje once
 * the session no longer has it, as the web's route guard does.
 */
fun NavGraphBuilder.backofficeNavGraph(navController: NavController) {
    navigation<BackofficeGraph>(startDestination = HojeDestination) {
        composable<HojeDestination> { HojeScreen() }
        catalogoNavGraph(navController)
        composable<MembersDestination> {
            PermissionGate(Permission.MembersManage, otherwise = { LeaveForHoje(navController) }) { MembersRoute() }
        }
        // Choosing a store moves the session; the root then starts that store's screens afresh.
        composable<StorePickerDestination> {
            // Pops the picker itself, so a second tap on the open store, still in flight, pops nothing more.
            StorePickerRoute(onClose = { navController.popBackStack(StorePickerDestination, inclusive = true) })
        }
    }
}

/**
 * Catálogo: the Itens and Kits tabs and the screens opened from them. Every role with `catalog:read` may read them; the
 * screens show editing controls only to `catalog:write`, and the API checks every call.
 */
private fun NavGraphBuilder.catalogoNavGraph(navController: NavController) {
    composable<CatalogoDestination> {
        PermissionGate(Permission.CatalogRead, otherwise = { LeaveForHoje(navController) }) {
            CatalogoRoute(
                CatalogoNavigation(
                    items =
                        ItemsNavigation(
                            onOpenItem = { itemId ->
                                navController.navigate(ItemFormDestination(itemId)) { launchSingleTop = true }
                            },
                            onOpenCategories = {
                                navController.navigate(
                                    CategoriesDestination,
                                ) { launchSingleTop = true }
                            },
                            onOpenAttributes = {
                                navController.navigate(
                                    AttributesDestination,
                                ) { launchSingleTop = true }
                            },
                        ),
                    onOpenKit = { kitId ->
                        navController.navigate(KitFormDestination(kitId)) { launchSingleTop = true }
                    },
                ),
            )
        }
    }
    composable<ItemFormDestination> { entry ->
        val itemId = entry.toRoute<ItemFormDestination>().itemId
        PermissionGate(Permission.CatalogRead, otherwise = { LeaveForHoje(navController) }) {
            ItemFormRoute(itemId = itemId, onClose = { navController.popBackStack() })
        }
    }
    composable<KitFormDestination> { entry ->
        val kitId = entry.toRoute<KitFormDestination>().kitId
        PermissionGate(Permission.CatalogRead, otherwise = { LeaveForHoje(navController) }) {
            KitFormRoute(kitId = kitId, onClose = { navController.popBackStack() })
        }
    }
    composable<CategoriesDestination> {
        PermissionGate(Permission.CatalogRead, otherwise = { LeaveForHoje(navController) }) { CategoriesRoute() }
    }
    composable<AttributesDestination> {
        PermissionGate(Permission.CatalogRead, otherwise = { LeaveForHoje(navController) }) { AttributesRoute() }
    }
}

/** Leaves a screen the session may no longer see, back to Hoje, which every role may. */
@Composable
private fun LeaveForHoje(navController: NavController) {
    LaunchedEffect(navController) { navController.popBackStack<HojeDestination>(inclusive = false) }
}

/** The login's saved-state key for "the password was just reset". */
private const val PASSWORD_CHANGED = "password_changed"
