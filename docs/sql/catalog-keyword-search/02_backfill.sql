-- Installs a maintenance helper only. Invoke batches explicitly with reviewed
-- size and continuation IDs; each call must use its own transaction.
CREATE FUNCTION catalog_search_backfill(
    after_role INTEGER, after_permission INTEGER, after_application INTEGER,
    after_assignment INTEGER, batch_size INTEGER)
RETURNS TABLE(source TEXT, last_id INTEGER, processed INTEGER)
LANGUAGE PLPGSQL SET search_path FROM CURRENT AS $$
DECLARE source_id INTEGER;
BEGIN
    IF batch_size IS NULL OR batch_size < 1 OR
       after_role IS NULL OR after_role < 0 OR
       after_permission IS NULL OR after_permission < 0 OR
       after_application IS NULL OR after_application < 0 OR
       after_assignment IS NULL OR after_assignment < 0 THEN
        RAISE EXCEPTION 'invalid catalog backfill bounds';
    END IF;
    source := 'role'; last_id := after_role; processed := 0;
    FOR source_id IN SELECT id FROM role WHERE id > after_role ORDER BY id LIMIT batch_size LOOP
        PERFORM refresh_role_search(source_id);
        last_id := source_id; processed := processed + 1;
    END LOOP;
    RETURN NEXT;
    source := 'permissions'; last_id := after_permission; processed := 0;
    FOR source_id IN SELECT id FROM permissions WHERE id > after_permission ORDER BY id LIMIT batch_size LOOP
        PERFORM refresh_permission_search(source_id);
        last_id := source_id; processed := processed + 1;
    END LOOP;
    RETURN NEXT;
    source := 'applications'; last_id := after_application; processed := 0;
    FOR source_id IN SELECT id FROM applications WHERE id > after_application ORDER BY id LIMIT batch_size LOOP
        PERFORM refresh_application_search(source_id);
        last_id := source_id; processed := processed + 1;
    END LOOP;
    RETURN NEXT;
    source := 'user_role'; last_id := after_assignment; processed := 0;
    FOR source_id IN SELECT id FROM user_role WHERE id > after_assignment ORDER BY id LIMIT batch_size LOOP
        PERFORM refresh_user_role_search(source_id);
        last_id := source_id; processed := processed + 1;
    END LOOP;
    RETURN NEXT;
END;
$$;
