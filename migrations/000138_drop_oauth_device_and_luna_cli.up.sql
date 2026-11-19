DELETE FROM public.access_tokens
WHERE oauth_application_id = 'oapp_luna_cli';

DELETE FROM public.oauth_applications
WHERE id = 'oapp_luna_cli';

DROP TABLE IF EXISTS public.oauth_device_authorizations;
