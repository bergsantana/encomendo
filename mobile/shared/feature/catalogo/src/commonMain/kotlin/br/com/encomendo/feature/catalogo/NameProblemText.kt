package br.com.encomendo.feature.catalogo

import androidx.compose.runtime.Composable
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.attribute_name_missing
import br.com.encomendo.feature.catalogo.resources.attribute_name_taken
import br.com.encomendo.feature.catalogo.resources.attribute_value_duplicate
import br.com.encomendo.feature.catalogo.resources.attribute_value_missing
import br.com.encomendo.feature.catalogo.resources.category_name_missing
import br.com.encomendo.feature.catalogo.resources.category_name_taken
import br.com.encomendo.feature.catalogo.resources.problem_supplier_name_control
import br.com.encomendo.feature.catalogo.resources.problem_supplier_name_missing
import br.com.encomendo.feature.catalogo.resources.problem_supplier_name_taken
import br.com.encomendo.feature.catalogo.resources.problem_supplier_name_too_long
import br.com.encomendo.feature.catalogo.resources.problem_supplier_phone_invalid
import br.com.encomendo.feature.catalogo.resources.problem_supplier_phone_missing
import org.jetbrains.compose.resources.stringResource

/** The text under a "Novo fornecedor" field for [this] problem. */
@Composable
internal fun SupplierProblem.text(): String =
    when (this) {
        SupplierProblem.NameMissing -> stringResource(Res.string.problem_supplier_name_missing)
        SupplierProblem.NameTooLong -> stringResource(Res.string.problem_supplier_name_too_long)
        SupplierProblem.NameControlCharacters -> stringResource(Res.string.problem_supplier_name_control)
        SupplierProblem.NameTaken -> stringResource(Res.string.problem_supplier_name_taken)
        SupplierProblem.PhoneMissing -> stringResource(Res.string.problem_supplier_phone_missing)
        SupplierProblem.PhoneInvalid -> stringResource(Res.string.problem_supplier_phone_invalid)
        is SupplierProblem.Server -> message
    }

/** The text under a category's name for [this] problem. */
@Composable
internal fun NameProblem.categoryText(): String =
    when (this) {
        NameProblem.Missing -> stringResource(Res.string.category_name_missing)
        NameProblem.Taken -> stringResource(Res.string.category_name_taken)
    }

/** The text under a field's name for [this] problem. */
@Composable
internal fun NameProblem.attributeText(): String =
    when (this) {
        NameProblem.Missing -> stringResource(Res.string.attribute_name_missing)
        NameProblem.Taken -> stringResource(Res.string.attribute_name_taken)
    }

/** The text under a field's value for [this] problem. */
@Composable
internal fun ValueProblem.text(): String =
    when (this) {
        ValueProblem.Missing -> stringResource(Res.string.attribute_value_missing)
        ValueProblem.Duplicate -> stringResource(Res.string.attribute_value_duplicate)
    }
