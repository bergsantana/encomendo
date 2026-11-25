package br.com.encomendo.core.domain.session

import kotlin.jvm.JvmInline

/**
 * Permission names one thing a role may do, exactly as the API names it
 * (for example `order:write`, the form web's `usePermission` takes). The
 * names come from the contract, so the app defines none of its own.
 */
@JvmInline
value class Permission(
    /** The permission key as the API sends it; compared exactly. */
    val key: String,
)

/**
 * GrantedPermissions is what the signed-in user may do in the current tenant.
 * The app only hides what is not granted; the backend stays the authority
 * and checks every call (docs/architecture.md 5.4).
 */
class GrantedPermissions(
    private val keys: Set<Permission>,
) {
    /** Returns true only when [required] was granted. */
    fun allows(required: Permission): Boolean = required in keys

    /** The grants before a session supplies them. */
    companion object {
        /** Nothing is allowed until the session says otherwise: the UI fails closed. */
        val None = GrantedPermissions(emptySet())
    }
}
