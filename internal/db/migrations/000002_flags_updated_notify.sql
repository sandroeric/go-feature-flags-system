CREATE OR REPLACE FUNCTION notify_flags_updated()
RETURNS TRIGGER AS $$
BEGIN
	PERFORM pg_notify('flags_updated', '');
	RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS flags_notify_updated ON flags;
CREATE TRIGGER flags_notify_updated
AFTER INSERT OR UPDATE OR DELETE ON flags
FOR EACH STATEMENT
EXECUTE FUNCTION notify_flags_updated();

DROP TRIGGER IF EXISTS variants_notify_updated ON variants;
CREATE TRIGGER variants_notify_updated
AFTER INSERT OR UPDATE OR DELETE ON variants
FOR EACH STATEMENT
EXECUTE FUNCTION notify_flags_updated();

DROP TRIGGER IF EXISTS rules_notify_updated ON rules;
CREATE TRIGGER rules_notify_updated
AFTER INSERT OR UPDATE OR DELETE ON rules
FOR EACH STATEMENT
EXECUTE FUNCTION notify_flags_updated();
