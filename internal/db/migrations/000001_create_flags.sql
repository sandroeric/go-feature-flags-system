CREATE TABLE IF NOT EXISTS flags (
	key TEXT PRIMARY KEY,
	enabled BOOLEAN NOT NULL DEFAULT false,
	default_variant TEXT NOT NULL,
	version INTEGER NOT NULL DEFAULT 1 CHECK (version >= 0),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS variants (
	flag_key TEXT NOT NULL REFERENCES flags(key) ON DELETE CASCADE,
	name TEXT NOT NULL,
	weight INTEGER NOT NULL CHECK (weight >= 0),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	PRIMARY KEY (flag_key, name)
);

CREATE TABLE IF NOT EXISTS rules (
	id BIGSERIAL PRIMARY KEY,
	flag_key TEXT NOT NULL REFERENCES flags(key) ON DELETE CASCADE,
	attribute TEXT NOT NULL,
	operator TEXT NOT NULL,
	values_json JSONB NOT NULL,
	variant TEXT NOT NULL,
	priority INTEGER NOT NULL CHECK (priority >= 0),
	created_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	updated_at TIMESTAMPTZ NOT NULL DEFAULT now(),
	UNIQUE (flag_key, priority),
	FOREIGN KEY (flag_key, variant) REFERENCES variants(flag_key, name)
);

CREATE INDEX IF NOT EXISTS rules_flag_key_priority_idx ON rules(flag_key, priority);
CREATE INDEX IF NOT EXISTS variants_flag_key_idx ON variants(flag_key);

CREATE OR REPLACE FUNCTION touch_updated_at()
RETURNS TRIGGER AS $$
BEGIN
	NEW.updated_at = now();
	RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS flags_touch_updated_at ON flags;
CREATE TRIGGER flags_touch_updated_at
BEFORE UPDATE ON flags
FOR EACH ROW
EXECUTE FUNCTION touch_updated_at();

DROP TRIGGER IF EXISTS variants_touch_updated_at ON variants;
CREATE TRIGGER variants_touch_updated_at
BEFORE UPDATE ON variants
FOR EACH ROW
EXECUTE FUNCTION touch_updated_at();

DROP TRIGGER IF EXISTS rules_touch_updated_at ON rules;
CREATE TRIGGER rules_touch_updated_at
BEFORE UPDATE ON rules
FOR EACH ROW
EXECUTE FUNCTION touch_updated_at();
