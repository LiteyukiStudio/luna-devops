-- Restores the interface style column with its previous default. Preferences
-- and site settings reset by the up migration cannot be recovered.

ALTER TABLE public.users
    ADD COLUMN interface_style text DEFAULT ''::text NOT NULL;
