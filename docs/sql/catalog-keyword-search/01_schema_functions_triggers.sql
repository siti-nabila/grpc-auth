-- Set search_path to the explicitly reviewed source schema before execution.
-- Fresh, additive migration. Run once with ON_ERROR_STOP; do not adopt an
-- existing projection/helper silently. Configure lock_timeout in the client.
BEGIN;
LOCK TABLE role, permissions, applications, user_role IN SHARE ROW EXCLUSIVE MODE;

CREATE FUNCTION catalog_search_normalize(raw TEXT) RETURNS TEXT
LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path FROM CURRENT AS $$
    SELECT btrim(regexp_replace(lower(coalesce(raw, '')), '[.[:space:]-]+', ' ', 'g'));
$$;

CREATE FUNCTION catalog_search_query(raw TEXT) RETURNS TSQUERY
LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path FROM CURRENT AS $$
    SELECT plainto_tsquery('simple', catalog_search_normalize(raw));
$$;

CREATE FUNCTION catalog_search_prefix_query(raw TEXT) RETURNS TSQUERY
LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path FROM CURRENT AS $$
    SELECT coalesce(string_agg(
        chr(39) || replace(replace(lexeme, chr(92), chr(92)||chr(92)),
                           chr(39), chr(39)||chr(39)) || chr(39) || ':*', ' & '), '')::tsquery
    FROM unnest(tsvector_to_array(to_tsvector('simple', catalog_search_normalize(raw)))) AS words(lexeme);
$$;

CREATE FUNCTION catalog_search_pattern(raw TEXT) RETURNS TEXT
LANGUAGE SQL IMMUTABLE PARALLEL SAFE SET search_path FROM CURRENT AS $$
    SELECT CASE WHEN normalized = '' THEN NULL ELSE
        '%' || replace(replace(replace(normalized, chr(92), chr(92)||chr(92)),
                                '%', chr(92)||'%'), '_', chr(92)||'_') || '%' END
    FROM (SELECT catalog_search_normalize(raw) AS normalized) value;
$$;

CREATE TABLE role_search (
    role_id INTEGER PRIMARY KEY REFERENCES role(id) ON DELETE CASCADE,
    fts_keyword TSVECTOR NOT NULL,
    fts_lexeme_text TEXT NOT NULL
);

CREATE TABLE permission_search (
    permission_id INTEGER PRIMARY KEY REFERENCES permissions(id) ON DELETE CASCADE,
    fts_keyword TSVECTOR NOT NULL,
    fts_lexeme_text TEXT NOT NULL
);

CREATE TABLE application_search (
    application_id INTEGER PRIMARY KEY REFERENCES applications(id) ON DELETE CASCADE,
    fts_keyword TSVECTOR NOT NULL,
    fts_lexeme_text TEXT NOT NULL
);

CREATE TABLE user_role_search (
    user_role_id INTEGER PRIMARY KEY REFERENCES user_role(id) ON DELETE CASCADE,
    fts_keyword TSVECTOR NOT NULL,
    fts_lexeme_text TEXT NOT NULL
);

CREATE FUNCTION refresh_role_search(source_id INTEGER) RETURNS VOID
LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
DECLARE document TSVECTOR;
BEGIN
    SELECT to_tsvector('simple', catalog_search_normalize(concat_ws(' ', role_name, code))) INTO document
    FROM role WHERE id = source_id FOR SHARE;
    IF NOT FOUND THEN RETURN; END IF;
    INSERT INTO role_search(role_id, fts_keyword, fts_lexeme_text)
    VALUES(source_id, document, array_to_string(tsvector_to_array(document), ' '))
    ON CONFLICT (role_id) DO UPDATE SET
        fts_keyword = EXCLUDED.fts_keyword, fts_lexeme_text = EXCLUDED.fts_lexeme_text;
END;
$$;

CREATE FUNCTION refresh_permission_search(source_id INTEGER) RETURNS VOID
LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
DECLARE document TSVECTOR;
BEGIN
    SELECT to_tsvector('simple', catalog_search_normalize(code)) INTO document
    FROM permissions WHERE id = source_id FOR SHARE;
    IF NOT FOUND THEN RETURN; END IF;
    INSERT INTO permission_search(permission_id, fts_keyword, fts_lexeme_text)
    VALUES(source_id, document, array_to_string(tsvector_to_array(document), ' '))
    ON CONFLICT (permission_id) DO UPDATE SET
        fts_keyword = EXCLUDED.fts_keyword, fts_lexeme_text = EXCLUDED.fts_lexeme_text;
END;
$$;

CREATE FUNCTION refresh_application_search(source_id INTEGER) RETURNS VOID
LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
DECLARE document TSVECTOR;
BEGIN
    SELECT to_tsvector('simple', catalog_search_normalize(concat_ws(' ', code, name))) INTO document
    FROM applications WHERE id = source_id FOR SHARE;
    IF NOT FOUND THEN RETURN; END IF;
    INSERT INTO application_search(application_id, fts_keyword, fts_lexeme_text)
    VALUES(source_id, document, array_to_string(tsvector_to_array(document), ' '))
    ON CONFLICT (application_id) DO UPDATE SET
        fts_keyword = EXCLUDED.fts_keyword, fts_lexeme_text = EXCLUDED.fts_lexeme_text;
END;
$$;

CREATE FUNCTION refresh_user_role_search(source_id INTEGER) RETURNS VOID
LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
DECLARE document TSVECTOR; assigned_role INTEGER;
BEGIN
    -- Lock the source before the projection, so resumable backfill cannot
    -- overwrite a committed reassignment from an older source read.
    SELECT role_id INTO assigned_role FROM user_role WHERE id = source_id FOR SHARE;
    IF NOT FOUND THEN RETURN; END IF;
    SELECT to_tsvector('simple', catalog_search_normalize(role_name)) INTO document
    FROM role WHERE id = assigned_role FOR SHARE;
    IF NOT FOUND THEN RETURN; END IF;
    INSERT INTO user_role_search(user_role_id, fts_keyword, fts_lexeme_text)
    VALUES(source_id, document, array_to_string(tsvector_to_array(document), ' '))
    ON CONFLICT(user_role_id) DO UPDATE SET
        fts_keyword = EXCLUDED.fts_keyword, fts_lexeme_text = EXCLUDED.fts_lexeme_text;
END;
$$;

CREATE FUNCTION trg_catalog_assignment_lock() RETURNS TRIGGER
LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
BEGIN
    IF TG_OP = 'INSERT' THEN
        PERFORM id FROM role WHERE id = NEW.role_id FOR SHARE;
    ELSE
        -- Both old/new roles serialize rename with administrative reassignment.
        PERFORM id FROM role WHERE id IN (OLD.role_id, NEW.role_id) ORDER BY id FOR SHARE;
    END IF;
    RETURN NEW;
END;
$$;

CREATE FUNCTION trg_refresh_user_role_search() RETURNS TRIGGER
LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
BEGIN
    PERFORM refresh_user_role_search(NEW.id);
    RETURN NEW;
END;
$$;

CREATE FUNCTION trg_refresh_role_search() RETURNS TRIGGER
LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
DECLARE document TSVECTOR; assignment_id INTEGER;
BEGIN
    PERFORM refresh_role_search(NEW.id);
    IF TG_OP = 'INSERT' THEN RETURN NEW; END IF;
    IF OLD.role_name IS NOT DISTINCT FROM NEW.role_name THEN RETURN NEW; END IF;
    document := to_tsvector('simple', catalog_search_normalize(NEW.role_name));
    -- Do not lock source assignment rows here: reassign already owns that row
    -- before taking ordered role locks. Only touch projections in key order.
    FOR assignment_id IN SELECT id FROM user_role WHERE role_id = NEW.id ORDER BY id LOOP
        INSERT INTO user_role_search(user_role_id, fts_keyword, fts_lexeme_text)
        VALUES(assignment_id, document, array_to_string(tsvector_to_array(document), ' '))
        ON CONFLICT(user_role_id) DO UPDATE SET
            fts_keyword = EXCLUDED.fts_keyword, fts_lexeme_text = EXCLUDED.fts_lexeme_text;
    END LOOP;
    RETURN NEW;
END;
$$;

CREATE FUNCTION trg_refresh_permission_search() RETURNS TRIGGER
LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
BEGIN PERFORM refresh_permission_search(NEW.id); RETURN NEW; END;
$$;

CREATE FUNCTION trg_refresh_application_search() RETURNS TRIGGER
LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
BEGIN PERFORM refresh_application_search(NEW.id); RETURN NEW; END;
$$;

CREATE TRIGGER role_search_insert AFTER INSERT ON role
FOR EACH ROW EXECUTE FUNCTION trg_refresh_role_search();
CREATE TRIGGER role_search_update AFTER UPDATE OF role_name, code ON role
FOR EACH ROW WHEN (OLD.role_name IS DISTINCT FROM NEW.role_name OR OLD.code IS DISTINCT FROM NEW.code)
EXECUTE FUNCTION trg_refresh_role_search();
CREATE TRIGGER permission_search_insert AFTER INSERT ON permissions
FOR EACH ROW EXECUTE FUNCTION trg_refresh_permission_search();
CREATE TRIGGER permission_search_update AFTER UPDATE OF code ON permissions
FOR EACH ROW WHEN (OLD.code IS DISTINCT FROM NEW.code) EXECUTE FUNCTION trg_refresh_permission_search();
CREATE TRIGGER application_search_insert AFTER INSERT ON applications
FOR EACH ROW EXECUTE FUNCTION trg_refresh_application_search();
CREATE TRIGGER application_search_update AFTER UPDATE OF code, name ON applications
FOR EACH ROW WHEN (OLD.code IS DISTINCT FROM NEW.code OR OLD.name IS DISTINCT FROM NEW.name)
EXECUTE FUNCTION trg_refresh_application_search();
CREATE TRIGGER user_role_search_lock_insert BEFORE INSERT ON user_role
FOR EACH ROW EXECUTE FUNCTION trg_catalog_assignment_lock();
CREATE TRIGGER user_role_search_lock_update BEFORE UPDATE OF role_id ON user_role
FOR EACH ROW WHEN (OLD.role_id IS DISTINCT FROM NEW.role_id) EXECUTE FUNCTION trg_catalog_assignment_lock();
CREATE TRIGGER user_role_search_insert AFTER INSERT ON user_role
FOR EACH ROW EXECUTE FUNCTION trg_refresh_user_role_search();
CREATE TRIGGER user_role_search_update AFTER UPDATE OF role_id ON user_role
FOR EACH ROW WHEN (OLD.role_id IS DISTINCT FROM NEW.role_id) EXECUTE FUNCTION trg_refresh_user_role_search();
COMMIT;
