package br.com.encomendo.feature.catalogo

import androidx.compose.foundation.layout.Arrangement
import androidx.compose.foundation.layout.Column
import androidx.compose.foundation.layout.Row
import androidx.compose.foundation.layout.fillMaxWidth
import androidx.compose.foundation.layout.heightIn
import androidx.compose.foundation.layout.padding
import androidx.compose.foundation.selection.toggleable
import androidx.compose.material3.Checkbox
import androidx.compose.material3.MaterialTheme
import androidx.compose.material3.OutlinedCard
import androidx.compose.material3.Text
import androidx.compose.runtime.Composable
import androidx.compose.runtime.getValue
import androidx.compose.runtime.setValue
import androidx.compose.ui.Alignment
import androidx.compose.ui.Modifier
import androidx.compose.ui.semantics.Role
import androidx.compose.ui.text.input.KeyboardType
import androidx.compose.ui.unit.dp
import br.com.encomendo.core.ui.TouchTarget
import br.com.encomendo.feature.catalogo.resources.Res
import br.com.encomendo.feature.catalogo.resources.choice_add
import br.com.encomendo.feature.catalogo.resources.choice_days
import br.com.encomendo.feature.catalogo.resources.choice_label
import br.com.encomendo.feature.catalogo.resources.choice_price
import br.com.encomendo.feature.catalogo.resources.choice_remove
import br.com.encomendo.feature.catalogo.resources.option_name
import br.com.encomendo.feature.catalogo.resources.option_new
import br.com.encomendo.feature.catalogo.resources.option_remove
import br.com.encomendo.feature.catalogo.resources.option_required
import br.com.encomendo.feature.catalogo.resources.options_hint
import br.com.encomendo.feature.catalogo.resources.options_title
import org.jetbrains.compose.resources.stringResource

/** "Opções do cliente": each option with its choices, and the buttons to add and remove them. */
@Composable
internal fun OptionsEditor(
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    SectionTitle(stringResource(Res.string.options_title))
    Text(
        text = stringResource(Res.string.options_hint),
        style = MaterialTheme.typography.bodyMedium,
        color = MaterialTheme.colorScheme.onSurfaceVariant,
    )
    state.fields.options.forEach { option -> OptionCard(option, state, actions) }
    SecondaryButton(stringResource(Res.string.option_new), { actions.onEdit(null) { it.withNewOption() } })
}

@Composable
private fun OptionCard(
    option: OptionFields,
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    OutlinedCard(modifier = Modifier.fillMaxWidth()) {
        Column(modifier = Modifier.padding(12.dp), verticalArrangement = Arrangement.spacedBy(8.dp)) {
            CatalogTextField(
                label = stringResource(Res.string.option_name),
                value = option.name,
                onValueChange = { text ->
                    actions.onEdit(
                        ItemField.OptionName(option.key),
                    ) { f -> f.withOption(option.key) { it.copy(name = text) } }
                },
                problem = state.issue(ItemField.OptionName(option.key))?.text(),
            )
            Row(
                modifier =
                    Modifier
                        .fillMaxWidth()
                        .heightIn(min = TouchTarget)
                        .toggleable(
                            value = option.required,
                            role = Role.Checkbox,
                            onValueChange = { checked ->
                                actions.onEdit(null) { f -> f.withOption(option.key) { it.copy(required = checked) } }
                            },
                        ),
                verticalAlignment = Alignment.CenterVertically,
            ) {
                Checkbox(checked = option.required, onCheckedChange = null)
                Text(stringResource(Res.string.option_required), modifier = Modifier.padding(start = 12.dp))
            }
            option.choices.forEach { choice -> ChoiceEditor(option, choice, state, actions) }
            state.issue(ItemField.OptionChoices(option.key))?.let {
                Text(it.text(), color = MaterialTheme.colorScheme.error, modifier = Modifier.announced())
            }
            SecondaryButton(
                stringResource(Res.string.choice_add),
                { actions.onEdit(null) { it.withNewChoice(option.key) } },
            )
            SecondaryButton(
                stringResource(Res.string.option_remove),
                { actions.onEdit(null) { f -> f.copy(options = f.options.filter { it.key != option.key }) } },
            )
        }
    }
}

@Composable
private fun ChoiceEditor(
    option: OptionFields,
    choice: ChoiceFields,
    state: ItemFormUiState,
    actions: ItemFormActions,
) {
    val edit = { field: ItemField, change: (ChoiceFields) -> ChoiceFields ->
        actions.onEdit(field) { f -> f.withChoice(option.key, choice.key, change) }
    }
    CatalogTextField(
        label = stringResource(Res.string.choice_label),
        value = choice.label,
        onValueChange = { text -> edit(ItemField.ChoiceLabel(option.key, choice.key)) { it.copy(label = text) } },
        problem = state.issue(ItemField.ChoiceLabel(option.key, choice.key))?.text(),
    )
    CatalogTextField(
        label = stringResource(Res.string.choice_price),
        value = choice.extraPrice,
        onValueChange = { text -> edit(ItemField.ChoicePrice(option.key, choice.key)) { it.copy(extraPrice = text) } },
        problem = state.issue(ItemField.ChoicePrice(option.key, choice.key))?.text(),
        keyboardType = KeyboardType.Decimal,
    )
    CatalogTextField(
        label = stringResource(Res.string.choice_days),
        value = choice.extraDays,
        onValueChange = { text -> edit(ItemField.ChoiceDays(option.key, choice.key)) { it.copy(extraDays = text) } },
        problem = state.issue(ItemField.ChoiceDays(option.key, choice.key))?.text(),
        keyboardType = KeyboardType.Number,
    )
    SecondaryButton(
        stringResource(Res.string.choice_remove),
        {
            actions.onEdit(null) { f ->
                f.withOption(option.key) { it.copy(choices = it.choices.filter { c -> c.key != choice.key }) }
            }
        },
    )
}
