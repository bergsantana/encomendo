package br.com.encomendo.core.di

import coil3.PlatformContext

/**
 * Returns a [PlatformContext] for tests that build the app's graph. Coil's context is the platform's own (an Android
 * `Context`), which common code cannot create, so each platform's tests supply one.
 */
expect fun testPlatformContext(): PlatformContext
