-- Stage 2 KB Hub billing and entitlement productionization.
-- Adds explicit entitlement plan records, invoice ledger, payout periods, refunds, and disputes.

ALTER TABLE kb_collections
    ADD COLUMN IF NOT EXISTS entitlement_mode TEXT NOT NULL DEFAULT 'free',
    ADD COLUMN IF NOT EXISTS trial_days BIGINT NOT NULL DEFAULT 0,
    ADD COLUMN IF NOT EXISTS billing_interval TEXT NOT NULL DEFAULT 'month',
    ADD COLUMN IF NOT EXISTS currency TEXT NOT NULL DEFAULT 'CNY';

DO $$ BEGIN
    ALTER TABLE kb_collections ADD CONSTRAINT chk_kb_collections_entitlement_mode CHECK (entitlement_mode IN ('free', 'paid', 'trial', 'granted'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE kb_collections ADD CONSTRAINT chk_kb_collections_billing_interval CHECK (billing_interval IN ('none', 'month', 'year'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

ALTER TABLE kb_subscriptions
    ADD COLUMN IF NOT EXISTS entitlement_type TEXT NOT NULL DEFAULT 'free',
    ADD COLUMN IF NOT EXISTS renewal_status TEXT NOT NULL DEFAULT 'none',
    ADD COLUMN IF NOT EXISTS current_period_start TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS current_period_end TIMESTAMPTZ,
    ADD COLUMN IF NOT EXISTS granted_by UUID,
    ADD COLUMN IF NOT EXISTS grant_reason TEXT;

DO $$ BEGIN
    ALTER TABLE kb_subscriptions ADD CONSTRAINT chk_kb_subscriptions_entitlement_type CHECK (entitlement_type IN ('free', 'paid', 'trial', 'granted'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

DO $$ BEGIN
    ALTER TABLE kb_subscriptions ADD CONSTRAINT chk_kb_subscriptions_renewal_status CHECK (renewal_status IN ('none', 'active', 'past_due', 'cancelled', 'expired'));
EXCEPTION
    WHEN duplicate_object THEN NULL;
END $$;

CREATE INDEX IF NOT EXISTS idx_kb_subscriptions_entitlement_type ON kb_subscriptions(entitlement_type);
CREATE INDEX IF NOT EXISTS idx_kb_subscriptions_renewal_status ON kb_subscriptions(renewal_status);
CREATE INDEX IF NOT EXISTS idx_kb_subscriptions_period_end ON kb_subscriptions(current_period_end);

CREATE TABLE IF NOT EXISTS kb_billing_plans (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    collection_id UUID NOT NULL,
    version BIGINT NOT NULL,
    entitlement_type TEXT NOT NULL,
    billing_interval TEXT NOT NULL DEFAULT 'month',
    price BIGINT NOT NULL DEFAULT 0,
    currency TEXT NOT NULL DEFAULT 'CNY',
    trial_days BIGINT NOT NULL DEFAULT 0,
    status TEXT NOT NULL DEFAULT 'active',
    effective_at TIMESTAMPTZ NOT NULL,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_kb_billing_plans_entitlement_type CHECK (entitlement_type IN ('free', 'paid', 'trial', 'granted')),
    CONSTRAINT chk_kb_billing_plans_billing_interval CHECK (billing_interval IN ('none', 'month', 'year')),
    CONSTRAINT chk_kb_billing_plans_status CHECK (status IN ('active', 'archived'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_kb_billing_plans_collection_version ON kb_billing_plans(collection_id, version);
CREATE INDEX IF NOT EXISTS idx_kb_billing_plans_collection_status ON kb_billing_plans(collection_id, status);

CREATE TABLE IF NOT EXISTS kb_invoices (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    user_id UUID NOT NULL,
    collection_id UUID,
    subscription_id UUID,
    status TEXT NOT NULL DEFAULT 'draft',
    currency TEXT NOT NULL DEFAULT 'CNY',
    subtotal BIGINT NOT NULL DEFAULT 0,
    platform_fee BIGINT NOT NULL DEFAULT 0,
    total BIGINT NOT NULL DEFAULT 0,
    period_start TIMESTAMPTZ,
    period_end TIMESTAMPTZ,
    issued_at TIMESTAMPTZ,
    paid_at TIMESTAMPTZ,
    voided_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_kb_invoices_status CHECK (status IN ('draft', 'issued', 'paid', 'void', 'refunded', 'disputed'))
);
CREATE INDEX IF NOT EXISTS idx_kb_invoices_user_status ON kb_invoices(user_id, status);
CREATE INDEX IF NOT EXISTS idx_kb_invoices_collection ON kb_invoices(collection_id);
CREATE INDEX IF NOT EXISTS idx_kb_invoices_subscription ON kb_invoices(subscription_id);

CREATE TABLE IF NOT EXISTS kb_invoice_items (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    invoice_id UUID NOT NULL,
    usage_record_id UUID,
    description TEXT NOT NULL,
    quantity BIGINT NOT NULL DEFAULT 1,
    unit_price BIGINT NOT NULL DEFAULT 0,
    amount BIGINT NOT NULL DEFAULT 0,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb
);
CREATE INDEX IF NOT EXISTS idx_kb_invoice_items_invoice ON kb_invoice_items(invoice_id);
CREATE INDEX IF NOT EXISTS idx_kb_invoice_items_usage_record ON kb_invoice_items(usage_record_id);

CREATE TABLE IF NOT EXISTS contributor_payout_periods (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    contributor_id UUID NOT NULL,
    period TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'open',
    gross_amount BIGINT NOT NULL DEFAULT 0,
    platform_fee BIGINT NOT NULL DEFAULT 0,
    net_amount BIGINT NOT NULL DEFAULT 0,
    paid_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_contributor_payout_periods_status CHECK (status IN ('open', 'pending', 'paid', 'held'))
);
CREATE UNIQUE INDEX IF NOT EXISTS idx_contributor_payout_periods_contributor_period ON contributor_payout_periods(contributor_id, period);
CREATE INDEX IF NOT EXISTS idx_contributor_payout_periods_status ON contributor_payout_periods(status);

CREATE TABLE IF NOT EXISTS kb_refunds (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    invoice_id UUID NOT NULL,
    user_id UUID NOT NULL,
    amount BIGINT NOT NULL,
    reason TEXT NOT NULL,
    status TEXT NOT NULL DEFAULT 'pending',
    processed_by UUID,
    processed_at TIMESTAMPTZ,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_kb_refunds_status CHECK (status IN ('pending', 'approved', 'rejected', 'processed'))
);
CREATE INDEX IF NOT EXISTS idx_kb_refunds_invoice ON kb_refunds(invoice_id);
CREATE INDEX IF NOT EXISTS idx_kb_refunds_user_status ON kb_refunds(user_id, status);

CREATE TABLE IF NOT EXISTS kb_billing_disputes (
    id UUID PRIMARY KEY,
    created_at TIMESTAMPTZ NOT NULL,
    updated_at TIMESTAMPTZ NOT NULL,
    invoice_id UUID NOT NULL,
    user_id UUID NOT NULL,
    reason TEXT NOT NULL,
    detail TEXT,
    status TEXT NOT NULL DEFAULT 'open',
    resolved_by UUID,
    resolved_at TIMESTAMPTZ,
    resolution TEXT,
    metadata JSONB NOT NULL DEFAULT '{}'::jsonb,
    CONSTRAINT chk_kb_billing_disputes_status CHECK (status IN ('open', 'accepted', 'rejected', 'cancelled'))
);
CREATE INDEX IF NOT EXISTS idx_kb_billing_disputes_invoice ON kb_billing_disputes(invoice_id);
CREATE INDEX IF NOT EXISTS idx_kb_billing_disputes_user_status ON kb_billing_disputes(user_id, status);
