package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.rememberScrollState
import androidx.compose.foundation.verticalScroll
import androidx.compose.material3.AlertDialog
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.ui.Modifier
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.form_cancel
import br.com.encomendo.feature.catalogo.resources.form_supplier_new
import br.com.encomendo.feature.catalogo.resources.supplier_add
import br.com.encomendo.feature.catalogo.resources.supplier_name
import br.com.encomendo.feature.catalogo.resources.supplier_phone
import br.com.encomendo.feature.catalogo.resources.supplier_phone_hint
import org.jetbrains.compose.resources.stringResource

/**
 * NewSupplierDialog is "Novo fornecedor": the supplier's name and WhatsApp, created inside the item form
 * (ADR 0025). Empty and wrong fields are marked on tap, as everywhere, and "Adicionar fornecedor" waits while the
 * supplier is saved.
 */
@Composable
internal fun NewSupplierDialog(
    form: NewSupplierForm,
    actions: NewSupplierActions,
) {
    AlertDialog(
        // Nothing closes it while the supplier is being saved: its answer must land here.
        onDismissRequest = { if (!form.saving) actions.onClose() },
        title = { Text(stringResource(Res.string.form_supplier_new)) },
        text = {
            Column(
                modifier = Modifier.verticalScroll(rememberScrollState()),
                verticalArrangement = Arrangement.spacedBy(8.dp),
            ) {
                form.message?.let { MessageText(message = it) }
                CatalogTextField(
                    label = stringResource(Res.string.supplier_name),
                    value = form.name,
                    onValueChange = actions.onNameChange,
                    problem = form.nameIssue?.text(),
                )
                CatalogTextField(
                    label = stringResource(Res.string.supplier_phone),
                    value = form.phone,
                    onValueChange = actions.onPhoneChange,
                    problem = form.phoneIssue?.text(),
                    hint = stringResource(Res.string.supplier_phone_hint),
                    keyboardType = KeyboardType.Phone,
                )
            }
        },
        confirmButton = {
            PrimaryButton(
                text = stringResource(Res.string.supplier_add),
                onClick = actions.onAdd,
                enabled = !form.saving,
            )
        },
        dismissButton = {
            SecondaryButton(stringResource(Res.string.form_cancel), actions.onClose, enabled = !form.saving)
        },
    )
}
