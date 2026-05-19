CREATE TABLE IF NOT EXISTS gold_prices (
    id          BIGSERIAL PRIMARY KEY,
    price_date  DATE        NOT NULL,
    recorded_at TIMESTAMPTZ NOT NULL DEFAULT NOW(),
    kadar       VARCHAR(20) NOT NULL,
    harga_beli  BIGINT      NOT NULL,
    CONSTRAINT uq_gold_prices_date_kadar UNIQUE (price_date, kadar)
);

CREATE INDEX IF NOT EXISTS idx_gold_prices_price_date ON gold_prices (price_date);
CREATE INDEX IF NOT EXISTS idx_gold_prices_kadar      ON gold_prices (kadar);
