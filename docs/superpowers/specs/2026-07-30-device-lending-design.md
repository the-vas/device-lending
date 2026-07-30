# Device Lending Portal — Design

## Purpose

A self-hosted web app for a shared group of people (family, neighbors, coworking
space, etc.) to catalog devices they own (drills, saws, and similar) and offer
them for lending to each other, with a request/approval/handover/return workflow
and email notifications at each step.

## Scope

Single instance, single shared pool of users — everyone can see everyone else's
devices (subject to the read-access toggle below). No multi-tenant/organization
support. Any user can both own devices and borrow devices; there is no separate
owner/borrower role, only an optional admin flag.

## 1. Architecture

A single Go binary that embeds [PocketBase](https://pocketbase.io) as a Go
framework (not the prebuilt CLI binary) and extends it with:

- Custom Go route handlers serving server-rendered HTML (`html/template`) for
  the web UI, using [htmx](https://htmx.org) for in-place dynamic interactions
  (search-as-you-type, accept/reject/withdraw/delete actions) — no JS build
  step, no SPA framework, no Node/npm anywhere in the toolchain.
- Custom Go hooks on PocketBase collections implementing the business rules
  (state machine transitions, email triggers, OIDC claim → admin-flag sync).
- PocketBase's built-in SQLite storage, file storage (device photos), and
  OAuth2/OIDC auth.
- PocketBase's built-in SMTP mailer for notification emails.
- PocketBase's built-in automated backups to S3-compatible storage.

**Why PocketBase stays on SQLite:** PocketBase does not support Postgres as a
storage backend — SQLite is baked into its architecture (JSON1/FTS5, migration
format, built-in backup/restore, realtime subscriptions), not a swappable
option. This was evaluated explicitly and accepted: for this app's write
volume (device edits and lending requests from a small user group), SQLite is
not a scaling risk. Safety for a future Kubernetes deployment is addressed via
a single-replica `Deployment`/`StatefulSet` with a `ReadWriteOnce` PVC, plus
PocketBase's built-in automated S3 backups — not via a multi-writer database
service.

Deployment: single container, single SQLite file + uploads directory as the
only persistent state, mounted as a volume.

## 2. Data Model

### `devices`

| Field | Type | Notes |
|---|---|---|
| name | text | required |
| description | text | |
| location_point | geoPoint | PocketBase's native geoPoint field (lat/lon); set by clicking a map |
| location_label | text, optional | e.g. "Garage", "Shed B" — shown alongside the map pin |
| photo | file (image), optional | |
| category | relation → `categories` | |
| status | select: `available`, `requested`, `lent`, `unavailable` | see state machine below |
| owner | relation → `users` | |
| current_borrower | relation → `users`, nullable | set only while `status = lent` |
| lend_start | date, nullable | auto-set to *now* at handover |
| lend_end | date, nullable | optional, settable at handover |

Map UI: Leaflet.js with OpenStreetMap tiles (both open-source), loaded via a
plain `<script>`/`<link>` tag (vendored locally for full self-hosting, no
external CDN dependency at runtime). Used both for picking a location (device
create/edit form) and displaying it (device detail view).

### `categories`

A managed collection (`name` field) rather than a hardcoded enum, so the list
of categories can grow without a code change. Managed via the PocketBase
admin panel (superuser) rather than a dedicated app UI, since category
changes are infrequent, low-volume config, not day-to-day usage.

### `lending_requests`

| Field | Type | Notes |
|---|---|---|
| device | relation → `devices` | |
| requester | relation → `users` | must be authenticated (so email is known) |
| status | select: `pending`, `accepted`, `rejected`, `withdrawn` | |
| requested_start | date | proposed by requester |
| requested_end | date, nullable | proposed by requester |
| message | text, optional | |
| decided_at | date, nullable | |

An `accepted` request is never deleted by regular users — it is kept
permanently as a read-only history log of past lendings for that device (see
Permissions below).

### `users`

PocketBase's built-in auth collection, populated via OIDC (name + email come
from the provider's claims). An `is_admin` boolean field is synced from the
OIDC `groups` claim on every login (see Auth below).

### State machine (`devices.status`)

- `available` → a new request arrives → `requested`.
- `requested` stays `requested` while ≥1 request on that device is `pending`.
- **Handover**: owner (or admin) picks a recipient — either from the device's
  pending requests, or manually if there was no prior request at all (e.g.
  lending to someone off-platform). The chosen request → `accepted`, device →
  `lent`, `current_borrower` set, `lend_start = now`, `lend_end` optionally
  set. Every other still-`pending` request for that device gets an email: the
  device is now unavailable, including `lend_end` if the owner set one, and
  the count of other pending requesters.
- Owner can explicitly **reject** a specific pending request at any time
  before handover (requester notified immediately). If that was the last
  pending request, device reverts to `available`.
- A requester can **withdraw** their own pending request; the same reversion
  rule applies. Owner is notified on withdrawal.
- While `lent`, new requests can still arrive — device stays `lent`, the new
  request just queues as `pending`.
- **Mark returned**: owner (or admin) clears `current_borrower`/dates; device
  → `requested` if pending requests remain (so a next recipient can be
  picked), else → `available`. All pending requesters are emailed that the
  device is available again.
- `unavailable` is a manual owner toggle from/to any other state (e.g.
  maintenance), independent of requests.

## 3. Auth & Permissions

- **OIDC login**: generic OIDC provider configuration (issuer URL, client
  ID/secret) via environment variables — works against Authentik today,
  portable to Zitadel or any standard OIDC provider later. A Go OAuth2 hook
  maps standard claims (name, email) into the PocketBase user record on
  login/callback. No local registration form; identity is fully delegated to
  the OIDC provider.
- **Admin role from OIDC**: an env var (`OIDC_ADMIN_GROUP=admin`) names a
  group. The same OAuth2 hook checks the OIDC `groups` claim on every login
  and sets `users.is_admin` accordingly, so removing someone from the IdP
  group revokes admin on their next login.
- **Read access** (list/search/view devices): a deploy-time env var toggles
  PocketBase's collection API rule between public (empty rule) and
  authenticated-only (`@request.auth.id != ""`).
- **Write access** (PocketBase API rules):
  - Create device / create request: any authenticated user.
  - Edit device / confirm handover / mark returned / reject a request: device
    `owner` or `is_admin`.
  - Withdraw a request: request `requester` or `is_admin`.
  - **Delete device**: `owner` or `is_admin`. Allowed while status is
    `available`, `unavailable`, or `requested` — never while `lent`. If
    status is `requested`, deletion first auto-rejects all pending requests,
    emailing each requester that the device was removed.
  - **Delete lending_request**: `requester` or `is_admin`. Allowed only while
    status is `pending`, `rejected`, or `withdrawn`. An `accepted` request is
    permanently undeletable by regular users, even after return (kept as
    history). `is_admin` can still delete for cleanup purposes, bypassing
    this restriction.
- Requesting a device always requires authentication, regardless of the read
  toggle, since the requester's email (from the OIDC identity) is required to
  drive notifications.
- **Borrower identity visibility**: `current_borrower`'s name is shown on the
  device detail page to authenticated users, but hidden from anonymous
  visitors when the public-read toggle is on — they just see the device is
  currently `lent`, not to whom.
- PocketBase's own superuser account/admin panel is used only for initial
  deploy configuration (OIDC, mailer, backups) — day-to-day admin cleanup
  happens through the app UI via `is_admin` users, not the PocketBase admin
  panel.

## 4. Notifications (email)

All emails sent via PocketBase's built-in mailer (SMTP configured via env
vars/admin panel), rendered from simple Go `html/template` templates.

| Event | Recipient | Content |
|---|---|---|
| New request created | Device owner | Device name, requester name, proposed dates, optional message |
| Request explicitly rejected | Requester | Device name, rejected |
| Handover confirmed (request accepted) | Requester | Device name, confirmed `lend_start`/`lend_end` |
| Handover confirmed | Other pending requesters for that device | Device now unavailable, `lend_end` if set, count of other pending requesters |
| Device marked returned | All pending requesters for that device | Device is available again |
| Request withdrawn | Device owner | Informational |
| Device deleted while `requested` | All pending requesters | Device was removed by its owner |

## 5. Frontend / UI Pages

Server-rendered HTML (Go `html/template`) with htmx for in-place updates. Base
styling is a small hand-written CSS file (not a classless framework off the
shelf) — simple, one accent color, no heavy shadows/gradients/animations, so it
reads as an intentional simple design rather than a generic template.

- **Browse/search** (`/`): grid of device cards (photo thumbnail, name,
  category, status badge, location label). Filter by category/status, text
  search by name. Public or login-gated per the read-access config flag.
- **Device detail** (`/devices/:id`): full description, photo, embedded map
  with pin, status, category. Conditional actions:
  - Not owner, device `available`/`requested`: "Request to borrow" (proposed
    dates + optional message).
  - Requester has a pending request on it: "Withdraw request".
  - Owner/admin, device `requested`: list of pending requesters with a
    "Hand over to this person" action per row.
  - Owner/admin, device `lent`: "Mark returned".
  - Owner/admin: "Edit", "Delete" (per the delete rules above), "Mark
    unavailable"/"Mark available" toggle.
- **Create/Edit device** (`/devices/new`, `/devices/:id/edit`): name,
  description, category select, map picker + optional location label, photo
  upload.
- **My devices** (`/my/devices`): devices you own, pending-request counts,
  quick actions.
- **My requests** (`/my/requests`): requests you've made, status, withdraw
  action.
- **Admin cleanup view** (`/admin`, `is_admin` only): all devices and requests
  with delete access regardless of ownership.
- **Login/logout**: OIDC redirect flow.

Not included in v1: an in-app notification center (all notifications are
email-only, per the source requirement) and a dedicated lending-history page
beyond what's visible per-device (the `lending_requests` collection retains
history, but there's no dedicated browsing UI for it yet).

## 6. Deployment

- **Dockerfile**: multi-stage build. Go build stage compiles a single static
  binary with templates/CSS/JS assets embedded via `go:embed`. Runtime stage
  is a minimal image (distroless or alpine) containing just that binary.
- **Persistent state**: one volume mounted at PocketBase's data directory
  (SQLite db + uploaded photos). Nothing else is persistent.
- **docker-compose.yml**: single service, one named volume, all configuration
  via environment variables — OIDC issuer/client ID/secret,
  `OIDC_ADMIN_GROUP`, SMTP settings, public-vs-authenticated read toggle,
  optional S3 backup target. 12-factor style, so it can be ported to a
  Kubernetes `Deployment` + PVC later without code changes, if ever needed —
  raw Kubernetes manifests are not part of this deliverable.

## 7. Testing

- **State machine / business logic**: Go unit tests around the hooks (request
  → handover → accepted/pending-notified, reject, withdraw, mark returned,
  delete-with-cascade) using PocketBase's Go test helpers (`tests.NewTestApp`),
  asserting collection records end up in the right state.
- **Permission rules**: tests asserting non-owners can't edit/delete, admins
  can bypass ownership, and the read-access toggle actually gates listing.
- **Emails**: PocketBase's mailer is swappable in tests — assert the right
  email fires for each event without hitting real SMTP.
- **UI**: server-rendered HTML + htmx, not a JS SPA, so no heavy e2e framework
  is warranted. A handful of Go `httptest` requests against key routes (create
  device, submit request, handover) cover that pages render and forms post
  correctly. Manual click-through for visual/UX checks.
