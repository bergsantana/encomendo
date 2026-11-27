package br.com.encomendo.core.data.catalog

import br.com.encomendo.core.domain.supplier.Supplier
import br.com.encomendo.core.domain.supplier.SupplierDraft
import br.com.encomendo.core.network.catalog.SupplierDto
import br.com.encomendo.core.network.catalog.SupplierWriteDto

internal fun SupplierDto.toSupplier(): Supplier =
    Supplier(id = id, name = name, whatsappPhone = whatsappPhone, version = version)

internal fun SupplierDraft.toWrite(version: Int?): SupplierWriteDto =
    SupplierWriteDto(name = name, whatsappPhone = whatsappPhone, version = version)
