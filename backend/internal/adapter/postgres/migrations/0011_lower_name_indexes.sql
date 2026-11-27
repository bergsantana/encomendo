-- Catalog and supplier lists page by lower(name), then id (ADR 0027), so a
-- name sorts the same whatever its case. The (tenant_id, name, id) indexes
-- of 0008 and 0009 cannot serve that order; these expression indexes do,
-- for the keyset pages of the Itens, Kits and supplier lists.

-- +goose Up
DROP INDEX items_tenant_name_idx;
CREATE INDEX items_tenant_lower_name_idx ON items (tenant_id, lower(name), id);
DROP INDEX kits_tenant_name_idx;
CREATE INDEX kits_tenant_lower_name_idx ON kits (tenant_id, lower(name), id);
DROP INDEX suppliers_tenant_name_idx;
CREATE INDEX suppliers_tenant_lower_name_idx ON suppliers (tenant_id, lower(name), id);

-- +goose Down
DROP INDEX suppliers_tenant_lower_name_idx;
CREATE INDEX suppliers_tenant_name_idx ON suppliers (tenant_id, name, id);
DROP INDEX kits_tenant_lower_name_idx;
CREATE INDEX kits_tenant_name_idx ON kits (tenant_id, name, id);
DROP INDEX items_tenant_lower_name_idx;
CREATE INDEX items_tenant_name_idx ON items (tenant_id, name, id);
