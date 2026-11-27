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
import br.com.encomendo.core.platform.AndroidPhotoPicker
import br.com.encomendo.core.ui.theme.EncomendoTheme
import org.koin.android.ext.android.inject

/** The single activity; every screen is a Compose destination under RootRoute. */
class MainActivity : ComponentActivity() {
    private val photoPicker: AndroidPhotoPicker by inject()

    override fun onCreate(savedInstanceState: Bundle?) {
        enableEdgeToEdge()
        super.onCreate(savedInstanceState)
        // Result launchers must be registered before the activity starts; screens then ask the picker for photos.
        photoPicker.attach(this)
        setContent {
            EncomendoTheme {
                Surface(modifier = Modifier.fillMaxSize()) {
                    RootRoute(modifier = Modifier.safeDrawingPadding())
                }
            }
        }
    }
}
