package br.com.encomendo.core.security

import br.com.encomendo.core.domain.session.Session

/**
 * StoredSession is a signed-in session as the device keeps it: the token pair
 * and what the API said about the session when it issued them. They are
 * stored and replaced together, so the permissions on screen always belong to
 * the tokens in use. [toString] never prints the tokens.
 */
data class StoredSession(
    /** The tokens the API client sends and refreshes. */
    val tokens: SessionTokens,
    /** The user, store, role and permissions those tokens were issued for. */
    val session: Session,
)
