package br.com.encomendo.android

import android.os.Bundle
import androidx.activity.ComponentActivity
import androidx.activity.compose.setContent
import androidx.activity.enableEdgeToEdge
import androidx.compose.foundation.layout.fillMaxSize
import androidx.compose.foundation.layout.safeDrawingPadding
import androidx.compose.material3.Surface
import androidx.compose.ui.Modifier
import br.com.encomendo.core.navigation.RootRoute
import br.com.encomendo.core.ui.theme.EncomendoTheme

/** The single activity; every screen is a Compose destination under RootRoute. */
class MainActivity : ComponentActivity() {
    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        setContent {
            EncomendoTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    RootRoute(modifier = Modifier.safeDrawingPadding())
                }
            }
        }
    }
}
