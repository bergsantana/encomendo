package br.com.encomendo.core.di

import android.content.Context
import android.content.ContextWrapper
import coil3.PlatformContext

/** A context with nothing behind it: building the image loader only keeps it, and these tests load no image. */
private class EmptyContext : ContextWrapper(null) {
    override fun getApplicationContext(): Context = this
}

actual fun testPlatformContext(): PlatformContext = EmptyContext()
