# Device Lending Portal — Local Dev Auth Bypass — Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Add an opt-in `DEV_AUTH=true` mode to device-lending that seeds two fixed PocketBase user accounts (regular + admin) with password login, so the app can be booted and fully exercised on `localhost` with zero OIDC provider, per `docs/superpowers/specs/2026-09-11-device-lending-dev-auth-design.md`.

**Architecture:** `internal/config.Load` gains a `DevAuth` flag that both relaxes the OIDC-required-fields check and adds a new `BASE_URL`-must-be-localhost guard. A new `internal/devauth` package seeds the two accounts and exposes `/dev/login` routes that dispatch to PocketBase's own `auth-with-password` endpoint in-process (the same technique the existing OIDC callback uses for `auth-with-oauth2`) and set the same signed session cookie. `main.go`'s bootstrap and route wiring branch on `cfg.DevAuth` so the OIDC path and the dev-auth path are mutually exclusive and never both active.

**Tech Stack:** Go, `github.com/pocketbase/pocketbase` (already pinned in `go.mod`), no new dependencies.

## Global Constraints

- `DEV_AUTH` defaults to `false`; every existing test and behavior must be unchanged when it is unset.
- `DEV_AUTH=true` must be structurally rejected by `config.Load` unless `BASE_URL` is `http://localhost...` or `http://127.0.0.1...` — enforced in code, not just documented.
- No configurable seeded-account credentials — the two dev accounts' email/password are fixed constants.
- No self-registration: `users.CreateRule`/`UpdateRule`/`DeleteRule` are never modified by this work — they stay exactly as migration `0002_users_is_admin.go` already set them.
- Every task that touches business logic must have a passing Go test before being considered done; every task that touches a web route must have a test asserting on rendered HTTP output via the `apis.NewRouter(app).BuildMux()` + `httptest` pattern already used in `internal/webauth/callback_test.go`.
- Commit after each task using the repo's existing commit conventions (plain, imperative messages; no unrelated changes bundled in).
- All work happens inside `device-lending/` (module root); file paths below are relative to that directory unless stated otherwise.

---

## Task 1: `DEV_AUTH` config flag and localhost guard

**Files:**
- Modify: `device-lending/internal/config/config.go`
- Modify: `device-lending/internal/config/config_test.go`

**Interfaces:**
- Produces: `Config.DevAuth bool` field; `Load` no longer requires `OIDC_ISSUER`/`OIDC_CLIENT_ID`/`OIDC_CLIENT_SECRET` when `DevAuth` is true, and rejects any `DevAuth=true` config whose `BASE_URL` isn't `http://localhost...` or `http://127.0.0.1...`. Consumed by Tasks 2–4.

- [ ] **Step 1: Write the failing tests**

Append to `internal/config/config_test.go`:

```go
func TestLoad_DevAuthSkipsOIDCRequirement(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"DEV_AUTH":       "true",
		"BASE_URL":       "http://localhost:8090",
		"SESSION_SECRET": "at-least-32-bytes-of-random-secret",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DevAuth {
		t.Error("expected DevAuth to be true")
	}
}

func TestLoad_DevAuthAcceptsLoopbackIP(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"DEV_AUTH":       "true",
		"BASE_URL":       "http://127.0.0.1:8090",
		"SESSION_SECRET": "at-least-32-bytes-of-random-secret",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !cfg.DevAuth {
		t.Error("expected DevAuth to be true")
	}
}

func TestLoad_DevAuthRejectsNonLocalBaseURL(t *testing.T) {
	_, err := Load(envMap(map[string]string{
		"DEV_AUTH":       "true",
		"BASE_URL":       "https://lending.example.com",
		"SESSION_SECRET": "at-least-32-bytes-of-random-secret",
	}))
	if err == nil {
		t.Fatal("expected error for DEV_AUTH=true with a non-localhost BASE_URL")
	}
}

func TestLoad_DevAuthRejectsHTTPSBaseURL(t *testing.T) {
	_, err := Load(envMap(map[string]string{
		"DEV_AUTH":       "true",
		"BASE_URL":       "https://localhost:8090",
		"SESSION_SECRET": "at-least-32-bytes-of-random-secret",
	}))
	if err == nil {
		t.Fatal("expected error for DEV_AUTH=true with an https:// BASE_URL")
	}
}

func TestLoad_DevAuthFalseStillRequiresOIDC(t *testing.T) {
	_, err := Load(envMap(map[string]string{
		"BASE_URL":       "http://localhost:8090",
		"SESSION_SECRET": "at-least-32-bytes-of-random-secret",
	}))
	if err == nil {
		t.Fatal("expected error: DEV_AUTH defaults to false, so OIDC vars are still required")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/... -v`
Expected: FAIL with `unknown field DevAuth in struct literal` / compile error, since `Config.DevAuth` doesn't exist yet.

- [ ] **Step 3: Implement the config changes**

Replace the full contents of `internal/config/config.go` with:

```go
// internal/config/config.go
package config

import (
	"fmt"
	"net/url"
	"strconv"
	"strings"
)

type Config struct {
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCAdminGroup   string
	PublicRead       bool
	BaseURL          string
	SessionSecret    string
	DevAuth          bool
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		OIDCIssuer:       getenv("OIDC_ISSUER"),
		OIDCClientID:     getenv("OIDC_CLIENT_ID"),
		OIDCClientSecret: getenv("OIDC_CLIENT_SECRET"),
		OIDCAdminGroup:   getenv("OIDC_ADMIN_GROUP"),
		BaseURL:          strings.TrimSuffix(getenv("BASE_URL"), "/"),
		SessionSecret:    getenv("SESSION_SECRET"),
	}

	if v := getenv("PUBLIC_READ"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid PUBLIC_READ value %q: %w", v, err)
		}
		cfg.PublicRead = b
	}

	if v := getenv("DEV_AUTH"); v != "" {
		b, err := strconv.ParseBool(v)
		if err != nil {
			return Config{}, fmt.Errorf("invalid DEV_AUTH value %q: %w", v, err)
		}
		cfg.DevAuth = b
	}

	var missing []string
	if !cfg.DevAuth {
		if cfg.OIDCIssuer == "" {
			missing = append(missing, "OIDC_ISSUER")
		}
		if cfg.OIDCClientID == "" {
			missing = append(missing, "OIDC_CLIENT_ID")
		}
		if cfg.OIDCClientSecret == "" {
			missing = append(missing, "OIDC_CLIENT_SECRET")
		}
	}
	if cfg.BaseURL == "" {
		missing = append(missing, "BASE_URL")
	}
	if cfg.SessionSecret == "" {
		missing = append(missing, "SESSION_SECRET")
	}
	if len(missing) > 0 {
		return Config{}, fmt.Errorf("missing required environment variables: %s", strings.Join(missing, ", "))
	}
	if len(cfg.SessionSecret) < 32 {
		return Config{}, fmt.Errorf("SESSION_SECRET must be at least 32 characters, got %d", len(cfg.SessionSecret))
	}

	if cfg.DevAuth {
		if err := requireLocalBaseURL(cfg.BaseURL); err != nil {
			return Config{}, err
		}
	}

	return cfg, nil
}

// requireLocalBaseURL enforces that DEV_AUTH=true can only ever boot against
// a plain-http localhost/127.0.0.1 BASE_URL, so this dev-only auth bypass can
// never activate in a real deployment.
func requireLocalBaseURL(baseURL string) error {
	u, err := url.Parse(baseURL)
	if err != nil {
		return fmt.Errorf("DEV_AUTH=true requires a valid BASE_URL: %w", err)
	}
	if u.Scheme != "http" {
		return fmt.Errorf("DEV_AUTH=true requires BASE_URL to use http, got scheme %q", u.Scheme)
	}
	host := u.Hostname()
	if host != "localhost" && host != "127.0.0.1" {
		return fmt.Errorf("DEV_AUTH=true requires BASE_URL host to be localhost or 127.0.0.1, got %q", host)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/... -v`
Expected: PASS (all tests, including the pre-existing ones from before this task).

- [ ] **Step 5: Run the full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS (nothing outside `internal/config` should be affected yet).

- [ ] **Step 6: Commit**

```bash
git add internal/config/config.go internal/config/config_test.go
git commit -m "Add DEV_AUTH config flag with localhost-only guard"
```

---

## Task 2: Seed dev-mode users (`internal/devauth`)

**Files:**
- Create: `device-lending/internal/devauth/setup.go`
- Test: `device-lending/internal/devauth/setup_test.go`

**Interfaces:**
- Consumes: nothing beyond `core.App` (PocketBase).
- Produces:
  ```go
  package devauth

  const (
      UserEmail     = "dev-user@example.com"
      UserPassword  = "dev-user-password"
      AdminEmail    = "dev-admin@example.com"
      AdminPassword = "dev-admin-password"
  )

  // Setup enables password login on the users collection and seeds the two
  // fixed dev accounts above (idempotent — safe to call on every boot).
  func Setup(app core.App) error
  ```
  Consumed by Task 4's `bindBootstrap` branch, and by Task 3's tests (to have seeded accounts to log in as).

- [ ] **Step 1: Write the failing tests**

```go
// internal/devauth/setup_test.go
package devauth_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestSetup_EnablesPasswordAuthAndSeedsUsers(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := devauth.Setup(app); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if !users.PasswordAuth.Enabled {
		t.Fatal("expected password auth to be enabled")
	}

	regular, err := app.FindAuthRecordByEmail(users, devauth.UserEmail)
	if err != nil {
		t.Fatalf("expected regular dev user to exist: %v", err)
	}
	if regular.GetBool("is_admin") {
		t.Error("expected regular dev user to not be admin")
	}

	admin, err := app.FindAuthRecordByEmail(users, devauth.AdminEmail)
	if err != nil {
		t.Fatalf("expected admin dev user to exist: %v", err)
	}
	if !admin.GetBool("is_admin") {
		t.Error("expected admin dev user to be admin")
	}
}

func TestSetup_IdempotentOnSecondCall(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := devauth.Setup(app); err != nil {
		t.Fatalf("unexpected error on first call: %v", err)
	}
	if err := devauth.Setup(app); err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	records, err := app.FindAllRecords(users)
	if err != nil {
		t.Fatal(err)
	}

	count := 0
	for _, r := range records {
		if r.GetString("email") == devauth.UserEmail || r.GetString("email") == devauth.AdminEmail {
			count++
		}
	}
	if count != 2 {
		t.Fatalf("expected exactly 2 seeded dev users after two Setup calls, got %d", count)
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devauth/... -v`
Expected: FAIL with `no Go files in ...` / `undefined: devauth.Setup` (the package doesn't exist yet).

- [ ] **Step 3: Implement setup.go**

```go
// internal/devauth/setup.go
package devauth

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

// Fixed dev-only credentials. Only ever reachable when config.DevAuth is
// true, which config.Load has already confirmed requires a localhost
// BASE_URL — see internal/config.requireLocalBaseURL. Not configurable by
// design: the /dev/login page's one-click buttons remove any need to know
// or type them.
const (
	UserEmail     = "dev-user@example.com"
	UserPassword  = "dev-user-password"
	AdminEmail    = "dev-admin@example.com"
	AdminPassword = "dev-admin-password"
)

// Setup enables password login on the users collection and seeds two fixed
// accounts (a regular user and an admin) so local development never needs a
// real OIDC provider. Idempotent: safe to call on every boot.
func Setup(app core.App) error {
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return fmt.Errorf("loading users collection: %w", err)
	}

	users.PasswordAuth.Enabled = true
	if err := app.Save(users); err != nil {
		return fmt.Errorf("enabling password auth: %w", err)
	}

	if err := seedUser(app, users, UserEmail, UserPassword, false); err != nil {
		return err
	}
	if err := seedUser(app, users, AdminEmail, AdminPassword, true); err != nil {
		return err
	}

	return nil
}

func seedUser(app core.App, users *core.Collection, email, password string, isAdmin bool) error {
	if existing, err := app.FindAuthRecordByEmail(users, email); err == nil && existing != nil {
		return nil
	}

	record := core.NewRecord(users)
	record.SetEmail(email)
	record.SetPassword(password)
	record.SetVerified(true)
	record.Set("is_admin", isAdmin)

	if err := app.Save(record); err != nil {
		return fmt.Errorf("seeding dev user %q: %w", email, err)
	}
	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devauth/... -v`
Expected: PASS

- [ ] **Step 5: Run the full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/devauth/setup.go internal/devauth/setup_test.go
git commit -m "Seed fixed dev accounts and enable password auth under DEV_AUTH"
```

---

## Task 3: Dev login routes (`internal/devauth`)

**Files:**
- Create: `device-lending/internal/devauth/login.go`
- Test: `device-lending/internal/devauth/login_test.go`

**Interfaces:**
- Consumes: `devauth.UserEmail`/`UserPassword`/`AdminEmail`/`AdminPassword` (Task 2), `webauth.Signer`, `webauth.SessionCookieName`, `webauth.ExchangeResult` (all already exported by the existing `internal/webauth` package — see `internal/webauth/session.go` and `internal/webauth/callback.go`).
- Produces:
  ```go
  package devauth

  // AuthWithPassword dispatches PocketBase's own auth-with-password endpoint
  // in-process (same technique webauth.RouterExchanger uses for auth-with-oauth2).
  func AuthWithPassword(app core.App, identity, password string) (webauth.ExchangeResult, error)

  // LoginPageHandler renders the two-button dev login page.
  func LoginPageHandler() func(e *core.RequestEvent) error

  // LoginAsHandler logs in as the given fixed identity/password and sets the
  // same signed session cookie the real OIDC callback sets.
  func LoginAsHandler(identity, password string, signer webauth.Signer) func(e *core.RequestEvent) error
  ```
  Consumed by Task 4's `bindAuthRoutes`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/devauth/login_test.go
package devauth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devauth"
	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func registerDevLoginRoutes(t *testing.T, app *tests.TestApp, signer webauth.Signer) http.Handler {
	t.Helper()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.GET("/dev/login", devauth.LoginPageHandler())
		e.Router.POST("/dev/login/user", devauth.LoginAsHandler(devauth.UserEmail, devauth.UserPassword, signer))
		e.Router.POST("/dev/login/admin", devauth.LoginAsHandler(devauth.AdminEmail, devauth.AdminPassword, signer))
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}
	return mux
}

func TestLoginAsHandler_RegularUser_SetsSessionCookie(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := devauth.Setup(app); err != nil {
		t.Fatalf("unexpected error seeding dev users: %v", err)
	}

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	mux := registerDevLoginRoutes(t, app, signer)

	req := httptest.NewRequest(http.MethodPost, "/dev/login/user", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Header().Get("Location") != "/" {
		t.Errorf("expected redirect to /, got %s", rec.Header().Get("Location"))
	}

	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == webauth.SessionCookieName {
			found = true
			if c.Value == "" {
				t.Error("expected non-empty session cookie value")
			}
			if _, err := signer.Verify(c.Value); err != nil {
				t.Errorf("expected verifiable session cookie, got error: %v", err)
			}
		}
	}
	if !found {
		t.Fatal("expected session cookie to be set")
	}
}

func TestLoginAsHandler_Admin_SetsSessionCookie(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := devauth.Setup(app); err != nil {
		t.Fatalf("unexpected error seeding dev users: %v", err)
	}

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	mux := registerDevLoginRoutes(t, app, signer)

	req := httptest.NewRequest(http.MethodPost, "/dev/login/admin", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestLoginAsHandler_UnseededAccount_Fails(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	// deliberately skip devauth.Setup: password auth stays disabled, so the
	// login attempt must fail rather than silently succeed.
	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	mux := registerDevLoginRoutes(t, app, signer)

	req := httptest.NewRequest(http.MethodPost, "/dev/login/user", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusFound {
		t.Fatalf("expected login to fail without seeded users, got 302 redirect")
	}
}

func TestLoginPageHandler_Renders(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	mux := registerDevLoginRoutes(t, app, signer)

	req := httptest.NewRequest(http.MethodGet, "/dev/login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devauth/... -v`
Expected: FAIL with `undefined: devauth.LoginPageHandler` / `undefined: devauth.LoginAsHandler`.

- [ ] **Step 3: Implement login.go**

```go
// internal/devauth/login.go
package devauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"

	"github.com/the-vas/device-lending/internal/webauth"
)

// AuthWithPassword dispatches PocketBase's own auth-with-password endpoint
// in-process, the same technique webauth.RouterExchanger uses for
// auth-with-oauth2 — reusing PocketBase's official password-verification
// logic without a real network call.
func AuthWithPassword(app core.App, identity, password string) (webauth.ExchangeResult, error) {
	router, err := apis.NewRouter(app)
	if err != nil {
		return webauth.ExchangeResult{}, fmt.Errorf("building router: %w", err)
	}

	mux, err := router.BuildMux()
	if err != nil {
		return webauth.ExchangeResult{}, fmt.Errorf("building mux: %w", err)
	}

	body, err := json.Marshal(map[string]string{
		"identity": identity,
		"password": password,
	})
	if err != nil {
		return webauth.ExchangeResult{}, err
	}

	req := httptest.NewRequest(http.MethodPost, "/api/collections/users/auth-with-password", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		return webauth.ExchangeResult{}, fmt.Errorf("password auth failed with status %d: %s", rec.Code, rec.Body.String())
	}

	var parsed struct {
		Token  string         `json:"token"`
		Record map[string]any `json:"record"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		return webauth.ExchangeResult{}, fmt.Errorf("parsing auth-with-password response: %w", err)
	}

	return webauth.ExchangeResult{Token: parsed.Token, Record: parsed.Record}, nil
}

const devLoginPage = `<!DOCTYPE html>
<html>
<head><title>Dev login</title></head>
<body>
<h1>Dev login (DEV_AUTH mode)</h1>
<form method="post" action="/dev/login/user"><button type="submit">Log in as regular user</button></form>
<form method="post" action="/dev/login/admin"><button type="submit">Log in as admin</button></form>
</body>
</html>`

// LoginPageHandler renders the two-button dev login page.
func LoginPageHandler() func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		return e.HTML(http.StatusOK, devLoginPage)
	}
}

// LoginAsHandler logs in as the given fixed identity/password via
// AuthWithPassword and sets the same signed session cookie the real OIDC
// callback (webauth.CallbackHandler) sets, so downstream session handling is
// identical regardless of how the token was obtained.
func LoginAsHandler(identity, password string, signer webauth.Signer) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		result, err := AuthWithPassword(e.App, identity, password)
		if err != nil {
			return e.InternalServerError("dev login failed", err)
		}
		if result.Token == "" {
			return e.InternalServerError("dev login returned no token", nil)
		}

		e.SetCookie(&http.Cookie{
			Name:     webauth.SessionCookieName,
			Value:    signer.Sign(result.Token),
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   60 * 60 * 24 * 5,
		})

		return e.Redirect(http.StatusFound, "/")
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devauth/... -v`
Expected: PASS

- [ ] **Step 5: Run the full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add internal/devauth/login.go internal/devauth/login_test.go
git commit -m "Add /dev/login routes backed by PocketBase's password auth"
```

---

## Task 4: Wire dev-auth mode into `main.go`

**Files:**
- Modify: `device-lending/main.go`
- Modify: `device-lending/main_test.go`

**Interfaces:**
- Consumes: `devauth.Setup`, `devauth.LoginPageHandler`, `devauth.LoginAsHandler`, `devauth.UserEmail`/`UserPassword`/`AdminEmail`/`AdminPassword` (Tasks 2–3); `config.Config.DevAuth` (Task 1).
- Produces: `bindAuthRoutes(r *router.Router[*core.RequestEvent], cfg config.Config, signer webauth.Signer)` — extracted so route registration is directly testable without booting a real server. Mutually exclusive with the OIDC routes: exactly one of `/dev/login*` or `/oidc/login`+`/oidc/callback` is ever registered, depending on `cfg.DevAuth`.

- [ ] **Step 1: Write the failing tests**

Append to `main_test.go` (add `"net/http"`, `"net/http/httptest"` are already imported; add two new imports: `"github.com/the-vas/device-lending/internal/devauth"` and `"github.com/the-vas/device-lending/internal/webauth"`):

```go
func TestBootstrap_DevAuth_SkipsOIDCAndSeedsUsers(t *testing.T) {
	app := core.NewBaseApp(core.BaseAppConfig{DataDir: t.TempDir()})
	defer app.ResetBootstrapState() //nolint:errcheck // best-effort test cleanup

	cfg := config.Config{
		DevAuth:       true,
		BaseURL:       "http://localhost:8090",
		SessionSecret: "at-least-32-bytes-of-random-secret",
	}

	discover := func(ctx context.Context, issuer string) (oidcdiscovery.Document, error) {
		t.Fatal("OIDC discovery must not be called when DevAuth is true")
		return oidcdiscovery.Document{}, nil
	}

	bindBootstrap(app, cfg, discover)

	if err := app.Bootstrap(); err != nil {
		t.Fatalf("bootstrap under DevAuth failed: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if !users.PasswordAuth.Enabled {
		t.Error("expected password auth to be enabled under DevAuth")
	}
	if _, err := app.FindAuthRecordByEmail(users, devauth.UserEmail); err != nil {
		t.Errorf("expected seeded dev user to exist: %v", err)
	}
}

func assertRouteStatus(t *testing.T, mux http.Handler, method, path string, want int) {
	t.Helper()
	req := httptest.NewRequest(method, path, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != want {
		t.Fatalf("%s %s: expected status %d, got %d", method, path, want, rec.Code)
	}
}

func TestBindAuthRoutes_DevAuth_RegistersDevLoginNotOIDC(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{DevAuth: true, BaseURL: "http://localhost:8090", SessionSecret: "at-least-32-bytes-of-random-secret"}
	signer := webauth.NewSigner(cfg.SessionSecret)

	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		bindAuthRoutes(e.Router, cfg, signer)
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}

	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	assertRouteStatus(t, mux, http.MethodGet, "/dev/login", http.StatusOK)
	assertRouteStatus(t, mux, http.MethodGet, "/oidc/login", http.StatusNotFound)
}

func TestBindAuthRoutes_OIDC_RegistersOIDCNotDevLogin(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}

	cfg := config.Config{BaseURL: "https://lending.example.com", SessionSecret: "at-least-32-bytes-of-random-secret"}
	signer := webauth.NewSigner(cfg.SessionSecret)

	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		bindAuthRoutes(e.Router, cfg, signer)
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}

	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	assertRouteStatus(t, mux, http.MethodGet, "/dev/login", http.StatusNotFound)

	req := httptest.NewRequest(http.MethodGet, "/oidc/login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code == http.StatusNotFound {
		t.Fatal("expected /oidc/login route to be registered")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test . -v`
Expected: FAIL with `undefined: bindAuthRoutes` / `undefined: devauth`.

- [ ] **Step 3: Update imports**

In `main.go`, change the import block to:

```go
import (
	"context"
	"io/fs"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"
	"github.com/pocketbase/pocketbase/tools/router"

	"github.com/the-vas/device-lending/internal/authsetup"
	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/devauth"
	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
	"github.com/the-vas/device-lending/internal/web"
	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)
```

(this adds `"github.com/pocketbase/pocketbase/tools/router"` and `"github.com/the-vas/device-lending/internal/devauth"`)

- [ ] **Step 4: Branch `bindBootstrap` on `cfg.DevAuth`**

In `main.go`, inside `bindBootstrap`, replace:

```go
		if err := authsetup.ConfigureOAuth2(e.App, cfg, discover); err != nil {
			return err
		}
		authsetup.BindAdminSync(e.App, cfg.OIDCAdminGroup)
		return authsetup.ApplyReadRules(e.App, cfg.PublicRead)
```

with:

```go
		if cfg.DevAuth {
			if err := devauth.Setup(e.App); err != nil {
				return err
			}
		} else {
			if err := authsetup.ConfigureOAuth2(e.App, cfg, discover); err != nil {
				return err
			}
			authsetup.BindAdminSync(e.App, cfg.OIDCAdminGroup)
		}
		return authsetup.ApplyReadRules(e.App, cfg.PublicRead)
```

- [ ] **Step 5: Add `bindAuthRoutes`**

In `main.go`, add this new function directly after `bindBootstrap`'s closing brace:

```go
// bindAuthRoutes registers either the dev-mode password-login routes or the
// real OIDC login/callback routes, mutually exclusively, depending on
// cfg.DevAuth. Kept as its own function (rather than inline in main) so
// route registration can be exercised directly in tests without booting a
// real server.
func bindAuthRoutes(r *router.Router[*core.RequestEvent], cfg config.Config, signer webauth.Signer) {
	if cfg.DevAuth {
		r.GET("/dev/login", devauth.LoginPageHandler())
		r.POST("/dev/login/user", devauth.LoginAsHandler(devauth.UserEmail, devauth.UserPassword, signer))
		r.POST("/dev/login/admin", devauth.LoginAsHandler(devauth.AdminEmail, devauth.AdminPassword, signer))
		return
	}
	r.GET("/oidc/login", webauth.LoginHandler(cfg.BaseURL, signer))
	r.GET("/oidc/callback", webauth.CallbackHandler(cfg.BaseURL, signer, webauth.RouterExchanger))
}
```

- [ ] **Step 6: Use `bindAuthRoutes` in `main`'s route wiring**

In `main.go`'s `func main()`, inside the `app.OnServe().BindFunc(...)` block, replace:

```go
		se.Router.GET("/oidc/login", webauth.LoginHandler(cfg.BaseURL, signer))
		se.Router.GET("/oidc/callback", webauth.CallbackHandler(cfg.BaseURL, signer, webauth.RouterExchanger))
		se.Router.POST("/logout", webauth.LogoutHandler())
```

with:

```go
		bindAuthRoutes(se.Router, cfg, signer)
		se.Router.POST("/logout", webauth.LogoutHandler())
```

- [ ] **Step 7: Run tests to verify they pass**

Run: `go test . -v`
Expected: PASS

- [ ] **Step 8: Run the full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 9: Manually verify the real binary boots in dev-auth mode**

```bash
DEV_AUTH=true BASE_URL=http://localhost:8091 SESSION_SECRET=at-least-32-bytes-of-random-secret go run . serve --http=127.0.0.1:8091 &
sleep 1
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8091/dev/login       # expect 200
curl -s -o /dev/null -w "%{http_code}\n" http://127.0.0.1:8091/oidc/login      # expect 404
curl -s -i -X POST http://127.0.0.1:8091/dev/login/admin | head -5             # expect 302 with a Set-Cookie: pb_session=... header
kill %1
```

- [ ] **Step 10: Commit**

```bash
git add main.go main_test.go
git commit -m "Wire DEV_AUTH mode into bootstrap and route registration"
```

---

## Task 5: Document `DEV_AUTH` in the README

**Files:**
- Modify: `device-lending/README.md`

**Interfaces:** None (documentation only).

- [ ] **Step 1: Add `DEV_AUTH` to the configuration table**

In `README.md`, in the `## Configuration` table, add a new row after the `PUBLIC_READ` row:

```markdown
| `DEV_AUTH` | no (default `false`) | `true` to skip OIDC entirely for local development — see "Local development without OIDC" below |
```

- [ ] **Step 2: Add a new "Local development without OIDC" section**

In `README.md`, add this new section immediately after the existing "### HTTPS is required" section and before "## Running with Docker Compose":

```markdown
## Local development without OIDC

For running the app on your own machine with no OIDC provider at all, set
`DEV_AUTH=true`. This swaps the entire OIDC login flow for two fixed,
pre-seeded PocketBase accounts you log into with one click:

```bash
DEV_AUTH=true
BASE_URL=http://localhost:8090
SESSION_SECRET=<any 32+ character string>
```

`OIDC_ISSUER`/`OIDC_CLIENT_ID`/`OIDC_CLIENT_SECRET` are not required in this
mode. Boot the app, then visit `${BASE_URL}/dev/login` and click "Log in as
regular user" or "Log in as admin" — no credentials to type, no provider to
register with.

**This is enforced to be local-only.** `DEV_AUTH=true` requires `BASE_URL` to
be `http://localhost...` or `http://127.0.0.1...` — the app refuses to boot
under `DEV_AUTH=true` with any other `BASE_URL`, so this cannot accidentally
end up active in a real deployment. Never set `DEV_AUTH=true` anywhere other
than your own machine.

The two seeded accounts let you exercise both `is_admin` code paths (e.g. the
`/admin` cleanup view, edit/delete permissions) without needing a real OIDC
group claim.
```

- [ ] **Step 3: Commit**

```bash
git add README.md
git commit -m "Document DEV_AUTH local-development mode"
```

---

## Task 6: Make every in-app login redirect and link honor `DEV_AUTH`

**Added after Task 4's implementation surfaced a real gap in this plan:** every unauthenticated-redirect across `internal/web`, and the header template's "Log in" link, hardcode the literal `/oidc/login`. That route doesn't exist when `DEV_AUTH=true` (Task 4 only wired `/dev/login`), so an unauthenticated visitor hitting any protected page — which happens immediately under the default `PUBLIC_READ=false` — gets redirected into a dead route instead of `/dev/login`. This task fixes that so dev-auth mode is actually click-through-usable, not just reachable by typing `/dev/login` manually.

**Files:**
- Create: `device-lending/internal/web/loginpath.go`
- Modify: `device-lending/internal/web/browse.go`
- Modify: `device-lending/internal/web/my_pages.go`
- Modify: `device-lending/internal/web/device_detail.go`
- Modify: `device-lending/internal/web/device_form.go`
- Modify: `device-lending/internal/web/admin.go`
- Modify: `device-lending/internal/web/owner_actions.go`
- Modify: `device-lending/internal/web/templates/layout.html`
- Modify: `device-lending/internal/web/templates_test.go`
- Test: `device-lending/internal/web/browse_test.go` (add a test; existing tests in this file are otherwise unaffected)
- Modify: `device-lending/main.go`
- Modify: `device-lending/main_test.go`

**Interfaces:**
- Produces: `web.LoginPath string` (package-level var, default `"/oidc/login"`) — the single source of truth for where unauthenticated visitors get sent, read by every redirect call site and by every template's `{{.LoginPath}}`. `main.configureLoginPath(cfg config.Config)` sets it to `"/dev/login"` when `cfg.DevAuth` is true, called once from `main()` right after config loads, before anything starts serving.
- Consumes: nothing new from earlier tasks beyond `config.Config.DevAuth` (Task 1).

- [ ] **Step 1: Write the failing tests**

Append to `internal/web/templates_test.go`:

```go
func TestRender_LayoutHonorsConfiguredLoginPath(t *testing.T) {
	defer func() { web.LoginPath = "/oidc/login" }()
	web.LoginPath = "/dev/login"

	html, err := web.Render(nil, map[string]any{
		"Title":       "Browse",
		"CurrentUser": nil,
		"IsAdmin":     false,
		"LoginPath":   web.LoginPath,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, `href="/dev/login"`) {
		t.Error("expected anonymous nav to link to the configured LoginPath")
	}
}
```

Append to `internal/web/browse_test.go`:

```go
func TestBrowseHandler_RedirectsToConfiguredLoginPath(t *testing.T) {
	defer func() { web.LoginPath = "/oidc/login" }()
	web.LoginPath = "/dev/login"

	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.GET("/", web.BrowseHandler(app, false))
	})

	req := httptest.NewRequest(http.MethodGet, "/", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302 redirect to login, got %d", rec.Code)
	}
	if rec.Header().Get("Location") != "/dev/login" {
		t.Errorf("expected redirect to /dev/login, got %s", rec.Header().Get("Location"))
	}
}
```

Append to `main_test.go` (add `"github.com/the-vas/device-lending/internal/web"` to its imports):

```go
func TestConfigureLoginPath(t *testing.T) {
	defer func() { web.LoginPath = "/oidc/login" }()

	web.LoginPath = "/oidc/login"
	configureLoginPath(config.Config{DevAuth: false})
	if web.LoginPath != "/oidc/login" {
		t.Errorf("expected LoginPath to stay /oidc/login when DevAuth is false, got %q", web.LoginPath)
	}

	configureLoginPath(config.Config{DevAuth: true})
	if web.LoginPath != "/dev/login" {
		t.Errorf("expected LoginPath to become /dev/login when DevAuth is true, got %q", web.LoginPath)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/... . -v`
Expected: FAIL — `TestRender_LayoutHonorsConfiguredLoginPath` and `TestBrowseHandler_RedirectsToConfiguredLoginPath` fail because the layout and `BrowseHandler` still hardcode `/oidc/login`; `TestConfigureLoginPath` fails to compile (`undefined: configureLoginPath`, `undefined: web.LoginPath`).

- [ ] **Step 3: Create the shared LoginPath var**

```go
// internal/web/loginpath.go
package web

// LoginPath is the path unauthenticated visitors are redirected to, and what
// the layout template renders as the "Log in" link. Defaults to the real
// OIDC login route; main.go overrides it to "/dev/login" when DEV_AUTH is
// enabled. Set once at boot before the app starts serving — never mutated
// while requests are in flight, so no synchronization is needed.
var LoginPath = "/oidc/login"
```

- [ ] **Step 4: Point every redirect at `LoginPath` instead of the literal**

In `internal/web/browse.go`, replace:

```go
		if !publicRead && e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}
```

with:

```go
		if !publicRead && e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}
```

In `internal/web/my_pages.go`, replace both occurrences of:

```go
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}
```

with:

```go
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}
```

(one inside `MyDevicesHandler`, one inside `MyRequestsHandler` — both have this exact 3-line shape).

In `internal/web/device_detail.go`, replace the occurrence inside `DeviceDetailHandler`:

```go
		if !publicRead && e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}
```

with:

```go
		if !publicRead && e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}
```

and replace both occurrences inside `RequestDeviceHandler` and `WithdrawRequestHandler`:

```go
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}
```

with:

```go
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}
```

In `internal/web/device_form.go`, replace all four occurrences of:

```go
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}
```

with:

```go
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, LoginPath)
		}
```

(one each inside `NewDeviceFormHandler`, `CreateDeviceHandler`, `EditDeviceFormHandler`, `UpdateDeviceHandler`).

In `internal/web/admin.go`, replace inside `requireAdmin`:

```go
func requireAdmin(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.Redirect(http.StatusFound, "/oidc/login")
	}
```

with:

```go
func requireAdmin(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.Redirect(http.StatusFound, LoginPath)
	}
```

In `internal/web/owner_actions.go`, replace inside `requireOwnerOrAdmin`:

```go
func requireOwnerOrAdmin(e *core.RequestEvent, device *core.Record) error {
	if e.Auth == nil {
		return e.Redirect(http.StatusFound, "/oidc/login")
	}
```

with:

```go
func requireOwnerOrAdmin(e *core.RequestEvent, device *core.Record) error {
	if e.Auth == nil {
		return e.Redirect(http.StatusFound, LoginPath)
	}
```

- [ ] **Step 5: Thread `LoginPath` into every template's data**

In `internal/web/browse.go`, in the `Render` call inside `BrowseHandler`, add a `"LoginPath"` key:

```go
		html, err := Render([]string{"templates/index.html"}, map[string]any{
			"Title":       "Browse",
			"CurrentUser": e.Auth,
			"IsAdmin":     isAdmin,
			"Devices":     items,
			"Query":       q,
			"LoginPath":   LoginPath,
		})
```

In `internal/web/my_pages.go`, add `"LoginPath": LoginPath,` to both `Render` call maps (inside `MyDevicesHandler` and `MyRequestsHandler`), e.g. the first becomes:

```go
		html, err := Render([]string{"templates/my_devices.html"}, map[string]any{
			"Title":       "My devices",
			"CurrentUser": e.Auth,
			"IsAdmin":     e.Auth.GetBool("is_admin"),
			"Devices":     items,
			"LoginPath":   LoginPath,
		})
```

and the second (in `MyRequestsHandler`) the same way with its existing `"Requests"` key left in place.

In `internal/web/device_detail.go`, add `"LoginPath": LoginPath,` to the `data := map[string]any{...}` literal inside `DeviceDetailHandler` (the one later passed to `Render([]string{"templates/device_detail.html"}, data)`), alongside its existing `"Title"`/`"CurrentUser"`/etc. keys.

In `internal/web/device_form.go`, add `"LoginPath": LoginPath,` to both `Render` call maps (inside `NewDeviceFormHandler` and `EditDeviceFormHandler`), alongside their existing `"Title"`/`"Categories"`/etc. keys.

In `internal/web/admin.go`, add `"LoginPath": LoginPath,` to the `Render` call map inside `AdminHandler`, alongside its existing keys.

- [ ] **Step 6: Update the layout template**

In `internal/web/templates/layout.html`, replace:

```html
      <a href="/oidc/login">Log in</a>
```

with:

```html
      <a href="{{.LoginPath}}">Log in</a>
```

- [ ] **Step 7: Fix the pre-existing template test for the new required map key**

In `internal/web/templates_test.go`, `TestRender_LayoutWithContent`'s call to `web.Render` currently omits `"LoginPath"`. Add it so the test keeps asserting real behavior instead of a template rendering `<no value>`:

```go
	html, err := web.Render(nil, map[string]any{
		"Title":       "Browse",
		"CurrentUser": nil,
		"IsAdmin":     false,
		"LoginPath":   "/oidc/login",
	})
```

(only the map literal changes; the rest of the test function is unchanged).

- [ ] **Step 8: Add `configureLoginPath` and call it from `main`**

In `main.go`, add this function directly after the `bindAuthRoutes` function (before `func main()`):

```go
// configureLoginPath points web.LoginPath at whichever login route is
// actually registered for this boot — /dev/login under DevAuth, the real
// OIDC login otherwise — so every in-app redirect and the header's "Log in"
// link never point at a route that doesn't exist.
func configureLoginPath(cfg config.Config) {
	if cfg.DevAuth {
		web.LoginPath = "/dev/login"
	}
}
```

In `main.go`'s `func main()`, replace:

```go
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	app := pocketbase.New()
```

with:

```go
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
	configureLoginPath(cfg)

	app := pocketbase.New()
```

- [ ] **Step 9: Run tests to verify they pass**

Run: `go test ./internal/web/... . -v`
Expected: PASS — all three new tests, plus the fixed `TestRender_LayoutWithContent`, plus every pre-existing test in `internal/web` and the root package (none of which changed behavior, since `LoginPath`'s default is still `"/oidc/login"`).

- [ ] **Step 10: Run the full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 11: Manually verify the redirect loop is actually fixed**

```bash
DEV_AUTH=true BASE_URL=http://localhost:8092 SESSION_SECRET=at-least-32-bytes-of-random-secret go run . serve --http=127.0.0.1:8092 &
sleep 1
curl -s -i http://127.0.0.1:8092/ | head -5    # expect: 302 Found, Location: /dev/login (NOT /oidc/login, NOT another 302 loop)
kill %1
```

- [ ] **Step 12: Commit**

```bash
git add internal/web/loginpath.go internal/web/browse.go internal/web/my_pages.go internal/web/device_detail.go internal/web/device_form.go internal/web/admin.go internal/web/owner_actions.go internal/web/templates/layout.html internal/web/templates_test.go internal/web/browse_test.go main.go main_test.go
git commit -m "Point every login redirect and link at the active auth mode's login route"
```
