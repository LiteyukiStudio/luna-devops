DELETE FROM app_configs
WHERE key IN (
    'billing.freeQuotaCredits',
    'billing.overdueGracePeriodHours',
    'billing.allowNegativeBalance'
);
