# OfficeCLI Usage-Limits Test Cases

This document lists the main test cases for anonymous hosted trial, account hosted credits, API key access credentials, consumption timing, blocking behavior, and state visibility.

## Platform-Side Cases

- `UL-PLAT-001`: first anonymous Hosted check on a new fingerprint creates trial quota and returns `access_mode=free`
- `UL-PLAT-002`: exhausted anonymous Hosted trial returns `reason_code=free_quota_exhausted` and login guidance
- `UL-PLAT-003`: valid CLI session returns account Hosted access and account hosted credit balance
- `UL-PLAT-004`: missing paid key returns `reason_code=invalid_api_key`
- `UL-PLAT-005`: disabled key returns `reason_code=disabled_api_key`
- `UL-PLAT-006`: expired key returns `reason_code=expired_api_key`
- `UL-PLAT-007`: exhausted account hosted credits returns `reason_code=hosted_credit_exhausted`
- `UL-PLAT-008`: successful anonymous Hosted consume decrements trial quota after generation succeeds
- `UL-PLAT-009`: successful CLI session or API key Hosted consume reserves and settles account hosted credits
- `UL-PLAT-010`: repeated consume with the same `request_id` is idempotent
- `UL-PLAT-011`: concurrent account hosted credit consume does not over-decrement
- `UL-PLAT-012`: anonymous Hosted trial does not merge into the account after login
- `UL-PLAT-013`: checkout, signup, invite, and Discord grants write account hosted credit ledger rows

## CLI-Side Cases

- `UL-CLI-001`: `new` stops before LLM execution when anonymous Hosted trial is exhausted and suggests `officecli login`
- `UL-CLI-002`: `login` prints a browser URL and short code, writes a CLI session token, and clears any local API key
- `UL-CLI-003`: `set-key` prompts when the argument is missing and clears any local CLI session before saving a key
- `UL-CLI-004`: failed key validation does not overwrite the previous config
- `UL-CLI-005`: `whoami` shows anonymous Hosted trial mode and remaining trial quota
- `UL-CLI-006`: `whoami` shows account login mode or API key mode with account hosted credits
- `UL-CLI-007`: exhausted account hosted credits shows a clear quota-exhausted message and points to account Billing
- `UL-CLI-008`: generation success plus failed consume keeps the file result but adds a warning
- `UL-CLI-009`: successful anonymous Hosted generation adds the remaining trial warning
- `UL-CLI-010`: successful account Hosted generation adds the remaining account hosted credits warning
- `UL-CLI-011`: disabled license checks return a bypass message
- `UL-CLI-012`: platform offline without account login or API key returns the original connectivity error
- `UL-CLI-013`: platform offline with account login or API key requires online validation and returns a clearer Hosted Mode error
- `UL-CLI-014`: failed generation must not trigger quota consumption
- `UL-CLI-015`: failed publish must not trigger quota consumption

## Known Gap

- `UL-GAP-001`: app and admin views still need stronger E2E proof after account hosted credit ledger changes
