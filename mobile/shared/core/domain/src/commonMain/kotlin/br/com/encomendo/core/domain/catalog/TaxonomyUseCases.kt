package br.com.encomendo.core.domain.catalog

// Thin on purpose, as the item use cases are.

/** ListCategories reads the store's categories in order. */
class ListCategories(
    private val categories: CategoryRepository,
) {
    /** Returns every category. */
    suspend operator fun invoke(): Fetched<List<Category>> = categories.categories()
}

/** ChangeCategories adds, renames, moves and deletes categories ("Categorias"). */
class ChangeCategories(
    private val categories: CategoryRepository,
) {
    /** Adds a category at the end; throws `Failure.AlreadyTaken` (field `name`) when the name is in use. */
    suspend fun create(name: String): Category = categories.createCategory(name)

    /** Renames the category [id]; throws `Failure.AlreadyTaken` (field `name`) when the name is in use. */
    suspend fun rename(
        id: String,
        name: String,
    ): Category = categories.renameCategory(id, name)

    /** Moves the category [id] to [position]. */
    suspend fun move(
        id: String,
        position: Int,
    ): Category = categories.moveCategory(id, position)

    /** Deletes the category [id]; throws `Failure.CategoryInUse` while items use it. */
    suspend fun delete(id: String) = categories.deleteCategory(id)
}

/** ListAttributes reads the item fields ("Serve em" …) with their values. */
class ListAttributes(
    private val attributes: AttributeRepository,
) {
    /** Returns every field in the store's order. */
    suspend operator fun invoke(): Fetched<List<Attribute>> = attributes.attributes()
}

/** ChangeAttributes adds, replaces and deletes item fields ("Campos"). */
class ChangeAttributes(
    private val attributes: AttributeRepository,
) {
    /** Adds a field with its values; throws `Failure.AlreadyTaken` (field `name`) when the name is in use. */
    suspend fun create(draft: AttributeDraft): Attribute = attributes.createAttribute(draft)

    /** Replaces the field [id]'s name and values; a value left out leaves the items and kits that picked it. */
    suspend fun replace(
        id: String,
        draft: AttributeDraft,
    ): Attribute = attributes.replaceAttribute(id, draft)

    /** Deletes the field [id]; its values leave every item and kit. */
    suspend fun delete(id: String) = attributes.deleteAttribute(id)
}
