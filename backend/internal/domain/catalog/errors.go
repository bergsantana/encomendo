package catalog

import (
	"errors"
	"fmt"

	"github.com/bergsantana/encomendo/backend/internal/domain/shared"
)

// Not-found errors. Each matches shared.ErrNotFound (404 not_found).
// Repositories return them; RLS makes another store's row look the same.
var (
	ErrItemNotFound      = fmt.Errorf("catalog: item not found: %w", shared.ErrNotFound)
	ErrKitNotFound       = fmt.Errorf("catalog: kit not found: %w", shared.ErrNotFound)
	ErrCategoryNotFound  = fmt.Errorf("catalog: category not found: %w", shared.ErrNotFound)
	ErrAttributeNotFound = fmt.Errorf("catalog: attribute not found: %w", shared.ErrNotFound)
	ErrPhotoNotFound     = fmt.Errorf("catalog: photo not found: %w", shared.ErrNotFound)
)

// Refusals that depend on the state of the catalog. Each matches
// shared.ErrConflict (409) and has its own API code.
var (
	// ErrNameTaken means another row of the same kind has the name, ignoring
	// case: 409 name_taken. Repositories return it from a unique index.
	ErrNameTaken = fmt.Errorf("catalog: name taken: %w", shared.ErrConflict)
	// ErrItemInKit means kits that are not archived still hold the item, so
	// it cannot be archived: 409 item_in_kit. Only the app layer knows the
	// kits, so only it returns this error.
	ErrItemInKit = fmt.Errorf("catalog: item is in a kit: %w", shared.ErrConflict)
	// ErrCategoryInUse means items still have the category: 409
	// category_in_use. The app layer returns it.
	ErrCategoryInUse = fmt.Errorf("catalog: category in use: %w", shared.ErrConflict)
	// ErrItemArchived means a kit would hold, or a client would swap to, an
	// archived item: 409 item_archived. ArchivedItemsError carries the items.
	ErrItemArchived = fmt.Errorf("catalog: item is archived: %w", shared.ErrConflict)
	// ErrPhotoLimit means the item or kit already has 8 photos: 409
	// photo_limit.
	ErrPhotoLimit = fmt.Errorf("catalog: at most %d photos: %w", maxPhotos, shared.ErrConflict)
	// ErrSwapPriceNotPositive means a swap in keep mode would leave the kit at
	// a price of zero or less: the owner's discount is larger than the
	// remaining items. The swap is impossible; the store page decides how to
	// show that (milestone 7).
	ErrSwapPriceNotPositive = fmt.Errorf("catalog: swapped kit price is not above zero: %w", shared.ErrConflict)
)

// Refusals of bad input that carry their own API code or have no field.
// Each matches shared.ErrInvalid (400).
var (
	// ErrPhotoInvalid means a photo's key, type or size breaks a rule: 400
	// photo_invalid.
	ErrPhotoInvalid = fmt.Errorf("catalog: invalid photo: %w", shared.ErrInvalid)
	// ErrSwapNotInKit means a swap names an item the kit does not hold.
	ErrSwapNotInKit = fmt.Errorf("catalog: swap of an item not in the kit: %w", shared.ErrInvalid)
	// ErrSwapNotAllowed means a swap names a kit item the client cannot
	// swap, or an item the kit already holds as another member.
	ErrSwapNotAllowed = fmt.Errorf("catalog: item is not swappable: %w", shared.ErrInvalid)
	// ErrSwapNotListed means a swap's new item is not one of the item's
	// listed alternatives.
	ErrSwapNotListed = fmt.Errorf("catalog: item is not a listed alternative: %w", shared.ErrInvalid)
	// ErrPublishArchived means an archived item or kit cannot be published:
	// restore it first. The contract answers 400 invalid
	// (setItemPublished, setKitPublished).
	ErrPublishArchived = fmt.Errorf("catalog: archived rows cannot be published: %w", shared.ErrInvalid)
	// ErrSwapRepeated means two swaps name the same kit item.
	ErrSwapRepeated = fmt.Errorf("catalog: kit item swapped twice: %w", shared.ErrInvalid)
)

// ErrItemMissing means an item the kit refers to is not in the ItemLookup the
// caller passed. It matches shared.ErrNotFound.
var ErrItemMissing = fmt.Errorf("catalog: item missing from the lookup: %w", shared.ErrNotFound)

// Errors for records built from parts that can only be wrong through a bug:
// a zero id, an unknown mode, or a stored row that breaks a rule. They match
// no shared sentinel, so the API answers 500. What a person can get wrong
// comes back as a shared.ValidationError instead.
var (
	ErrInvalidItem      = errors.New("catalog: invalid item")
	ErrInvalidKit       = errors.New("catalog: invalid kit")
	ErrInvalidCategory  = errors.New("catalog: invalid category")
	ErrInvalidAttribute = errors.New("catalog: invalid attribute")
	ErrInvalidOption    = errors.New("catalog: invalid customization option")
)

// ArchivedItemsError lists the archived items a kit or a swap refers to. It
// matches ErrItemArchived (and so shared.ErrConflict); the API names the
// items in the problem's references.
type ArchivedItemsError struct {
	// Items are the archived items, in the order the kit holds them.
	Items []shared.ItemID
}

// Error lists the archived items, for logs.
func (e *ArchivedItemsError) Error() string {
	return fmt.Sprintf("catalog: %d archived item(s): %v", len(e.Items), e.Items)
}

// Unwrap makes errors.Is(err, ErrItemArchived) true.
func (e *ArchivedItemsError) Unwrap() error { return ErrItemArchived }

// Field names (those of docs/api/openapi.yaml) and pt-BR messages, from the
// "Catálogo" section of docs/product/design-document.md.
const (
	fieldName            = "name"
	fieldDescription     = "description"
	fieldPrice           = "priceCents"
	fieldSourcingMode    = "sourcingMode"
	fieldProductionDays  = "productionDays"
	fieldProductionCost  = "productionCostCents"
	fieldMainSupplier    = "mainSupplier"
	fieldBackupSupplier  = "backupSupplier"
	fieldSupplierID      = "supplierId"
	fieldCostCents       = "costCents"
	fieldAttributeValues = "attributeValueIds"
	fieldOptions         = "options"
	fieldChoices         = "choices"
	fieldLabel           = "label"
	fieldExtraPrice      = "extraPriceCents"
	fieldExtraDays       = "extraDays"
	fieldValues          = "values"
	fieldPosition        = "position"
	fieldPhotoIDs        = "photoIds"
	fieldSwapDiscount    = "swapDiscount"
	fieldAssemblyDays    = "assemblyDays"
	fieldSafetyMargin    = "safetyMarginDays"
	fieldItems           = "items"
	fieldItemID          = "itemId"
	fieldQuantity        = "quantity"
	fieldAlternatives    = "alternativeItemIds"

	msgItemNameRequired     = "Informe o nome do item."
	msgKitNameRequired      = "Informe o nome do kit."
	msgCategoryNameRequired = "Informe o nome da categoria."
	msgAttributeNameMissing = "Informe o nome do campo."
	msgOptionNameRequired   = "Informe o nome da opção."
	msgNameTooLong          = "Use um nome de até 120 caracteres."
	msgShortNameTooLong     = "Use um nome de até 80 caracteres."
	msgNameControl          = "Use um nome sem quebras de linha ou caracteres de controle."
	msgDescriptionTooLong   = "Use uma descrição de até 2000 caracteres."
	msgItemPrice            = "Informe o preço."
	msgKitPrice             = "Informe o preço do kit."
	msgAmountTooHigh        = "Use um valor de até R$ 1.000.000.000,00."
	msgSourcingMode         = "Escolha a origem do item."

	msgMainSupplier       = "Escolha o fornecedor principal."
	msgBackupSupplier     = "Escolha o fornecedor reserva."
	msgCostNegative       = "Informe um custo de zero ou mais."
	msgBackupDiffers      = "O fornecedor reserva precisa ser outro."
	msgNoSuppliersInHouse = "Produção própria não usa fornecedor."
	msgProductionDays     = "Informe os dias de produção."
	msgProductionRange    = "Use de 1 a 365 dias de produção."
	msgProductionOnly     = "Dias e custo de produção só valem para produção própria."

	msgValueRepeated   = "Cada valor só pode ser escolhido uma vez."
	msgTooManyValues   = "Escolha até 200 valores."
	msgTooManyOptions  = "Use até 20 opções."
	msgNoChoices       = "Adicione ao menos uma escolha."
	msgTooManyChoices  = "Use até 50 escolhas."
	msgChoiceRequired  = "Informe a escolha."
	msgChoiceTooLong   = "Use uma escolha de até 80 caracteres."
	msgChoiceRepeated  = "Já existe uma escolha com este nome."
	msgChoiceControl   = "Use uma escolha sem quebras de linha ou caracteres de controle."
	msgExtraPrice      = "Informe um acréscimo de zero ou mais."
	msgExtraDays       = "Use de 0 a 365 dias a mais."
	msgValueRequired   = "Informe o valor."
	msgValueTooLong    = "Use um valor de até 80 caracteres."
	msgValueControl    = "Use um valor sem quebras de linha ou caracteres de controle."
	msgValueDuplicate  = "Este valor já existe neste campo."
	msgTooManyAttrVals = "Use até 200 valores por campo."
	msgPhotoOrder      = "Liste todas as fotos, cada uma uma vez."
	msgPosition        = "Use uma posição de zero ou mais."

	msgSwapMode        = "Escolha o que acontece ao trocar um item."
	msgDays0To60       = "Use de 0 a 60 dias."
	msgNoKitItems      = "Adicione ao menos um item."
	msgTooManyKitItems = "Use até 50 itens no kit."
	msgKitItemUnknown  = "Escolha um item do catálogo."
	msgKitItemRepeated = "Este item já está no kit."
	msgQuantity        = "Use uma quantidade de 1 a 99."
	msgNoAlternatives  = "Escolha ao menos um item para a troca."
	msgAlternativesOff = "Só itens que o cliente pode trocar têm alternativas."
	msgAlternativeSelf = "O item não pode ser trocado por ele mesmo."
	msgAlternativeDup  = "Esta alternativa já foi escolhida."
	msgTooManyAlts     = "Use até 50 alternativas."
)
