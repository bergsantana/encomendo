package br.com.encomendo.feature.catalogo

import kotlin.test.Test
import kotlin.test.assertEquals

class CatalogTextTest {
    @Test
    fun everyFieldProblemHasItsText() {
        assertEquals(FieldProblem.entries.toSet(), problemTexts.keys)
    }
}
