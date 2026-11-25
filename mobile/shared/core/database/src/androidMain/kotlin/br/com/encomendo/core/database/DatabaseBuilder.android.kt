package br.com.encomendo.core.database

import android.content.Context
import androidx.room3.Room
import androidx.room3.RoomDatabase

/** The database file name on the device. */
private const val DATABASE_NAME = "encomendo.db"

/** Returns the Android builder for [EncomendoDatabase]; pass it to [buildDatabase]. */
fun databaseBuilder(context: Context): RoomDatabase.Builder<EncomendoDatabase> {
    val appContext = context.applicationContext
    return Room.databaseBuilder<EncomendoDatabase>(
        context = appContext,
        name = appContext.getDatabasePath(DATABASE_NAME).absolutePath,
    )
}
