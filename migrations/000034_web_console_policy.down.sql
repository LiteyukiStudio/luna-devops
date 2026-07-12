DO $$
BEGIN
    RAISE EXCEPTION 'migration 000034 is irreversible: removing Web Console policy columns would lose explicit disable decisions'
        USING ERRCODE = 'feature_not_supported';
END
$$;
