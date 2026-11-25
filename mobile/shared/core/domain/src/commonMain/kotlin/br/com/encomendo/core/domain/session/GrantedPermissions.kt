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
 *
 * It is a value: grants with the same keys are equal, and it keeps its own
 * copy, so a later change to the caller's collection never changes it.
 */
class GrantedPermissions(
    keys: Collection<Permission>,
) {
    private val keys: Set<Permission> = keys.toSet()

    /** Returns true only when [required] was granted. */
    fun allows(required: Permission): Boolean = required in keys

    override fun equals(other: Any?): Boolean = other is GrantedPermissions && other.keys == keys

    override fun hashCode(): Int = keys.hashCode()

    /** Holds [None], the grants before a session supplies them. */
    companion object {
        /** None allows nothing, so the UI fails closed until the session says otherwise. */
        val None = GrantedPermissions(emptySet())
    }
}
