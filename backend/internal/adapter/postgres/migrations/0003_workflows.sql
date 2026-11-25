-- Status workflows (ADR 0004, ADR 0006): the global presets each store picks
-- one of, and their ordered statuses, each mapped to a fixed phase.
--
-- Global, not tenant-scoped: every store reads the same presets, like
-- templates. encomendo_app may only read them. A published (id, version)
-- never changes; a change is a new version row, and orders keep the version
-- they were created with. The workflow domain and its validation land in
-- milestone 4; until then test/integration/postgres checks the seed.

-- +goose Up
CREATE TABLE status_workflows (
    id         uuid        NOT NULL,
    version    int         NOT NULL CHECK (version >= 1),
    key        text        NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{1,62}$'),
    name       text        NOT NULL CHECK (char_length(name) BETWEEN 1 AND 80), -- pt-BR
    -- New stores take the default until niche templates suggest one (ADR 0006).
    -- The one mutable column: publishing a new version of the default preset
    -- moves the flag to it, in the migration that publishes it.
    is_default boolean     NOT NULL DEFAULT false,
    created_at timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (id, version),
    UNIQUE (key, version)
);

CREATE UNIQUE INDEX status_workflows_one_default ON status_workflows (is_default) WHERE is_default;

-- cancelled is implicit in every workflow, so it is never a status row.
CREATE TABLE workflow_statuses (
    workflow_id      uuid NOT NULL,
    workflow_version int  NOT NULL,
    key              text NOT NULL CHECK (key ~ '^[a-z][a-z0-9_]{1,62}$'),
    position         int  NOT NULL CHECK (position >= 1),
    label            text NOT NULL CHECK (char_length(label) BETWEEN 1 AND 60),        -- staff, pt-BR
    client_label     text NOT NULL CHECK (char_length(client_label) BETWEEN 1 AND 60), -- store page, pt-BR
    phase            text NOT NULL CHECK (phase IN
        ('draft', 'awaiting_deposit', 'confirmed', 'sourcing', 'assembling', 'ready', 'handed_over')),
    PRIMARY KEY (workflow_id, workflow_version, key),
    UNIQUE (workflow_id, workflow_version, position),
    FOREIGN KEY (workflow_id, workflow_version) REFERENCES status_workflows (id, version) ON DELETE CASCADE
);

-- The launch presets of docs/architecture.md 4.2. The staff labels are the
-- ones listed there; the client labels are plain words for the tracking
-- page. Both are still to be confirmed with real shops. Client labels never
-- name suppliers or purchases (docs/product/design-document.md, System rules).
INSERT INTO status_workflows (id, version, key, name, is_default) VALUES
    ('01a20ada-2c00-7000-8000-000000000001', 1, 'encomenda_montagem', 'Encomenda com montagem', true),
    ('01a20ada-2c00-7000-8000-000000000002', 1, 'orcamento_primeiro', 'Orçamento primeiro', false),
    ('01a20ada-2c00-7000-8000-000000000003', 1, 'producao_propria', 'Produção própria', false),
    ('01a20ada-2c00-7000-8000-000000000004', 1, 'pronta_entrega', 'Pronta entrega', false);

INSERT INTO workflow_statuses (workflow_id, workflow_version, position, key, label, client_label, phase) VALUES
    ('01a20ada-2c00-7000-8000-000000000001', 1, 1, 'draft', 'Rascunho', 'Pedido recebido', 'draft'),
    ('01a20ada-2c00-7000-8000-000000000001', 1, 2, 'awaiting_deposit', 'Aguardando sinal', 'Aguardando o sinal', 'awaiting_deposit'),
    ('01a20ada-2c00-7000-8000-000000000001', 1, 3, 'purchasing', 'Compras', 'Pedido confirmado', 'sourcing'),
    ('01a20ada-2c00-7000-8000-000000000001', 1, 4, 'items_arriving', 'Itens chegando', 'Itens a caminho', 'sourcing'),
    ('01a20ada-2c00-7000-8000-000000000001', 1, 5, 'assembling', 'Em montagem', 'Em montagem', 'assembling'),
    ('01a20ada-2c00-7000-8000-000000000001', 1, 6, 'ready', 'Pronto', 'Pronto', 'ready'),
    ('01a20ada-2c00-7000-8000-000000000001', 1, 7, 'delivered', 'Entregue', 'Entregue', 'handed_over'),

    ('01a20ada-2c00-7000-8000-000000000002', 1, 1, 'quote', 'Orçamento', 'Orçamento', 'draft'),
    ('01a20ada-2c00-7000-8000-000000000002', 1, 2, 'awaiting_deposit', 'Aguardando sinal', 'Aguardando o sinal', 'awaiting_deposit'),
    ('01a20ada-2c00-7000-8000-000000000002', 1, 3, 'approved', 'Aprovado', 'Pedido aprovado', 'confirmed'),
    ('01a20ada-2c00-7000-8000-000000000002', 1, 4, 'preparing', 'Em preparação', 'Em preparação', 'sourcing'),
    ('01a20ada-2c00-7000-8000-000000000002', 1, 5, 'ready', 'Pronto', 'Pronto', 'ready'),
    ('01a20ada-2c00-7000-8000-000000000002', 1, 6, 'delivered', 'Entregue', 'Entregue', 'handed_over'),

    ('01a20ada-2c00-7000-8000-000000000003', 1, 1, 'draft', 'Rascunho', 'Pedido recebido', 'draft'),
    ('01a20ada-2c00-7000-8000-000000000003', 1, 2, 'awaiting_deposit', 'Aguardando sinal', 'Aguardando o sinal', 'awaiting_deposit'),
    ('01a20ada-2c00-7000-8000-000000000003', 1, 3, 'queued', 'Na fila', 'Na fila de produção', 'sourcing'),
    ('01a20ada-2c00-7000-8000-000000000003', 1, 4, 'in_production', 'Em produção', 'Em produção', 'sourcing'),
    ('01a20ada-2c00-7000-8000-000000000003', 1, 5, 'finishing', 'Acabamento', 'Acabamento', 'assembling'),
    ('01a20ada-2c00-7000-8000-000000000003', 1, 6, 'ready', 'Pronto', 'Pronto', 'ready'),
    ('01a20ada-2c00-7000-8000-000000000003', 1, 7, 'delivered', 'Entregue', 'Entregue', 'handed_over'),

    ('01a20ada-2c00-7000-8000-000000000004', 1, 1, 'draft', 'Rascunho', 'Pedido recebido', 'draft'),
    ('01a20ada-2c00-7000-8000-000000000004', 1, 2, 'awaiting_deposit', 'Aguardando sinal', 'Aguardando o sinal', 'awaiting_deposit'),
    ('01a20ada-2c00-7000-8000-000000000004', 1, 3, 'picking', 'Separando', 'Separando seu pedido', 'sourcing'),
    ('01a20ada-2c00-7000-8000-000000000004', 1, 4, 'ready_for_pickup', 'Pronto para retirada', 'Pronto para retirada', 'ready'),
    ('01a20ada-2c00-7000-8000-000000000004', 1, 5, 'picked_up', 'Retirado', 'Retirado', 'handed_over');

GRANT SELECT ON status_workflows, workflow_statuses TO encomendo_app;

-- +goose Down
DROP TABLE workflow_statuses;
DROP TABLE status_workflows;
