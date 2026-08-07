# Device Lending Portal

Self-hosted web app for cataloging lendable devices (drills, saws, etc.) and
running a request → handover → return lending workflow with email
notifications. Built on [PocketBase](https://pocketbase.io) embedded in a
single Go binary; server-rendered UI with htmx, no Node.js/JS build step.

## Prerequisites

- An OIDC provider (tested against [Authentik](https://goauthentik.io); any
  standard OIDC provider, e.g. Zitadel, should work).
- Docker and Docker Compose, for self-hosted deployment.

## OIDC provider setup

1. Create an OAuth2/OIDC application/provider for this app.
2. Set its redirect URI to `${BASE_URL}/oidc/callback` (must match `BASE_URL`
   exactly, including scheme and no trailing slash).
3. Add a **groups** scope mapping to the client so the `groups` claim is
   included in the userinfo/id_token response — required for admin sync to
   work. Without this, `OIDC_ADMIN_GROUP` has nothing to match against and no
   user will ever become an admin via login.
4. Note the issuer URL, client ID, and client secret for the next step.

## Configuration

Copy `.env.example` to `.env` and fill in:

| Variable | Required | Description |
|---|---|---|
| `OIDC_ISSUER` | yes | Your provider's issuer URL (its `/.well-known/openid-configuration` must be reachable at `<issuer>/.well-known/openid-configuration`) |
| `OIDC_CLIENT_ID` / `OIDC_CLIENT_SECRET` | yes | From your OIDC provider |
| `OIDC_ADMIN_GROUP` | no | Group name granting admin/cleanup rights; leave unset to disable admin sync |
| `BASE_URL` | yes | Public URL this app is reachable at, no trailing slash — must be `https://` in any real deployment, see below |
| `SESSION_SECRET` | yes | Random string, 32+ characters, used to sign session cookies |
| `PUBLIC_READ` | no (default `false`) | `true` to let anyone browse/view devices without logging in; requesting a device always requires login regardless |

### HTTPS is required

`BASE_URL` must be an `https://` URL (a real hostname, not `localhost`) in any
deployment you actually log into. The session cookie this app sets after an
OIDC login is marked `Secure`, so browsers only store and return it over a
secure context — over plain `http://` on a non-`localhost` host the cookie is
silently dropped and login appears to do nothing: you are redirected back to
the home page still logged out, with no error anywhere.

Terminate TLS in front of the container (reverse proxy, ingress, tunnel) and
point `BASE_URL` at that public HTTPS address. `http://localhost:8090` is fine
for local development only, because browsers treat `localhost` as a secure
context.

## Running with Docker Compose

```bash
cp .env.example .env
# edit .env with real values
docker compose up -d --build
```

The app listens on port 8090. All state (SQLite database + uploaded photos)
lives in the `pb_data` named volume.

## First-run setup

On first boot, PocketBase has no superuser account yet. Create one to access
the PocketBase admin panel (used only for initial setup — category
management and configuring automated backups — not for day-to-day app use):

```bash
docker compose exec device-lending ./device-lending superuser upsert admin@example.com <a-strong-password>
```

Then visit `${BASE_URL}/_/` to log into the PocketBase admin panel, where you
can add device categories (Collections → categories) and configure
S3-compatible automated backups (Settings → Backups).

### Email / SMTP settings

The app sends notification emails for the whole lending workflow (new request,
request declined, handover, device now unavailable, device available again,
request withdrawn, device removed). There are no SMTP environment variables:
mail is configured in the PocketBase admin panel you just logged into, under
**Settings → Mail settings** — enable the SMTP server and fill in host, port,
credentials and the sender address/name. Use its "Send test email" button to
confirm the settings before relying on them.

Until SMTP is configured, notification emails simply do not go out: PocketBase
falls back to a local `sendmail` binary, which is not present in this app's
Alpine-based image, so every send fails. This is logged (visit Logs in the
admin panel) but is deliberately not treated as an error by the app — the
underlying action still succeeds and is saved, so a user who submits a request
or hands a device over gets the normal confirmation and the state change
sticks. Only the notification is lost, so people will not be told about
requests they need to act on until you configure SMTP.

## Becoming an admin

Log into the app once via `${BASE_URL}/oidc/login` so your user record
exists, then make sure your account is a member of the `OIDC_ADMIN_GROUP`
group in your OIDC provider. Admin status syncs on every login.
