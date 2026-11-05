-- Theme simplification: the interface style (standard/minimal) preference is
-- removed and the brand color catalog shrinks to six single-color presets.
-- Personal presets outside the catalog fall back to following the platform;
-- a site preset outside the catalog falls back to blue.

UPDATE public.users
SET brand_color_preset = ''
WHERE brand_color_preset NOT IN ('', 'blue', 'violet', 'teal', 'green', 'orange', 'pink');

UPDATE public.app_configs
SET value = 'blue', updated_at = now()
WHERE key = 'site.brandColorPreset'
  AND value NOT IN ('blue', 'violet', 'teal', 'green', 'orange', 'pink');

DELETE FROM public.app_configs
WHERE key = 'site.minimalModeDefault';

ALTER TABLE public.users
    DROP COLUMN interface_style;
