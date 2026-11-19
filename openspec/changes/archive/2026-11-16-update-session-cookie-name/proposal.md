## Why
The bound STS migration (canonical/secure-token-service#87) moves STS to chmike/securecookie v2, which enforces the `__Host-` cookie-name prefix. The session cookie STS issues is renamed from `session_id` to `__Host-session_id`, so the external authorization service must extract the new name, and the spec scenarios naming the old cookie become outdated.

## What Changes
- The user session cookie name expected by the authz dispatch flow changes from `session_id` to `__Host-session_id`.
- Dispatch semantics (single-credential enforcement, conflict rejection, missing-credential rejection) are unchanged; only the cookie name changes.

## Non-goals
- No change to `ExchangeSession`/`ExchangeToken` RPCs, JWT injection, OpenFGA enforcement, or metrics.

## Success Metrics
- All `external-authorization` scenarios pass with the `__Host-session_id` name.
- Zero behavioral drift between spec and code for the dispatch requirement.
