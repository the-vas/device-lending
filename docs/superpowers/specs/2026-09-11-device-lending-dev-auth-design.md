# Device Lending Portal — Local Dev Auth Bypass — Design

## Purpose

Today, booting device-lending requires a real, reachable OIDC provider:
`internal/config/config.go` fails fast if `OIDC_ISSUER`/`OIDC_CLIENT_ID`/
`OIDC_CLIENT_SECRET` are missing, and `internal/authsetup/oauth2_provider.go`
does a synchronous OIDC discovery call at boot that is fatal on failure. There
is no way to run the app locally — even just to click through the UI — without
first standing up or registering against a real OIDC provider.

This is an addendum to `2026-07-30-device-lending-design.md`: it adds an
explicit, opt-in local-development mode that seeds two fixed PocketBase user
accounts (a regular user and an admin) with password login, so a developer can
run and exercise the full app on `localhost` with zero OIDC dependency.

## Scope

Strictly a local-development convenience. It must be structurally impossible
for this mode to activate in a real deployment — enforced in code (a
`BASE_URL` guard), not just by documentation telling people not to set the
flag. The OIDC-only auth model described in the base design doc is unchanged
for any non-`localhost` deployment.

Out of scope:
- Configurable credentials/env overrides for the seeded accounts — buttons on
  the dev login page remove any need to type or configure them.
- Arbitrary self-registration of additional local accounts in dev mode — the
  `users` collection's `CreateRule` (locked to `@request.context = 'oauth2'`
  by migration `0002_users_is_admin.go`) stays untouched; dev mode only adds a
  second way to *authenticate* against the two accounts it seeds itself.
- Any change to `docker-compose.yml`'s tracked defaults — this is orthogonal
  to fixing that file's missing `OIDC_*` vars (a separate, already-identified
  issue).

## 1. Config gating (`internal/config`)

- New field: `Config.DevAuth bool`, parsed from `DEV_AUTH` (`strconv.ParseBool`,
  same pattern as `PUBLIC_READ`; default `false`).
- When `DevAuth == true`, `OIDC_ISSUER`/`OIDC_CLIENT_ID`/`OIDC_CLIENT_SECRET`
  are removed from the required-field check in `Load` — the app boots without
  them.
- New validation, evaluated only when `DevAuth == true`: `BASE_URL` must parse
  (via `net/url`) with scheme `http` and hostname `localhost` or `127.0.0.1`
  (any port). Any other value — including a bare parse failure — makes
  `Load` return an error, using the same "fail fast at boot" style as the
  existing `SESSION_SECRET` length check. This is the sole enforcement
  mechanism keeping dev mode off of real deployments.
- When `DevAuth == false` (the default), behavior is byte-for-byte identical
  to today — this feature is purely additive to `Load`.

## 2. Seeding (`internal/devauth`, new package)

- `func Setup(app core.App) error`, invoked from `main.go`'s `OnBootstrap`
  handler in place of `authsetup.ConfigureOAuth2` + `authsetup.BindAdminSync`
  when `cfg.DevAuth` is true. The two branches are mutually exclusive: dev
  mode never calls OIDC discovery or touches OIDC provider config at all.
- Sets `users.PasswordAuth.Enabled = true` on the `users` collection.
  Everything else stays exactly as migration `0002_users_is_admin.go` left it:
  `CreateRule = "@request.context = 'oauth2'"`, `UpdateRule = nil`,
  `DeleteRule = nil`, `OTP`/`MFA` disabled. Turning password auth back on only
  permits *authenticating* against records that already carry a password
  hash; it does not reopen self-registration or self-editing, since those
  remain governed by the untouched `CreateRule`/`UpdateRule`.
- Upserts two fixed records directly via `app.Save()` — the same
  rules-bypassing technique the OIDC callback path already relies on from Go
  code:
  - a regular user: fixed dev-only email/password, `is_admin = false`.
  - an admin user: fixed dev-only email/password, `is_admin = true`.
  - Idempotent across restarts: find-by-email first; create only if missing.
    Existing records (and any data owned by them — devices, requests) are
    left untouched on subsequent boots.

## 3. Login route (`internal/devauth`)

- Three routes, registered in `main.go`'s route block **only when
  `cfg.DevAuth` is true**: `GET /dev/login`, `POST /dev/login/user`,
  `POST /dev/login/admin`. Symmetrically, `/oidc/login` and `/oidc/callback`
  are registered **only when `cfg.DevAuth` is false** — the two auth modes
  never coexist in the route table.
- `GET /dev/login` renders a minimal page: two plain
  `<form method="post">` buttons, "Log in as regular user" / "Log in as
  admin" — no JS, no typed credentials, consistent with the rest of the app's
  server-rendered, htmx-only-where-needed style.
- Each `POST` handler dispatches in-process to PocketBase's own
  `/api/collections/users/auth-with-password` endpoint via
  `apis.NewRouter(app).BuildMux()` — the same in-process-dispatch technique
  `internal/webauth/callback.go`'s `RouterExchanger` already uses for
  `auth-with-oauth2`, just pointed at a different PocketBase endpoint, with
  the fixed seeded email/password for that role.
- On success, the handler sets the identical signed `pb_session` cookie
  (`webauth.SessionCookieName`, `webauth.Signer`) that the real OIDC callback
  sets, then redirects to `/`. Session handling
  (`webauth.LoadSession`, `/logout`) is agnostic to how the underlying
  PocketBase token was obtained, so it works unmodified for dev-mode
  sessions — no changes needed there.

## 4. Testing

- `internal/config`: new table-driven cases —
  - `DEV_AUTH=true` boots successfully without any `OIDC_*` vars set.
  - `DEV_AUTH=true` with a non-`localhost`/non-`127.0.0.1` `BASE_URL` (or an
    `https://` scheme) returns an error.
  - `DEV_AUTH=false` (or unset) preserves every existing required-field
    behavior unchanged (regression coverage).
- `internal/devauth`:
  - `Setup` seeds exactly two records with the expected `is_admin` values and
    flips `PasswordAuth.Enabled`.
  - Calling `Setup` twice does not create duplicate records or error.
  - Route-level test, following `callback_test.go`'s
    `apis.NewRouter(app).BuildMux()` + `httptest` pattern: `POST /dev/login/user`
    and `POST /dev/login/admin` each yield a response with a valid, signer-
    verifiable `pb_session` cookie.
- `main` / route-wiring test: under `DevAuth=true`, `GET /oidc/login` 404s
  (route never registered); under `DevAuth=false`, `GET /dev/login` 404s.
  Confirms the two modes are mutually exclusive at the route-table level, not
  just in intent.

## 5. Documentation

- `device-lending/README.md` gets a new "Local development without OIDC"
  section, placed near the existing "OIDC provider setup" section: what
  `DEV_AUTH=true` does, the `localhost`-only guard and why it exists, the two
  seeded accounts (regular + admin, used to exercise both `is_admin` code
  paths — e.g. `/admin` cleanup view, edit/delete rules), and an explicit
  "never set this in a real deployment" callout.

## Non-goals (recap)

- No configurable seeded-account credentials.
- No self-registration of arbitrary local accounts.
- No changes to `docker-compose.yml`'s tracked OIDC-related defaults.
