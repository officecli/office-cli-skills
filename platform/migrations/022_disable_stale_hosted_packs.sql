-- Disable hosted credit packs whose codes are no longer offered.
-- The application-layer whitelist in billing.Service.Pricing() also filters
-- these out, but flipping enabled=0 keeps the admin UI and any downstream
-- reports consistent with the live catalog.
UPDATE hosted_credit_packs
   SET enabled = 0
 WHERE enabled = 1
   AND code NOT IN ('hosted-100', 'hosted-500', 'hosted-2000');
