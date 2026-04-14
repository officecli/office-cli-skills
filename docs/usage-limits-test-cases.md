# OfficeCLI Usage-Limits Test Cases

This document lists the main test cases for licensing, free quota, paid quota, consumption timing, blocking behavior, and state visibility.

## Platform-Side Cases

- `UL-PLAT-001`: first free check on a new fingerprint creates quota and returns `access_mode=free`
- `UL-PLAT-002`: exhausted free quota returns `reason_code=free_quota_exhausted`
- `UL-PLAT-003`: valid paid key returns `access_mode=paid`
- `UL-PLAT-004`: missing paid key returns `reason_code=invalid_api_key`
- `UL-PLAT-005`: disabled key returns `reason_code=disabled_api_key`
- `UL-PLAT-006`: expired key returns `reason_code=expired_api_key`
- `UL-PLAT-007`: exhausted paid quota returns `reason_code=paid_quota_exhausted`
- `UL-PLAT-008`: successful free consume decrements remaining quota
- `UL-PLAT-009`: successful paid consume decrements remaining quota
- `UL-PLAT-010`: repeated consume with the same `request_id` is idempotent
- `UL-PLAT-011`: concurrent free consume does not over-decrement
- `UL-PLAT-012`: admin-side quota adjustment affects the next check result

## CLI-Side Cases

- `UL-CLI-001`: `new` stops before LLM execution when free quota is exhausted
- `UL-CLI-002`: `auth set-key` writes a validated paid key successfully
- `UL-CLI-003`: `auth set-key` prompts when the argument is missing
- `UL-CLI-004`: failed key validation does not overwrite the previous config
- `UL-CLI-005`: `auth status` shows free mode and remaining free quota
- `UL-CLI-006`: `auth status` shows paid mode and remaining paid quota
- `UL-CLI-007`: exhausted paid quota shows a clear quota-exhausted message
- `UL-CLI-008`: generation success plus failed consume keeps the file result but adds a warning
- `UL-CLI-009`: successful free-mode generation adds the remaining-free warning
- `UL-CLI-010`: successful paid-mode generation adds the remaining-paid warning
- `UL-CLI-011`: disabled license checks return a bypass message
- `UL-CLI-012`: platform offline without a paid key returns the original connectivity error
- `UL-CLI-013`: platform offline with a paid key requires online validation and returns a clearer paid-mode error
- `UL-CLI-014`: failed generation must not trigger quota consumption
- `UL-CLI-015`: failed publish must not trigger quota consumption

## Known Gap

- `UL-GAP-001`: app and admin views still need stronger E2E proof after admin-side quota changes
