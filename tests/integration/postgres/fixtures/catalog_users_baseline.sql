-- Test-only copy of the user-supplied refresh formula and wrapper definitions.
-- public qualification is omitted so every object stays in the owned fixture.
-- No catalog normalization is applied to email/name/phone.
CREATE FUNCTION refresh_user_profile_search(IN p_profile_id BIGINT)
RETURNS VOID LANGUAGE PLPGSQL AS $$
BEGIN
    INSERT INTO user_profile_search(profile_id,fts_keyword,fts_lexeme_text)
    SELECT src.profile_id,src.fts_keyword,
           array_to_string(tsvector_to_array(src.fts_keyword),' ')
    FROM (
        SELECT p.id AS profile_id,
               to_tsvector('simple',concat_ws(' ',a.email,p."name",p.phone,
                   regexp_replace(coalesce(p.phone,''),'\D','','g'))) AS fts_keyword
        FROM profile p JOIN auth a ON a.id=p.user_id
        WHERE p.id=p_profile_id
    ) src
    ON CONFLICT(profile_id) DO UPDATE SET
        fts_keyword=EXCLUDED.fts_keyword,fts_lexeme_text=EXCLUDED.fts_lexeme_text;
END;
$$;

CREATE FUNCTION trg_auth_refresh_user_profile_search() RETURNS TRIGGER
LANGUAGE PLPGSQL AS $$
BEGIN
    PERFORM refresh_user_profile_search(p.id)
    FROM profile p
    WHERE p.user_id=NEW.id;
    RETURN NEW;
END;
$$;

CREATE FUNCTION trg_profile_refresh_user_profile_search() RETURNS TRIGGER
LANGUAGE PLPGSQL AS $$
BEGIN
    PERFORM refresh_user_profile_search(NEW.id);
    RETURN NEW;
END;
$$;

CREATE TRIGGER auth_refresh_user_profile_search_after_email_update
AFTER UPDATE OF email ON auth FOR EACH ROW
WHEN(OLD.email IS DISTINCT FROM NEW.email)
EXECUTE FUNCTION trg_auth_refresh_user_profile_search();

CREATE TRIGGER profile_refresh_user_profile_search_after_insert_update
AFTER INSERT OR UPDATE OF user_id,name,phone ON profile FOR EACH ROW
EXECUTE FUNCTION trg_profile_refresh_user_profile_search();

SELECT refresh_user_profile_search(id) FROM profile;
