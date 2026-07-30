# Device Lending Portal Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Build a self-hosted, PocketBase-based web app for cataloging lendable devices and running a request → handover → return lending workflow with OIDC auth and email notifications, per `docs/superpowers/specs/2026-07-30-device-lending-design.md`.

**Architecture:** A single Go binary embeds PocketBase as a framework (`github.com/pocketbase/pocketbase`), adds custom collections via Go migrations, custom business logic via Go hooks/service functions, and a server-rendered HTML UI (Go `html/template` + htmx) served through PocketBase's router. OIDC login is a pure server-side redirect flow (no JS SDK): our own `/oidc/login` and `/oidc/callback` routes drive PKCE and exchange the code for a token by dispatching an in-process request through PocketBase's own `/api/collections/users/auth-with-oauth2` endpoint (via `apis.NewRouter(app).BuildMux()`), then store the returned token in an HttpOnly session cookie.

**Tech Stack:** Go 1.23+, `github.com/pocketbase/pocketbase` v0.39.10, SQLite (via PocketBase), htmx (vendored), Leaflet.js + OpenStreetMap tiles (vendored), Docker.

## Global Constraints

- Go module path: `github.com/the-vas/device-lending`, living in `device-lending/` subdirectory of the `utils-box` repo.
- PocketBase version pinned to `v0.39.10` in `go.mod`.
- No Node.js, npm, or any JS build step anywhere in the repo. All frontend JS (htmx, Leaflet) is vendored as plain files, not fetched from a CDN at runtime, so the app is fully self-hosted with zero external runtime dependencies.
- No PocketBase superuser-only admin-panel dependency for day-to-day app usage — only for initial deploy config (OIDC, SMTP, backups).
- All configuration is via environment variables (12-factor), read once at boot in `internal/config`.
- Every task that touches business logic must have a passing Go test before being considered done; every task that touches a web route must have a test asserting on rendered HTTP output via `tests.ApiScenario` or the equivalent manual `apis.NewRouter(app).BuildMux()` + `httptest` pattern.
- Commit after each task using the repo's existing commit conventions (plain, imperative messages; no unrelated changes bundled in).

---

## Task 1: Project scaffolding and health check route

**Files:**
- Create: `device-lending/go.mod`
- Create: `device-lending/main.go`
- Create: `device-lending/internal/migrations/migrations.go`
- Test: `device-lending/main_test.go`

**Interfaces:**
- Produces: a bootable `pocketbase.New()` app in `main.go`, migration auto-run via `migratecmd.MustRegister`, and an empty `package migrations` at `internal/migrations` that later tasks add files to (kept as a stable blank-import target: `_ "github.com/the-vas/device-lending/internal/migrations"`).

- [ ] **Step 1: Initialize the Go module**

```bash
cd device-lending 2>/dev/null || (mkdir device-lending && cd device-lending)
go mod init github.com/the-vas/device-lending
```

- [ ] **Step 2: Create the empty migrations package (import target)**

```go
// internal/migrations/migrations.go
// Package migrations registers this app's custom PocketBase collections.
// Files in this package call migrations.Register(up, down) in an init()
// function; main.go blank-imports this package so they run automatically.
package migrations
```

- [ ] **Step 3: Write main.go**

```go
// main.go
package main

import (
	"log"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func main() {
	app := pocketbase.New()

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: true,
	})

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		se.Router.GET("/healthz", func(e *core.RequestEvent) error {
			return e.String(200, "ok")
		})
		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 4: Fetch dependencies**

```bash
go get github.com/pocketbase/pocketbase@v0.39.10
go mod tidy
```

- [ ] **Step 5: Write the failing test**

```go
// main_test.go
package main

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"
)

func TestHealthz(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}

	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.GET("/healthz", func(re *core.RequestEvent) error {
			return re.String(200, "ok")
		})
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}

	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/healthz", nil)
	mux.ServeHTTP(rec, req)

	if rec.Code != 200 {
		t.Fatalf("expected 200, got %d", rec.Code)
	}
	if rec.Body.String() != "ok" {
		t.Fatalf("expected body 'ok', got %q", rec.Body.String())
	}
}
```

- [ ] **Step 6: Run test to verify it fails**

Run: `go test ./... -run TestHealthz -v`
Expected: FAIL (compile error or route not registered) before `main.go`'s route is wired into the test — since the test above registers the route itself as an inline duplicate of `main.go`'s logic, it should actually PASS once dependencies compile. This step instead just confirms the module builds: run `go build ./...` first and fix any errors, then run the test and confirm PASS (there is no "expect fail" state here since this is a scaffolding smoke test, not TDD-for-new-behavior — skip straight to step 7 if it passes on first run).

- [ ] **Step 7: Run test to verify it passes**

Run: `go test ./... -v`
Expected: PASS

- [ ] **Step 8: Verify the real binary boots**

Run: `go run . serve --http=127.0.0.1:8091 &` then `curl -s http://127.0.0.1:8091/healthz` (expect `ok`), then stop the background process.

- [ ] **Step 9: Commit**

```bash
git add device-lending/go.mod device-lending/go.sum device-lending/main.go device-lending/internal/migrations/migrations.go device-lending/main_test.go
git commit -m "Scaffold PocketBase-embedded Go app with health check route"
```

---

## Task 2: Configuration loading from environment variables

**Files:**
- Create: `device-lending/internal/config/config.go`
- Test: `device-lending/internal/config/config_test.go`

**Interfaces:**
- Produces:
  ```go
  package config

  type Config struct {
      HTTPAddr        string // e.g. "0.0.0.0:8090"
      OIDCIssuer      string
      OIDCClientID    string
      OIDCClientSecret string
      OIDCAdminGroup  string // group name granting is_admin; empty disables admin sync
      PublicRead      bool   // if true, device listing/viewing is open to anonymous users
      BaseURL         string // public base URL used to build the OIDC redirect URI and email links, e.g. "https://lending.example.com"
      SessionSecret   string // HMAC secret for signing the PKCE and session cookies (Task 13)
  }

  // Load reads configuration from environment variables, applying defaults,
  // and returns an error if a required variable is missing or malformed.
  func Load(getenv func(string) string) (Config, error)
  ```

- [ ] **Step 1: Write the failing tests**

```go
// internal/config/config_test.go
package config

import "testing"

func envMap(m map[string]string) func(string) string {
	return func(key string) string { return m[key] }
}

func TestLoad_RequiredFieldsMissing(t *testing.T) {
	_, err := Load(envMap(map[string]string{}))
	if err == nil {
		t.Fatal("expected error when required env vars are missing")
	}
}

func TestLoad_Defaults(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"OIDC_ISSUER":        "https://auth.example.com",
		"OIDC_CLIENT_ID":     "abc",
		"OIDC_CLIENT_SECRET": "secret",
		"BASE_URL":           "https://lending.example.com",
		"SESSION_SECRET":     "at-least-32-bytes-of-random-secret",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != "0.0.0.0:8090" {
		t.Errorf("expected default HTTPAddr, got %q", cfg.HTTPAddr)
	}
	if cfg.PublicRead != false {
		t.Errorf("expected PublicRead to default to false")
	}
	if cfg.OIDCAdminGroup != "" {
		t.Errorf("expected empty admin group by default, got %q", cfg.OIDCAdminGroup)
	}
}

func TestLoad_Overrides(t *testing.T) {
	cfg, err := Load(envMap(map[string]string{
		"OIDC_ISSUER":        "https://auth.example.com",
		"OIDC_CLIENT_ID":     "abc",
		"OIDC_CLIENT_SECRET": "secret",
		"BASE_URL":           "https://lending.example.com",
		"SESSION_SECRET":     "at-least-32-bytes-of-random-secret",
		"HTTP_ADDR":          "0.0.0.0:9000",
		"PUBLIC_READ":        "true",
		"OIDC_ADMIN_GROUP":   "admin",
	}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if cfg.HTTPAddr != "0.0.0.0:9000" {
		t.Errorf("expected overridden HTTPAddr, got %q", cfg.HTTPAddr)
	}
	if !cfg.PublicRead {
		t.Errorf("expected PublicRead true")
	}
	if cfg.OIDCAdminGroup != "admin" {
		t.Errorf("expected admin group 'admin', got %q", cfg.OIDCAdminGroup)
	}
}

func TestLoad_ShortSessionSecretRejected(t *testing.T) {
	_, err := Load(envMap(map[string]string{
		"OIDC_ISSUER":        "https://auth.example.com",
		"OIDC_CLIENT_ID":     "abc",
		"OIDC_CLIENT_SECRET": "secret",
		"BASE_URL":           "https://lending.example.com",
		"SESSION_SECRET":     "too-short",
	}))
	if err == nil {
		t.Fatal("expected error for a SESSION_SECRET shorter than 32 characters")
	}
}

```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/config/... -v`
Expected: FAIL with "undefined: Load" / "undefined: Config"

- [ ] **Step 3: Implement config.go**

```go
// internal/config/config.go
package config

import (
	"fmt"
	"strconv"
	"strings"
)

type Config struct {
	HTTPAddr         string
	OIDCIssuer       string
	OIDCClientID     string
	OIDCClientSecret string
	OIDCAdminGroup   string
	PublicRead       bool
	BaseURL          string
	SessionSecret    string
}

func Load(getenv func(string) string) (Config, error) {
	cfg := Config{
		HTTPAddr:         orDefault(getenv("HTTP_ADDR"), "0.0.0.0:8090"),
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

	var missing []string
	if cfg.OIDCIssuer == "" {
		missing = append(missing, "OIDC_ISSUER")
	}
	if cfg.OIDCClientID == "" {
		missing = append(missing, "OIDC_CLIENT_ID")
	}
	if cfg.OIDCClientSecret == "" {
		missing = append(missing, "OIDC_CLIENT_SECRET")
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

	return cfg, nil
}

func orDefault(v, def string) string {
	if v == "" {
		return def
	}
	return v
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/config/... -v`
Expected: PASS

- [ ] **Step 5: Wire config loading into main.go**

Modify `main.go` to load config at startup and fail fast on error:

```go
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}
```

Add `"os"` and `"github.com/the-vas/device-lending/internal/config"` to imports. `cfg` will be threaded into later tasks (OAuth2 setup, read rules, mail links); for now just log it's loaded: add `log.Printf("config loaded: base_url=%s public_read=%v", cfg.BaseURL, cfg.PublicRead)` right after.

- [ ] **Step 6: Run full test suite and build**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add device-lending/internal/config device-lending/main.go
git commit -m "Add environment-based configuration loading"
```

---

## Task 3: Migration — categories collection

**Files:**
- Create: `device-lending/internal/migrations/0001_categories.go`
- Test: `device-lending/internal/migrations/0001_categories_test.go`

**Interfaces:**
- Produces: a `categories` collection (fields: `name` text, required, unique) that Task 4's `devices` migration relates to by name lookup (`app.FindCollectionByNameOrId("categories")`).

- [ ] **Step 1: Write the failing test**

```go
// internal/migrations/0001_categories_test.go
package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestCategoriesCollectionExists(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	collection, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatalf("expected categories collection to exist: %v", err)
	}

	if collection.Fields.GetByName("name") == nil {
		t.Fatal("expected categories collection to have a 'name' field")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/migrations/... -run TestCategoriesCollectionExists -v`
Expected: FAIL with "expected categories collection to exist"

- [ ] **Step 3: Implement the migration**

```go
// internal/migrations/0001_categories.go
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		collection := core.NewBaseCollection("categories")

		collection.Fields.Add(
			&core.TextField{Name: "name", Required: true, Max: 100},
		)

		collection.Indexes = []string{
			"CREATE UNIQUE INDEX idx_categories_name ON categories (name)",
		}

		collection.ListRule = types.Pointer("")
		collection.ViewRule = types.Pointer("")
		collection.CreateRule = nil
		collection.UpdateRule = nil
		collection.DeleteRule = nil

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("categories")
		if err != nil {
			return err
		}
		return app.Delete(collection)
	})
}
```

Add `"github.com/pocketbase/pocketbase/tools/types"` to the imports (needed for `types.Pointer`). Note: `ListRule`/`ViewRule` set to an empty string pointer means "anyone can list/view" (public), matching the design doc's decision that categories are low-volume config data, not sensitive; `CreateRule`/`UpdateRule`/`DeleteRule` left `nil` means only superusers can manage them (via the PocketBase admin panel), matching the design doc.

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/migrations/... -run TestCategoriesCollectionExists -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/migrations/0001_categories.go device-lending/internal/migrations/0001_categories_test.go
git commit -m "Add categories collection migration"
```

---

## Task 4: Migration — users `is_admin` field

Runs before the `devices`/`lending_requests` migrations (Tasks 5–6) since their API rules reference `@request.auth.is_admin`.

**Files:**
- Create: `device-lending/internal/migrations/0002_users_is_admin.go`
- Test: `device-lending/internal/migrations/0002_users_is_admin_test.go`

**Interfaces:**
- Produces: `users.is_admin` bool field (default `false`), read by API rules in Tasks 5–6 and set by the OIDC admin-sync hook in Task 9.

- [ ] **Step 1: Write the failing test**

```go
// internal/migrations/0002_users_is_admin_test.go
package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestUsersIsAdminFieldExists(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	field := users.Fields.GetByName("is_admin")
	if field == nil {
		t.Fatal("expected users collection to have an 'is_admin' field")
	}
	if field.Type() != "bool" {
		t.Fatalf("expected is_admin to be a bool field, got %s", field.Type())
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/migrations/... -run TestUsersIsAdminFieldExists -v`
Expected: FAIL

- [ ] **Step 3: Implement the migration**

```go
// internal/migrations/0002_users_is_admin.go
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
)

func init() {
	m.Register(func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		users.Fields.Add(&core.BoolField{Name: "is_admin"})

		return app.Save(users)
	}, func(app core.App) error {
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		users.Fields.RemoveByName("is_admin")

		return app.Save(users)
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/migrations/... -run TestUsersIsAdminFieldExists -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/migrations/0002_users_is_admin.go device-lending/internal/migrations/0002_users_is_admin_test.go
git commit -m "Add is_admin field to users collection"
```

---

## Task 5: Migration — devices collection

**Files:**
- Create: `device-lending/internal/migrations/0003_devices.go`
- Test: `device-lending/internal/migrations/0003_devices_test.go`

**Interfaces:**
- Consumes: `categories` collection (Task 3), `users.is_admin` field (Task 4).
- Produces: `devices` collection with fields `name`, `description`, `location_label`, `location_point` (geoPoint), `photo`, `category` (relation), `status` (select: `available`/`requested`/`lent`/`unavailable`), `owner` (relation), `current_borrower` (relation), `lend_start`, `lend_end` (date). `ListRule`/`ViewRule` start as authenticated-only; Task 10 overwrites them based on the `PUBLIC_READ` config toggle. `CreateRule`/`UpdateRule`/`DeleteRule` are static (owner-or-admin, delete blocked while `status = "lent"`) and set here.

- [ ] **Step 1: Write the failing test**

```go
// internal/migrations/0003_devices_test.go
package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestDevicesCollectionExists(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	collection, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatalf("expected devices collection to exist: %v", err)
	}

	for _, name := range []string{
		"name", "description", "location_label", "location_point",
		"photo", "category", "status", "owner", "current_borrower",
		"lend_start", "lend_end",
	} {
		if collection.Fields.GetByName(name) == nil {
			t.Errorf("expected devices collection to have a %q field", name)
		}
	}

	statusField, ok := collection.Fields.GetByName("status").(*core.SelectField)
	if !ok {
		t.Fatal("expected status to be a select field")
	}
	wantValues := []string{"available", "requested", "lent", "unavailable"}
	if len(statusField.Values) != len(wantValues) {
		t.Fatalf("expected %d status values, got %d", len(wantValues), len(statusField.Values))
	}

	if collection.DeleteRule == nil || *collection.DeleteRule == "" {
		t.Fatal("expected a non-empty DeleteRule")
	}
}
```

Add `"github.com/pocketbase/pocketbase/core"` to the test file's imports.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/migrations/... -run TestDevicesCollectionExists -v`
Expected: FAIL

- [ ] **Step 3: Implement the migration**

```go
// internal/migrations/0003_devices.go
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		categories, err := app.FindCollectionByNameOrId("categories")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		collection := core.NewBaseCollection("devices")

		collection.Fields.Add(
			&core.TextField{Name: "name", Required: true, Max: 200},
			&core.TextField{Name: "description", Max: 5000},
			&core.TextField{Name: "location_label", Max: 200},
			&core.GeoPointField{Name: "location_point"},
			&core.FileField{
				Name:      "photo",
				MaxSelect: 1,
				MaxSize:   5 << 20,
				MimeTypes: []string{"image/jpeg", "image/png", "image/webp"},
			},
			&core.RelationField{Name: "category", Required: true, CollectionId: categories.Id, MaxSelect: 1},
			&core.SelectField{
				Name:      "status",
				Required:  true,
				Values:    []string{"available", "requested", "lent", "unavailable"},
				MaxSelect: 1,
			},
			&core.RelationField{Name: "owner", Required: true, CollectionId: users.Id, MaxSelect: 1},
			&core.RelationField{Name: "current_borrower", CollectionId: users.Id, MaxSelect: 1},
			&core.DateField{Name: "lend_start"},
			&core.DateField{Name: "lend_end"},
		)

		// List/View are provisionally authenticated-only; Task 10's boot-time
		// setup overwrites them per the PUBLIC_READ config toggle.
		collection.ListRule = types.Pointer(`@request.auth.id != ""`)
		collection.ViewRule = types.Pointer(`@request.auth.id != ""`)

		collection.CreateRule = types.Pointer(`@request.auth.id != "" && owner = @request.auth.id`)
		collection.UpdateRule = types.Pointer(`@request.auth.id != "" && (owner = @request.auth.id || @request.auth.is_admin = true)`)
		collection.DeleteRule = types.Pointer(`@request.auth.id != "" && (owner = @request.auth.id || @request.auth.is_admin = true) && status != "lent"`)

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("devices")
		if err != nil {
			return err
		}
		return app.Delete(collection)
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/migrations/... -run TestDevicesCollectionExists -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/migrations/0003_devices.go device-lending/internal/migrations/0003_devices_test.go
git commit -m "Add devices collection migration"
```

---

## Task 6: Migration — lending_requests collection

**Files:**
- Create: `device-lending/internal/migrations/0004_lending_requests.go`
- Test: `device-lending/internal/migrations/0004_lending_requests_test.go`

**Interfaces:**
- Consumes: `devices` collection (Task 5), `users.is_admin` field (Task 4).
- Produces: `lending_requests` collection with fields `device` (relation), `requester` (relation), `status` (select: `pending`/`accepted`/`rejected`/`withdrawn`), `requested_start`, `requested_end` (date), `message`, `decided_at` (date). `ListRule`/`ViewRule` restrict to the requester, the related device's owner, or an admin. `CreateRule` requires auth and `requester = @request.auth.id`. `UpdateRule` is `nil` (only admins via superuser panel or our own service-layer code using `app.Save` directly — regular status transitions happen through the dedicated action routes in Tasks 17-21, not raw PATCH). `DeleteRule` implements the history-retention rule from the design doc: the requester can delete their own request only while `status != "accepted"`; an admin can always delete.

- [ ] **Step 1: Write the failing test**

```go
// internal/migrations/0004_lending_requests_test.go
package migrations_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestLendingRequestsCollectionExists(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	collection, err := app.FindCollectionByNameOrId("lending_requests")
	if err != nil {
		t.Fatalf("expected lending_requests collection to exist: %v", err)
	}

	for _, name := range []string{
		"device", "requester", "status", "requested_start",
		"requested_end", "message", "decided_at",
	} {
		if collection.Fields.GetByName(name) == nil {
			t.Errorf("expected lending_requests collection to have a %q field", name)
		}
	}

	if collection.DeleteRule == nil {
		t.Fatal("expected a non-nil DeleteRule")
	}
	if collection.UpdateRule != nil {
		t.Fatal("expected UpdateRule to be nil (status changes go through service-layer code, not the public API)")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/migrations/... -run TestLendingRequestsCollectionExists -v`
Expected: FAIL

- [ ] **Step 3: Implement the migration**

```go
// internal/migrations/0004_lending_requests.go
package migrations

import (
	"github.com/pocketbase/pocketbase/core"
	m "github.com/pocketbase/pocketbase/migrations"
	"github.com/pocketbase/pocketbase/tools/types"
)

func init() {
	m.Register(func(app core.App) error {
		devices, err := app.FindCollectionByNameOrId("devices")
		if err != nil {
			return err
		}
		users, err := app.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		collection := core.NewBaseCollection("lending_requests")

		collection.Fields.Add(
			&core.RelationField{Name: "device", Required: true, CollectionId: devices.Id, MaxSelect: 1},
			&core.RelationField{Name: "requester", Required: true, CollectionId: users.Id, MaxSelect: 1},
			&core.SelectField{
				Name:      "status",
				Required:  true,
				Values:    []string{"pending", "accepted", "rejected", "withdrawn"},
				MaxSelect: 1,
			},
			&core.DateField{Name: "requested_start", Required: true},
			&core.DateField{Name: "requested_end"},
			&core.TextField{Name: "message", Max: 2000},
			&core.DateField{Name: "decided_at"},
		)

		viewRule := `@request.auth.id != "" && (requester = @request.auth.id || device.owner = @request.auth.id || @request.auth.is_admin = true)`
		collection.ListRule = types.Pointer(viewRule)
		collection.ViewRule = types.Pointer(viewRule)
		collection.CreateRule = types.Pointer(`@request.auth.id != "" && requester = @request.auth.id`)
		collection.UpdateRule = nil
		collection.DeleteRule = types.Pointer(`@request.auth.is_admin = true || (requester = @request.auth.id && status != "accepted")`)

		return app.Save(collection)
	}, func(app core.App) error {
		collection, err := app.FindCollectionByNameOrId("lending_requests")
		if err != nil {
			return err
		}
		return app.Delete(collection)
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/migrations/... -run TestLendingRequestsCollectionExists -v`
Expected: PASS

- [ ] **Step 5: Run the full migrations test suite**

Run: `go test ./internal/migrations/... -v`
Expected: PASS (all of Tasks 3–6's tests)

- [ ] **Step 6: Commit**

```bash
git add device-lending/internal/migrations/0004_lending_requests.go device-lending/internal/migrations/0004_lending_requests_test.go
git commit -m "Add lending_requests collection migration"
```

---

## Task 7: OIDC discovery helper

**Files:**
- Create: `device-lending/internal/oidcdiscovery/discovery.go`
- Test: `device-lending/internal/oidcdiscovery/discovery_test.go`

**Interfaces:**
- Produces:
  ```go
  package oidcdiscovery

  type Document struct {
      AuthorizationEndpoint string
      TokenEndpoint         string
      UserinfoEndpoint      string
  }

  // Fetch retrieves and parses the issuer's /.well-known/openid-configuration document.
  func Fetch(ctx context.Context, issuer string) (Document, error)
  ```
  Consumed by Task 8's boot-time OAuth2 provider setup.

- [ ] **Step 1: Write the failing tests**

```go
// internal/oidcdiscovery/discovery_test.go
package oidcdiscovery

import (
	"context"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestFetch_Success(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/.well-known/openid-configuration" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		w.Write([]byte(`{
			"authorization_endpoint": "https://auth.example.com/authorize",
			"token_endpoint": "https://auth.example.com/token",
			"userinfo_endpoint": "https://auth.example.com/userinfo"
		}`))
	}))
	defer srv.Close()

	doc, err := Fetch(context.Background(), srv.URL)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if doc.AuthorizationEndpoint != "https://auth.example.com/authorize" {
		t.Errorf("unexpected AuthorizationEndpoint: %s", doc.AuthorizationEndpoint)
	}
	if doc.TokenEndpoint != "https://auth.example.com/token" {
		t.Errorf("unexpected TokenEndpoint: %s", doc.TokenEndpoint)
	}
	if doc.UserinfoEndpoint != "https://auth.example.com/userinfo" {
		t.Errorf("unexpected UserinfoEndpoint: %s", doc.UserinfoEndpoint)
	}
}

func TestFetch_MissingEndpoints(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error for a discovery document missing required endpoints")
	}
}

func TestFetch_ServerError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer srv.Close()

	_, err := Fetch(context.Background(), srv.URL)
	if err == nil {
		t.Fatal("expected error on non-200 response")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/oidcdiscovery/... -v`
Expected: FAIL with "undefined: Fetch"

- [ ] **Step 3: Implement discovery.go**

```go
// internal/oidcdiscovery/discovery.go
package oidcdiscovery

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
)

type Document struct {
	AuthorizationEndpoint string `json:"authorization_endpoint"`
	TokenEndpoint         string `json:"token_endpoint"`
	UserinfoEndpoint      string `json:"userinfo_endpoint"`
}

func Fetch(ctx context.Context, issuer string) (Document, error) {
	url := strings.TrimSuffix(issuer, "/") + "/.well-known/openid-configuration"

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return Document{}, fmt.Errorf("building discovery request: %w", err)
	}

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return Document{}, fmt.Errorf("fetching discovery document: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return Document{}, fmt.Errorf("discovery endpoint returned status %d", resp.StatusCode)
	}

	var doc Document
	if err := json.NewDecoder(resp.Body).Decode(&doc); err != nil {
		return Document{}, fmt.Errorf("parsing discovery document: %w", err)
	}

	if doc.AuthorizationEndpoint == "" || doc.TokenEndpoint == "" || doc.UserinfoEndpoint == "" {
		return Document{}, fmt.Errorf("discovery document missing required endpoints")
	}

	return doc, nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/oidcdiscovery/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/oidcdiscovery
git commit -m "Add OIDC discovery document fetcher"
```

---

## Task 8: Boot-time OAuth2 provider configuration

Configures the `users` collection's OAuth2 provider from env-derived config (issuer discovery + client credentials), and registers a custom `groups` scope on the `oidc` provider so the admin-sync hook (Task 9) has a claim to read. Must run once at every boot, after migrations and before `app.Start()` begins serving.

**Files:**
- Create: `device-lending/internal/authsetup/oauth2_provider.go`
- Test: `device-lending/internal/authsetup/oauth2_provider_test.go`

**Interfaces:**
- Consumes: `config.Config` (Task 2), `oidcdiscovery.Fetch` (Task 7, injected as a function parameter for testability).
- Produces:
  ```go
  package authsetup

  // RegisterOIDCScopes mutates the package-level OIDC provider factory to
  // request additional scopes (e.g. "groups") beyond PocketBase's default
  // openid/email/profile. Call once, before app.Start().
  func RegisterOIDCScopes(extra ...string)

  // ConfigureOAuth2 upserts the "oidc" provider on the users collection's
  // OAuth2 config using discovered endpoints and the given credentials.
  func ConfigureOAuth2(app core.App, cfg config.Config, discover func(ctx context.Context, issuer string) (oidcdiscovery.Document, error)) error
  ```

- [ ] **Step 1: Write the failing tests**

```go
// internal/authsetup/oauth2_provider_test.go
package authsetup_test

import (
	"context"
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/auth"

	"github.com/the-vas/device-lending/internal/authsetup"
	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func fakeDiscovery(ctx context.Context, issuer string) (oidcdiscovery.Document, error) {
	return oidcdiscovery.Document{
		AuthorizationEndpoint: "https://auth.example.com/authorize",
		TokenEndpoint:         "https://auth.example.com/token",
		UserinfoEndpoint:      "https://auth.example.com/userinfo",
	}, nil
}

func TestConfigureOAuth2(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	cfg := config.Config{
		OIDCIssuer:       "https://auth.example.com",
		OIDCClientID:     "client-123",
		OIDCClientSecret: "secret-456",
	}

	if err := authsetup.ConfigureOAuth2(app, cfg, fakeDiscovery); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}

	if !users.OAuth2.Enabled {
		t.Fatal("expected OAuth2 to be enabled")
	}

	provider, ok := users.OAuth2.GetProviderConfig("oidc")
	if !ok {
		t.Fatal("expected an 'oidc' provider config")
	}
	if provider.ClientId != "client-123" {
		t.Errorf("unexpected ClientId: %s", provider.ClientId)
	}
	if provider.AuthURL != "https://auth.example.com/authorize" {
		t.Errorf("unexpected AuthURL: %s", provider.AuthURL)
	}
	if provider.TokenURL != "https://auth.example.com/token" {
		t.Errorf("unexpected TokenURL: %s", provider.TokenURL)
	}
	if provider.UserInfoURL != "https://auth.example.com/userinfo" {
		t.Errorf("unexpected UserInfoURL: %s", provider.UserInfoURL)
	}

	// calling it again (as happens on every boot) must not create a duplicate provider entry
	if err := authsetup.ConfigureOAuth2(app, cfg, fakeDiscovery); err != nil {
		t.Fatalf("unexpected error on second call: %v", err)
	}
	users, err = app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if len(users.OAuth2.Providers) != 1 {
		t.Fatalf("expected exactly 1 provider after re-running setup, got %d", len(users.OAuth2.Providers))
	}
}

func TestRegisterOIDCScopes(t *testing.T) {
	authsetup.RegisterOIDCScopes("groups")

	provider, err := auth.NewProviderByName(auth.NameOIDC)
	if err != nil {
		t.Fatal(err)
	}

	found := false
	for _, s := range provider.Scopes() {
		if s == "groups" {
			found = true
		}
	}
	if !found {
		t.Errorf("expected 'groups' scope to be registered, got %v", provider.Scopes())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/authsetup/... -v`
Expected: FAIL with "undefined: authsetup.ConfigureOAuth2" / "undefined: authsetup.RegisterOIDCScopes"

- [ ] **Step 3: Implement oauth2_provider.go**

```go
// internal/authsetup/oauth2_provider.go
package authsetup

import (
	"context"
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/auth"

	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
)

func RegisterOIDCScopes(extra ...string) {
	auth.Providers[auth.NameOIDC] = func() auth.Provider {
		p := auth.NewOIDCProvider()
		p.SetScopes(append(p.Scopes(), extra...))
		return p
	}
}

func ConfigureOAuth2(
	app core.App,
	cfg config.Config,
	discover func(ctx context.Context, issuer string) (oidcdiscovery.Document, error),
) error {
	doc, err := discover(context.Background(), cfg.OIDCIssuer)
	if err != nil {
		return fmt.Errorf("discovering OIDC issuer %q: %w", cfg.OIDCIssuer, err)
	}

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		return fmt.Errorf("loading users collection: %w", err)
	}

	users.OAuth2.Enabled = true

	newProvider := core.OAuth2ProviderConfig{
		Name:         "oidc",
		ClientId:     cfg.OIDCClientID,
		ClientSecret: cfg.OIDCClientSecret,
		AuthURL:      doc.AuthorizationEndpoint,
		TokenURL:     doc.TokenEndpoint,
		UserInfoURL:  doc.UserinfoEndpoint,
		DisplayName:  "Log in",
	}

	replaced := false
	for i, p := range users.OAuth2.Providers {
		if p.Name == "oidc" {
			users.OAuth2.Providers[i] = newProvider
			replaced = true
			break
		}
	}
	if !replaced {
		users.OAuth2.Providers = append(users.OAuth2.Providers, newProvider)
	}

	return app.Save(users)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/authsetup/... -v`
Expected: PASS

- [ ] **Step 5: Wire into main.go**

Add to `main.go`, after config loading and before `app.Start()`:

```go
	authsetup.RegisterOIDCScopes("groups")

	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		return authsetup.ConfigureOAuth2(e.App, cfg, oidcdiscovery.Fetch)
	})
```

Add `"github.com/the-vas/device-lending/internal/authsetup"` and `"github.com/the-vas/device-lending/internal/oidcdiscovery"` to imports.

- [ ] **Step 6: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add device-lending/internal/authsetup device-lending/main.go
git commit -m "Configure OIDC OAuth2 provider on boot via discovery"
```

---

## Task 9: Admin role sync from OIDC `groups` claim

**Files:**
- Create: `device-lending/internal/authsetup/admin_sync.go`
- Test: `device-lending/internal/authsetup/admin_sync_test.go`

**Interfaces:**
- Produces:
  ```go
  // HasAdminGroup reports whether the OIDC user's raw claims include the given group.
  func HasAdminGroup(rawUser map[string]any, adminGroup string) bool

  // BindAdminSync registers a hook that sets users.is_admin from the OIDC
  // "groups" claim on every OAuth2 login.
  func BindAdminSync(app core.App, adminGroup string)
  ```
  `HasAdminGroup` is unit-tested directly (pure function). `BindAdminSync`'s wiring is exercised end-to-end by Task 15's OIDC callback test, since simulating a full `OnRecordAuthWithOAuth2Request` trigger in isolation requires a real token exchange; that later end-to-end test is the source of truth for this hook's wiring, and this task's own test only covers the pure decision logic.

- [ ] **Step 1: Write the failing tests**

```go
// internal/authsetup/admin_sync_test.go
package authsetup_test

import (
	"testing"

	"github.com/the-vas/device-lending/internal/authsetup"
)

func TestHasAdminGroup(t *testing.T) {
	cases := []struct {
		name       string
		rawUser    map[string]any
		adminGroup string
		want       bool
	}{
		{"empty admin group disables check", map[string]any{"groups": []any{"admin"}}, "", false},
		{"no groups claim", map[string]any{}, "admin", false},
		{"groups claim wrong type", map[string]any{"groups": "admin"}, "admin", false},
		{"group present", map[string]any{"groups": []any{"users", "admin"}}, "admin", true},
		{"group absent", map[string]any{"groups": []any{"users"}}, "admin", false},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			got := authsetup.HasAdminGroup(c.rawUser, c.adminGroup)
			if got != c.want {
				t.Errorf("HasAdminGroup(%v, %q) = %v, want %v", c.rawUser, c.adminGroup, got, c.want)
			}
		})
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/authsetup/... -run TestHasAdminGroup -v`
Expected: FAIL with "undefined: authsetup.HasAdminGroup"

- [ ] **Step 3: Implement admin_sync.go**

```go
// internal/authsetup/admin_sync.go
package authsetup

import "github.com/pocketbase/pocketbase/core"

func HasAdminGroup(rawUser map[string]any, adminGroup string) bool {
	if adminGroup == "" {
		return false
	}

	raw, ok := rawUser["groups"]
	if !ok {
		return false
	}

	groups, ok := raw.([]any)
	if !ok {
		return false
	}

	for _, g := range groups {
		if s, ok := g.(string); ok && s == adminGroup {
			return true
		}
	}

	return false
}

func BindAdminSync(app core.App, adminGroup string) {
	app.OnRecordAuthWithOAuth2Request("users").BindFunc(func(e *core.RecordAuthWithOAuth2RequestEvent) error {
		if err := e.Next(); err != nil {
			return err
		}

		if e.Record == nil {
			return nil
		}

		isAdmin := HasAdminGroup(e.OAuth2User.RawUser, adminGroup)
		if e.Record.GetBool("is_admin") == isAdmin {
			return nil
		}

		e.Record.Set("is_admin", isAdmin)
		return e.App.Save(e.Record)
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/authsetup/... -run TestHasAdminGroup -v`
Expected: PASS

- [ ] **Step 5: Wire into main.go**

Add after the `ConfigureOAuth2` call inside the `OnBootstrap` hook from Task 8:

```go
		authsetup.BindAdminSync(e.App, cfg.OIDCAdminGroup)
		return nil
```

(Replace the previous `return authsetup.ConfigureOAuth2(...)` with a version that checks the error first, then binds admin sync, then returns nil — see full Step 5 code below.)

```go
	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if err := authsetup.ConfigureOAuth2(e.App, cfg, oidcdiscovery.Fetch); err != nil {
			return err
		}
		authsetup.BindAdminSync(e.App, cfg.OIDCAdminGroup)
		return nil
	})
```

- [ ] **Step 6: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Commit**

```bash
git add device-lending/internal/authsetup device-lending/main.go
git commit -m "Sync is_admin from OIDC groups claim on login"
```

---

## Task 10: Read-access toggle for device listing

**Files:**
- Create: `device-lending/internal/authsetup/read_rules.go`
- Test: `device-lending/internal/authsetup/read_rules_test.go`

**Interfaces:**
- Produces: `func ApplyReadRules(app core.App, publicRead bool) error` — sets `devices.ListRule`/`ViewRule` to `""` (public) or `@request.auth.id != ""` (authenticated-only) per the `PUBLIC_READ` config flag. `categories` stays permanently public (already set in Task 3's migration) and `lending_requests` stays permanently authenticated-only (Task 6) — neither is affected by this toggle, matching the design doc.

- [ ] **Step 1: Write the failing tests**

```go
// internal/authsetup/read_rules_test.go
package authsetup_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/authsetup"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestApplyReadRules_Public(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := authsetup.ApplyReadRules(app, true); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	devices, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	if devices.ListRule == nil || *devices.ListRule != "" {
		t.Errorf("expected empty (public) ListRule, got %v", devices.ListRule)
	}
	if devices.ViewRule == nil || *devices.ViewRule != "" {
		t.Errorf("expected empty (public) ViewRule, got %v", devices.ViewRule)
	}
}

func TestApplyReadRules_AuthenticatedOnly(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	if err := authsetup.ApplyReadRules(app, false); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	devices, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	want := `@request.auth.id != ""`
	if devices.ListRule == nil || *devices.ListRule != want {
		t.Errorf("expected ListRule %q, got %v", want, devices.ListRule)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/authsetup/... -run TestApplyReadRules -v`
Expected: FAIL with "undefined: authsetup.ApplyReadRules"

- [ ] **Step 3: Implement read_rules.go**

```go
// internal/authsetup/read_rules.go
package authsetup

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

func ApplyReadRules(app core.App, publicRead bool) error {
	rule := `@request.auth.id != ""`
	if publicRead {
		rule = ""
	}

	devices, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		return err
	}

	devices.ListRule = types.Pointer(rule)
	devices.ViewRule = types.Pointer(rule)

	return app.Save(devices)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/authsetup/... -run TestApplyReadRules -v`
Expected: PASS

- [ ] **Step 5: Wire into main.go**

Add `authsetup.ApplyReadRules(e.App, cfg.PublicRead)` (with error check) into the same `OnBootstrap` block as Tasks 8–9, after `BindAdminSync`.

- [ ] **Step 6: Run full build and test suite, then commit**

```bash
go build ./... && go test ./...
git add device-lending/internal/authsetup device-lending/main.go
git commit -m "Add PUBLIC_READ toggle for device listing/viewing"
```

---

## Task 11: Guard hook blocking direct edits of device state-machine fields

Prevents a device owner (or anyone with API access) from bypassing the lending state machine by directly `PATCH`ing `status`, `current_borrower`, `lend_start`, or `lend_end` through the public REST API. Those fields may only change through the service-layer functions in Tasks 17–21, which call `app.Save()` directly and are therefore unaffected by this request-scoped hook.

**Files:**
- Create: `device-lending/internal/devices/guard.go`
- Test: `device-lending/internal/devices/guard_test.go`

**Interfaces:**
- Produces: `func BindStateFieldGuard(app core.App)`.

- [ ] **Step 1: Write the failing test**

```go
// internal/devices/guard_test.go
package devices_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func setupDeviceForGuardTest(t *testing.T, app core.App) (owner *core.Record, device *core.Record) {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	owner = core.NewRecord(users)
	owner.SetEmail("owner@example.com")
	owner.SetRandomPassword()
	if err := app.Save(owner); err != nil {
		t.Fatal(err)
	}

	categories, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatal(err)
	}
	category := core.NewRecord(categories)
	category.Set("name", "Power tools")
	if err := app.Save(category); err != nil {
		t.Fatal(err)
	}

	devicesCol, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	device = core.NewRecord(devicesCol)
	device.Set("name", "Drill")
	device.Set("category", category.Id)
	device.Set("owner", owner.Id)
	device.Set("status", "available")
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}

	return owner, device
}

func TestStateFieldGuard_BlocksDirectStatusEdit(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	devices.BindStateFieldGuard(app)

	owner, device := setupDeviceForGuardTest(t, app)

	token, err := owner.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	if err := app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error { return e.Next() }); err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/collections/devices/records/"+device.Id, strings.NewReader(`{"status":"lent"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 blocking direct status edit, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestStateFieldGuard_AllowsNonGuardedFieldEdit(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	devices.BindStateFieldGuard(app)

	owner, device := setupDeviceForGuardTest(t, app)

	token, err := owner.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	if err := app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error { return e.Next() }); err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPatch, "/api/collections/devices/records/"+device.Id, strings.NewReader(`{"description":"cordless, 18V"}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200 allowing a non-guarded field edit, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devices/... -v`
Expected: FAIL with "undefined: devices.BindStateFieldGuard"

- [ ] **Step 3: Implement guard.go**

```go
// internal/devices/guard.go
package devices

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
)

var guardedFields = []string{"status", "current_borrower", "lend_start", "lend_end"}

func BindStateFieldGuard(app core.App) {
	app.OnRecordUpdateRequest("devices").BindFunc(func(e *core.RecordRequestEvent) error {
		original, err := e.App.FindRecordById("devices", e.Record.Id)
		if err != nil {
			return err
		}

		for _, field := range guardedFields {
			if fmt.Sprint(e.Record.Get(field)) != fmt.Sprint(original.Get(field)) {
				return e.BadRequestError(
					fmt.Sprintf("field %q can only be changed via the dedicated action endpoints, not a direct update", field),
					nil,
				)
			}
		}

		return e.Next()
	})
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devices/... -v`
Expected: PASS

- [ ] **Step 5: Wire into main.go**

Add `devices.BindStateFieldGuard(app)` near the top-level hook registrations in `main.go` (module-scope, runs once, unlike the per-boot `OnBootstrap` block from Tasks 8–10). Add `"github.com/the-vas/device-lending/internal/devices"` to imports.

- [ ] **Step 6: Run full build and test suite, then commit**

```bash
go build ./... && go test ./...
git add device-lending/internal/devices device-lending/main.go
git commit -m "Guard device state-machine fields against direct API edits"
```

---

## Task 12: Mail notifier

Implements the 7 notification triggers from the design doc's section 4. Uses `html/template` (not `fmt.Sprintf`) to escape user-controlled data (device names, requester names) embedded in email HTML.

**Files:**
- Create: `device-lending/internal/mail/notify.go`
- Test: `device-lending/internal/mail/notify_test.go`

**Interfaces:**
- Produces:
  ```go
  package mail

  type Notifier struct{ /* unexported */ }

  func New(app core.App, baseURL string) *Notifier

  func (n *Notifier) NewRequest(owner, device, requester *core.Record) error
  func (n *Notifier) RequestRejected(requester, device *core.Record) error
  func (n *Notifier) HandoverAccepted(requester, device *core.Record, lendEnd string) error
  func (n *Notifier) DeviceUnavailable(requester, device *core.Record, lendEnd string, otherPendingCount int) error
  func (n *Notifier) DeviceAvailableAgain(requester, device *core.Record) error
  func (n *Notifier) RequestWithdrawn(owner, device, requester *core.Record) error
  func (n *Notifier) DeviceRemoved(requester, device *core.Record) error
  ```
  Consumed by the service-layer tasks (17–22).

- [ ] **Step 1: Write the failing tests**

```go
// internal/mail/notify_test.go
package mail_test

import (
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func newTestUser(t *testing.T, app core.App, email, name string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(users)
	r.SetEmail(email)
	r.Set("name", name)
	r.SetRandomPassword()
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func newTestDevice(t *testing.T, app core.App, owner *core.Record, name string) *core.Record {
	t.Helper()
	categories, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatal(err)
	}
	category := core.NewRecord(categories)
	category.Set("name", "Power tools")
	if err := app.Save(category); err != nil {
		t.Fatal(err)
	}

	devicesCol, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	d := core.NewRecord(devicesCol)
	d.Set("name", name)
	d.Set("category", category.Id)
	d.Set("owner", owner.Id)
	d.Set("status", "available")
	if err := app.Save(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func TestNotifier_AllTriggers(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUser(t, app, "owner@example.com", "Alice")
	requester := newTestUser(t, app, "requester@example.com", "Bob <script>")
	device := newTestDevice(t, app, owner, "Cordless Drill")

	n := mail.New(app, "https://lending.example.com")

	cases := []struct {
		name        string
		call        func() error
		wantTo      string
		wantSubject string
		wantBody    []string
		notWantBody []string
	}{
		{"NewRequest", func() error { return n.NewRequest(owner, device, requester) }, owner.Email(), "Cordless Drill", []string{"Bob &lt;script&gt;"}, []string{"<script>"}},
		{"RequestRejected", func() error { return n.RequestRejected(requester, device) }, requester.Email(), "Cordless Drill", []string{"declined"}, nil},
		{"HandoverAccepted", func() error { return n.HandoverAccepted(requester, device, "2026-08-10") }, requester.Email(), "Cordless Drill", []string{"2026-08-10"}, nil},
		{"DeviceUnavailable", func() error { return n.DeviceUnavailable(requester, device, "2026-08-10", 2) }, requester.Email(), "Cordless Drill", []string{"2 other"}, nil},
		{"DeviceAvailableAgain", func() error { return n.DeviceAvailableAgain(requester) }, requester.Email(), "Cordless Drill", []string{"available again"}, nil},
	}

	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			app.TestMailer.Reset()

			if err := c.call(); err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if app.TestMailer.TotalSend() != 1 {
				t.Fatalf("expected 1 email sent, got %d", app.TestMailer.TotalSend())
			}
			msg := app.TestMailer.LastMessage()
			if len(msg.To) != 1 || msg.To[0].Address != c.wantTo {
				t.Errorf("expected recipient %s, got %v", c.wantTo, msg.To)
			}
			if !strings.Contains(msg.Subject, c.wantSubject) {
				t.Errorf("expected subject to contain %q, got %q", c.wantSubject, msg.Subject)
			}
			for _, want := range c.wantBody {
				if !strings.Contains(msg.HTML, want) {
					t.Errorf("expected body to contain %q, got %q", want, msg.HTML)
				}
			}
			for _, notWant := range c.notWantBody {
				if strings.Contains(msg.HTML, notWant) {
					t.Errorf("expected body NOT to contain unescaped %q, got %q", notWant, msg.HTML)
				}
			}
		})
	}
}

func TestNotifier_RequestWithdrawnAndDeviceRemoved(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUser(t, app, "owner@example.com", "Alice")
	requester := newTestUser(t, app, "requester@example.com", "Bob")
	device := newTestDevice(t, app, owner, "Cordless Drill")

	n := mail.New(app, "https://lending.example.com")

	if err := n.RequestWithdrawn(owner, device, requester); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != owner.Email() {
		t.Errorf("expected RequestWithdrawn to notify the owner")
	}

	app.TestMailer.Reset()

	if err := n.DeviceRemoved(requester, device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != requester.Email() {
		t.Errorf("expected DeviceRemoved to notify the requester")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/mail/... -v`
Expected: FAIL with "undefined: mail.New"

- [ ] **Step 3: Implement notify.go**

```go
// internal/mail/notify.go
package mail

import (
	"bytes"
	"fmt"
	"html/template"
	"net/mail"

	"github.com/pocketbase/pocketbase/core"
	pbmailer "github.com/pocketbase/pocketbase/tools/mailer"
)

type Notifier struct {
	app     core.App
	baseURL string
}

func New(app core.App, baseURL string) *Notifier {
	return &Notifier{app: app, baseURL: baseURL}
}

var bodyTmpl = template.Must(template.New("email").Parse(
	`<p>{{.Intro}}</p>{{if .Detail}}<p>{{.Detail}}</p>{{end}}<p><a href="{{.Link}}">View on the portal</a></p>`,
))

type bodyData struct {
	Intro  string
	Detail string
	Link   string
}

func (n *Notifier) render(intro, detail, link string) (string, error) {
	var buf bytes.Buffer
	if err := bodyTmpl.Execute(&buf, bodyData{Intro: intro, Detail: detail, Link: link}); err != nil {
		return "", err
	}
	return buf.String(), nil
}

func (n *Notifier) send(to *core.Record, subject, intro, detail, link string) error {
	html, err := n.render(intro, detail, link)
	if err != nil {
		return err
	}

	settings := n.app.Settings()
	message := &pbmailer.Message{
		From:    mail.Address{Address: settings.Meta.SenderAddress, Name: settings.Meta.SenderName},
		To:      []mail.Address{{Address: to.Email(), Name: to.GetString("name")}},
		Subject: subject,
		HTML:    html,
	}
	return n.app.NewMailClient().Send(message)
}

func deviceLink(baseURL string, device *core.Record) string {
	return fmt.Sprintf("%s/devices/%s", baseURL, device.Id)
}

func (n *Notifier) NewRequest(owner, device, requester *core.Record) error {
	intro := fmt.Sprintf("%s requested to borrow %q.", requester.GetString("name"), device.GetString("name"))
	return n.send(owner, "New borrow request for "+device.GetString("name"), intro, "", deviceLink(n.baseURL, device))
}

func (n *Notifier) RequestRejected(requester, device *core.Record) error {
	intro := fmt.Sprintf("Your request to borrow %q was declined.", device.GetString("name"))
	return n.send(requester, "Request declined: "+device.GetString("name"), intro, "", deviceLink(n.baseURL, device))
}

func (n *Notifier) HandoverAccepted(requester, device *core.Record, lendEnd string) error {
	intro := fmt.Sprintf("%q is ready for you to pick up.", device.GetString("name"))
	detail := ""
	if lendEnd != "" {
		detail = "Expected return date: " + lendEnd
	}
	return n.send(requester, "You're getting "+device.GetString("name"), intro, detail, deviceLink(n.baseURL, device))
}

func (n *Notifier) DeviceUnavailable(requester, device *core.Record, lendEnd string, otherPendingCount int) error {
	intro := fmt.Sprintf("%q was just lent to someone else and is no longer available for now.", device.GetString("name"))
	detail := fmt.Sprintf("%d other request(s) besides yours are still waiting for it.", otherPendingCount)
	if lendEnd != "" {
		detail = "Expected back: " + lendEnd + ". " + detail
	}
	return n.send(requester, device.GetString("name")+" is now unavailable", intro, detail, deviceLink(n.baseURL, device))
}

func (n *Notifier) DeviceAvailableAgain(requester, device *core.Record) error {
	intro := fmt.Sprintf("%q was returned and is available again.", device.GetString("name"))
	return n.send(requester, device.GetString("name")+" is available again", intro, "", deviceLink(n.baseURL, device))
}

func (n *Notifier) RequestWithdrawn(owner, device, requester *core.Record) error {
	intro := fmt.Sprintf("%s withdrew their request to borrow %q.", requester.GetString("name"), device.GetString("name"))
	return n.send(owner, "Request withdrawn: "+device.GetString("name"), intro, "", deviceLink(n.baseURL, device))
}

func (n *Notifier) DeviceRemoved(requester, device *core.Record) error {
	intro := fmt.Sprintf("%q was removed by its owner and is no longer available.", device.GetString("name"))
	return n.send(requester, device.GetString("name")+" was removed", intro, "", n.baseURL)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/mail/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/mail
git commit -m "Add email notifier for lending workflow events"
```

---

## Task 13: Signed cookie helper

A small HMAC-signing helper shared by the PKCE cookie (Task 14) and the session cookie (Tasks 15–16), so a client can't forge either.

**Files:**
- Create: `device-lending/internal/webauth/session.go`
- Test: `device-lending/internal/webauth/session_test.go`

**Interfaces:**
- Produces:
  ```go
  package webauth

  type Signer struct{ /* unexported */ }

  func NewSigner(secret string) Signer

  // Sign encodes and HMAC-signs value into an opaque cookie-safe string.
  func (s Signer) Sign(value string) string

  // Verify decodes a value produced by Sign and checks its signature,
  // returning an error if the value was tampered with or malformed.
  func (s Signer) Verify(signed string) (string, error)
  ```

- [ ] **Step 1: Write the failing tests**

```go
// internal/webauth/session_test.go
package webauth_test

import (
	"testing"

	"github.com/the-vas/device-lending/internal/webauth"
)

func TestSigner_RoundTrip(t *testing.T) {
	s := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	signed := s.Sign("hello world")
	got, err := s.Verify(signed)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got != "hello world" {
		t.Errorf("expected 'hello world', got %q", got)
	}
}

func TestSigner_RejectsTamperedValue(t *testing.T) {
	s := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	signed := s.Sign("hello world")
	tampered := signed[:len(signed)-1] + "x"

	if _, err := s.Verify(tampered); err == nil {
		t.Fatal("expected error for tampered signature")
	}
}

func TestSigner_RejectsWrongSecret(t *testing.T) {
	s1 := webauth.NewSigner("secret-one-at-least-32-bytes-long")
	s2 := webauth.NewSigner("secret-two-at-least-32-bytes-long")

	signed := s1.Sign("hello world")
	if _, err := s2.Verify(signed); err == nil {
		t.Fatal("expected error when verifying with a different secret")
	}
}

func TestSigner_RejectsMalformedValue(t *testing.T) {
	s := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	if _, err := s.Verify("not-a-signed-value"); err == nil {
		t.Fatal("expected error for a malformed signed value")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/webauth/... -v`
Expected: FAIL with "undefined: webauth.NewSigner"

- [ ] **Step 3: Implement session.go**

```go
// internal/webauth/session.go
package webauth

import (
	"crypto/hmac"
	"crypto/sha256"
	"encoding/base64"
	"errors"
	"strings"
)

type Signer struct {
	secret []byte
}

func NewSigner(secret string) Signer {
	return Signer{secret: []byte(secret)}
}

func (s Signer) Sign(value string) string {
	mac := hmac.New(sha256.New, s.secret)
	mac.Write([]byte(value))
	sig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))
	return base64.RawURLEncoding.EncodeToString([]byte(value)) + "." + sig
}

func (s Signer) Verify(signed string) (string, error) {
	parts := strings.SplitN(signed, ".", 2)
	if len(parts) != 2 {
		return "", errors.New("malformed signed value")
	}

	valueBytes, err := base64.RawURLEncoding.DecodeString(parts[0])
	if err != nil {
		return "", errors.New("malformed signed value")
	}

	mac := hmac.New(sha256.New, s.secret)
	mac.Write(valueBytes)
	expectedSig := base64.RawURLEncoding.EncodeToString(mac.Sum(nil))

	if !hmac.Equal([]byte(expectedSig), []byte(parts[1])) {
		return "", errors.New("invalid signature")
	}

	return string(valueBytes), nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/webauth/... -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/webauth
git commit -m "Add signed cookie helper for OIDC PKCE and session state"
```

---

## Task 14: OIDC login route (`/oidc/login`)

Server-side PKCE flow, no JS SDK: builds the provider's consent URL directly using the `oidc` provider config saved on the `users` collection (Task 8), stores `state`+`code_verifier` in a signed, short-lived, path-scoped cookie, and redirects the browser.

**Files:**
- Create: `device-lending/internal/webauth/login.go`
- Test: `device-lending/internal/webauth/login_test.go`

**Interfaces:**
- Consumes: `Signer` (Task 13), the `oidc` provider config on the `users` collection (Task 8).
- Produces: `const PKCECookieName = "oidc_pkce"` and `func LoginHandler(baseURL string, signer Signer) func(e *core.RequestEvent) error`, registered at `GET /oidc/login` in Task 23's route wiring.

- [ ] **Step 1: Write the failing test**

```go
// internal/webauth/login_test.go
package webauth_test

import (
	"context"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/authsetup"
	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func fakeDiscovery(ctx context.Context, issuer string) (oidcdiscovery.Document, error) {
	return oidcdiscovery.Document{
		AuthorizationEndpoint: "https://auth.example.com/authorize",
		TokenEndpoint:         "https://auth.example.com/token",
		UserinfoEndpoint:      "https://auth.example.com/userinfo",
	}, nil
}

func TestLoginHandler(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	cfg := config.Config{OIDCIssuer: "https://auth.example.com", OIDCClientID: "abc", OIDCClientSecret: "secret"}
	if err := authsetup.ConfigureOAuth2(app, cfg, fakeDiscovery); err != nil {
		t.Fatal(err)
	}

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.GET("/oidc/login", webauth.LoginHandler("https://lending.example.com", signer))
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/oidc/login", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	loc := rec.Header().Get("Location")
	if !strings.HasPrefix(loc, "https://auth.example.com/authorize") {
		t.Errorf("unexpected redirect location: %s", loc)
	}
	if !strings.Contains(loc, "code_challenge=") {
		t.Errorf("expected code_challenge param, got %s", loc)
	}

	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == webauth.PKCECookieName {
			found = true
			if c.Value == "" {
				t.Error("expected non-empty pkce cookie value")
			}
		}
	}
	if !found {
		t.Error("expected pkce cookie to be set")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/webauth/... -run TestLoginHandler -v`
Expected: FAIL with "undefined: webauth.LoginHandler"

- [ ] **Step 3: Implement login.go**

```go
// internal/webauth/login.go
package webauth

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/security"
	"golang.org/x/oauth2"
)

const PKCECookieName = "oidc_pkce"

func LoginHandler(baseURL string, signer Signer) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		users, err := e.App.FindCollectionByNameOrId("users")
		if err != nil {
			return err
		}

		providerConfig, ok := users.OAuth2.GetProviderConfig("oidc")
		if !ok {
			return e.InternalServerError("OIDC provider is not configured", nil)
		}

		provider, err := providerConfig.InitProvider()
		if err != nil {
			return e.InternalServerError("failed to initialize OIDC provider", err)
		}

		state := security.RandomString(30)
		codeVerifier := security.RandomString(43)
		codeChallenge := security.S256Challenge(codeVerifier)

		provider.SetRedirectURL(baseURL + "/oidc/callback")

		authURL := provider.BuildAuthURL(
			state,
			oauth2.SetAuthURLParam("code_challenge", codeChallenge),
			oauth2.SetAuthURLParam("code_challenge_method", "S256"),
		)

		e.SetCookie(&http.Cookie{
			Name:     PKCECookieName,
			Value:    signer.Sign(state + "|" + codeVerifier),
			Path:     "/oidc/callback",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   600,
		})

		return e.Redirect(http.StatusFound, authURL)
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/webauth/... -run TestLoginHandler -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/webauth/login.go device-lending/internal/webauth/login_test.go
git commit -m "Add server-side OIDC login redirect route"
```

---

## Task 15: OIDC callback route (`/oidc/callback`)

Exchanges the authorization code for a PocketBase auth token by dispatching an in-process request through PocketBase's own `/api/collections/users/auth-with-oauth2` endpoint — reusing its official record find-or-create/verification logic instead of reimplementing it — then stores the token in an HttpOnly session cookie. The exchange mechanism is injected as an `Exchanger` function so the route handler is unit-testable without a real OIDC provider.

**Files:**
- Create: `device-lending/internal/webauth/callback.go`
- Test: `device-lending/internal/webauth/callback_test.go`

**Interfaces:**
- Consumes: `Signer` (Task 13).
- Produces:
  ```go
  const SessionCookieName = "pb_session"

  type ExchangeResult struct {
      Token  string
      Record map[string]any
  }

  // Exchanger exchanges an authorization code for a token + user record.
  type Exchanger func(app core.App, code, codeVerifier, redirectURL string) (ExchangeResult, error)

  // RouterExchanger is the production Exchanger: it dispatches the exchange
  // request in-process through apis.NewRouter(app).BuildMux(), no real network call.
  func RouterExchanger(app core.App, code, codeVerifier, redirectURL string) (ExchangeResult, error)

  func CallbackHandler(baseURL string, signer Signer, exchange Exchanger) func(e *core.RequestEvent) error
  ```
  `SessionCookieName` and `Signer` are consumed by Task 16's `LoadSession` middleware.

- [ ] **Step 1: Write the failing tests**

```go
// internal/webauth/callback_test.go
package webauth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func registerCallbackRoute(t *testing.T, app *tests.TestApp, signer webauth.Signer, exchange webauth.Exchanger) http.Handler {
	t.Helper()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.GET("/oidc/callback", webauth.CallbackHandler("https://lending.example.com", signer, exchange))
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

func TestCallbackHandler_Success(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	fakeExchange := func(app core.App, code, codeVerifier, redirectURL string) (webauth.ExchangeResult, error) {
		if code != "auth-code-123" {
			t.Errorf("unexpected code: %s", code)
		}
		if codeVerifier != "verifier-abc" {
			t.Errorf("unexpected codeVerifier: %s", codeVerifier)
		}
		return webauth.ExchangeResult{Token: "fake-auth-token", Record: map[string]any{"id": "u1"}}, nil
	}

	mux := registerCallbackRoute(t, app, signer, fakeExchange)

	req := httptest.NewRequest(http.MethodGet, "/oidc/callback?code=auth-code-123&state=state-xyz", nil)
	req.AddCookie(&http.Cookie{
		Name:  webauth.PKCECookieName,
		Value: signer.Sign("state-xyz|verifier-abc"),
	})
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
		}
	}
	if !found {
		t.Error("expected session cookie to be set")
	}
}

func TestCallbackHandler_StateMismatch(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	neverCalled := func(app core.App, code, codeVerifier, redirectURL string) (webauth.ExchangeResult, error) {
		t.Fatal("exchange should not be called on state mismatch")
		return webauth.ExchangeResult{}, nil
	}

	mux := registerCallbackRoute(t, app, signer, neverCalled)

	req := httptest.NewRequest(http.MethodGet, "/oidc/callback?code=auth-code-123&state=wrong-state", nil)
	req.AddCookie(&http.Cookie{
		Name:  webauth.PKCECookieName,
		Value: signer.Sign("state-xyz|verifier-abc"),
	})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}

func TestCallbackHandler_MissingCookie(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")
	neverCalled := func(app core.App, code, codeVerifier, redirectURL string) (webauth.ExchangeResult, error) {
		t.Fatal("exchange should not be called without a pkce cookie")
		return webauth.ExchangeResult{}, nil
	}

	mux := registerCallbackRoute(t, app, signer, neverCalled)

	req := httptest.NewRequest(http.MethodGet, "/oidc/callback?code=auth-code-123&state=state-xyz", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d", rec.Code)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/webauth/... -run TestCallbackHandler -v`
Expected: FAIL with "undefined: webauth.CallbackHandler"

- [ ] **Step 3: Implement callback.go**

```go
// internal/webauth/callback.go
package webauth

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
)

const SessionCookieName = "pb_session"

type ExchangeResult struct {
	Token  string
	Record map[string]any
}

type Exchanger func(app core.App, code, codeVerifier, redirectURL string) (ExchangeResult, error)

// RouterExchanger dispatches the auth-with-oauth2 exchange in-process
// through PocketBase's own router, reusing its official record
// find-or-create and verification logic without a real network call.
func RouterExchanger(app core.App, code, codeVerifier, redirectURL string) (ExchangeResult, error) {
	router, err := apis.NewRouter(app)
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("building router: %w", err)
	}

	mux, err := router.BuildMux()
	if err != nil {
		return ExchangeResult{}, fmt.Errorf("building mux: %w", err)
	}

	body, err := json.Marshal(map[string]string{
		"provider":     "oidc",
		"code":         code,
		"codeVerifier": codeVerifier,
		"redirectURL":  redirectURL,
	})
	if err != nil {
		return ExchangeResult{}, err
	}

	req := httptest.NewRequest(http.MethodPost, "/api/collections/users/auth-with-oauth2", bytes.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		return ExchangeResult{}, fmt.Errorf("oauth2 exchange failed with status %d: %s", rec.Code, rec.Body.String())
	}

	var parsed struct {
		Token  string         `json:"token"`
		Record map[string]any `json:"record"`
	}
	if err := json.Unmarshal(rec.Body.Bytes(), &parsed); err != nil {
		return ExchangeResult{}, fmt.Errorf("parsing oauth2 exchange response: %w", err)
	}

	return ExchangeResult{Token: parsed.Token, Record: parsed.Record}, nil
}

func CallbackHandler(baseURL string, signer Signer, exchange Exchanger) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		code := e.Request.URL.Query().Get("code")
		state := e.Request.URL.Query().Get("state")
		if code == "" || state == "" {
			return e.BadRequestError("missing code or state", nil)
		}

		cookie, err := e.Request.Cookie(PKCECookieName)
		if err != nil {
			return e.BadRequestError("missing pkce cookie; please retry login", nil)
		}

		raw, err := signer.Verify(cookie.Value)
		if err != nil {
			return e.BadRequestError("invalid pkce cookie", nil)
		}

		parts := strings.SplitN(raw, "|", 2)
		if len(parts) != 2 || parts[0] != state {
			return e.BadRequestError("state mismatch", nil)
		}
		codeVerifier := parts[1]

		result, err := exchange(e.App, code, codeVerifier, baseURL+"/oidc/callback")
		if err != nil {
			return e.InternalServerError("oidc token exchange failed", err)
		}

		e.SetCookie(&http.Cookie{
			Name:     SessionCookieName,
			Value:    signer.Sign(result.Token),
			Path:     "/",
			HttpOnly: true,
			Secure:   true,
			SameSite: http.SameSiteLaxMode,
			MaxAge:   60 * 60 * 24 * 5,
		})

		e.SetCookie(&http.Cookie{
			Name:   PKCECookieName,
			Value:  "",
			Path:   "/oidc/callback",
			MaxAge: -1,
		})

		return e.Redirect(http.StatusFound, "/")
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/webauth/... -run TestCallbackHandler -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/webauth/callback.go device-lending/internal/webauth/callback_test.go
git commit -m "Add OIDC callback route exchanging code for a session cookie"
```

---

## Task 16: Web session middleware + logout route

**Files:**
- Create: `device-lending/internal/webauth/middleware.go`
- Test: `device-lending/internal/webauth/middleware_test.go`

**Interfaces:**
- Consumes: `SessionCookieName`, `Signer` (Task 15/13).
- Produces:
  ```go
  // LoadSession reads the session cookie (if present and valid) and sets
  // e.Auth to the corresponding user record, so downstream handlers and
  // templates can tell who's logged in. Never blocks the request.
  func LoadSession(signer Signer) func(e *core.RequestEvent) error

  // RequireWebAuth redirects to loginPath if e.Auth is nil.
  func RequireWebAuth(loginPath string) func(e *core.RequestEvent) error

  func LogoutHandler() func(e *core.RequestEvent) error
  ```
  `LoadSession` is bound as global router middleware in Task 23; `RequireWebAuth` is bound per-route-group on write-side pages in Tasks 25–29.

- [ ] **Step 1: Write the failing tests**

```go
// internal/webauth/middleware_test.go
package webauth_test

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func newTestUserForSession(t *testing.T, app core.App) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(users)
	r.SetEmail("session@example.com")
	r.SetRandomPassword()
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestLoadSession_ValidCookie(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	user := newTestUserForSession(t, app)
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}
	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.BindFunc(webauth.LoadSession(signer))
		e.Router.GET("/whoami", func(re *core.RequestEvent) error {
			if re.Auth == nil {
				return re.String(http.StatusUnauthorized, "anonymous")
			}
			return re.String(http.StatusOK, re.Auth.Id)
		})
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	req.AddCookie(&http.Cookie{Name: webauth.SessionCookieName, Value: signer.Sign(token)})
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if rec.Body.String() != user.Id {
		t.Errorf("expected body %q, got %q", user.Id, rec.Body.String())
	}
}

func TestLoadSession_NoCookie(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	signer := webauth.NewSigner("test-secret-at-least-32-bytes-long")

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.BindFunc(webauth.LoadSession(signer))
		e.Router.GET("/whoami", func(re *core.RequestEvent) error {
			if re.Auth == nil {
				return re.String(http.StatusUnauthorized, "anonymous")
			}
			return re.String(http.StatusOK, re.Auth.Id)
		})
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodGet, "/whoami", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusUnauthorized {
		t.Fatalf("expected 401, got %d", rec.Code)
	}
}

func TestLogoutHandler(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		e.Router.POST("/logout", webauth.LogoutHandler())
		return e.Next()
	})
	if err != nil {
		t.Fatal(err)
	}
	mux, err := router.BuildMux()
	if err != nil {
		t.Fatal(err)
	}

	req := httptest.NewRequest(http.MethodPost, "/logout", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d", rec.Code)
	}
	found := false
	for _, c := range rec.Result().Cookies() {
		if c.Name == webauth.SessionCookieName && c.MaxAge < 0 {
			found = true
		}
	}
	if !found {
		t.Error("expected logout to clear the session cookie")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/webauth/... -run "TestLoadSession|TestLogoutHandler" -v`
Expected: FAIL with "undefined: webauth.LoadSession"

- [ ] **Step 3: Implement middleware.go**

```go
// internal/webauth/middleware.go
package webauth

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func LoadSession(signer Signer) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		cookie, err := e.Request.Cookie(SessionCookieName)
		if err != nil {
			return e.Next()
		}

		token, err := signer.Verify(cookie.Value)
		if err != nil {
			return e.Next()
		}

		record, err := e.App.FindAuthRecordByToken(token, core.TokenTypeAuth)
		if err != nil {
			return e.Next()
		}

		e.Auth = record
		return e.Next()
	}
}

func RequireWebAuth(loginPath string) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, loginPath)
		}
		return e.Next()
	}
}

func LogoutHandler() func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		e.SetCookie(&http.Cookie{
			Name:   SessionCookieName,
			Value:  "",
			Path:   "/",
			MaxAge: -1,
		})
		return e.Redirect(http.StatusFound, "/")
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/webauth/... -v`
Expected: PASS (all of Tasks 13–16's tests)

- [ ] **Step 5: Wire LoadSession as global middleware in main.go**

In `main.go`'s `OnServe` block (from Task 1), bind it before registering page routes:

```go
	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		signer := webauth.NewSigner(cfg.SessionSecret)

		se.Router.BindFunc(webauth.LoadSession(signer))
		se.Router.GET("/oidc/login", webauth.LoginHandler(cfg.BaseURL, signer))
		se.Router.GET("/oidc/callback", webauth.CallbackHandler(cfg.BaseURL, signer, webauth.RouterExchanger))
		se.Router.POST("/logout", webauth.LogoutHandler())

		se.Router.GET("/healthz", func(e *core.RequestEvent) error {
			return e.String(200, "ok")
		})

		return se.Next()
	})
```

This replaces the smaller `OnServe` block from Task 1 (which only had `/healthz`); remove the old duplicate block. Add `"github.com/the-vas/device-lending/internal/webauth"` to imports.

- [ ] **Step 6: Run full build and test suite**

Run: `go build ./... && go test ./...`
Expected: PASS

- [ ] **Step 7: Manual smoke test**

Run: `go run . serve --http=127.0.0.1:8091 &`, then `curl -sI http://127.0.0.1:8091/oidc/login` and confirm a `302` with a `Location` header pointing at the configured OIDC issuer's authorize endpoint (requires `OIDC_ISSUER`/`OIDC_CLIENT_ID`/`OIDC_CLIENT_SECRET`/`BASE_URL`/`SESSION_SECRET` env vars set to real or placeholder values first). Stop the background process afterward.

- [ ] **Step 8: Commit**

```bash
git add device-lending/internal/webauth device-lending/main.go
git commit -m "Add session middleware, logout route, and wire OIDC routes into main"
```

---

## Task 17: Service — create a lending request

Establishes the shared test fixture helpers (`newUser`, `newDevice`, `newRequest`) in `internal/devices` package `devices_test`, reused unmodified by Tasks 18–22.

**Files:**
- Create: `device-lending/internal/devices/testfixtures_test.go`
- Create: `device-lending/internal/devices/create_request.go`
- Test: `device-lending/internal/devices/create_request_test.go`

**Interfaces:**
- Consumes: `mail.Notifier` (Task 12).
- Produces:
  ```go
  package devices

  // pendingRequests returns all "pending" lending_requests for a device, newest first.
  func pendingRequests(app core.App, deviceID string) ([]*core.Record, error)

  // CreateRequest creates a pending lending_requests record, transitions the device
  // from "available" to "requested" if needed, and emails the owner.
  func CreateRequest(app core.App, notifier *mail.Notifier, device, requester *core.Record, requestedStart, requestedEnd types.DateTime, message string) (*core.Record, error)
  ```
  `pendingRequests` is reused by Tasks 18–22.

- [ ] **Step 1: Write the shared test fixtures**

```go
// internal/devices/testfixtures_test.go
package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/core"
)

func newUser(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(users)
	r.SetEmail(email)
	r.Set("name", email)
	r.SetRandomPassword()
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func newDevice(t *testing.T, app core.App, owner *core.Record, name, status string) *core.Record {
	t.Helper()

	categories, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatal(err)
	}
	category := core.NewRecord(categories)
	category.Set("name", "Category for "+name)
	if err := app.Save(category); err != nil {
		t.Fatal(err)
	}

	devicesCol, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	d := core.NewRecord(devicesCol)
	d.Set("name", name)
	d.Set("category", category.Id)
	d.Set("owner", owner.Id)
	d.Set("status", status)
	if err := app.Save(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func newRequest(t *testing.T, app core.App, device, requester *core.Record, status string) *core.Record {
	t.Helper()

	col, err := app.FindCollectionByNameOrId("lending_requests")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("device", device.Id)
	r.Set("requester", requester.Id)
	r.Set("status", status)
	r.Set("requested_start", "2026-08-01 00:00:00.000Z")
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}
```

- [ ] **Step 2: Write the failing tests**

```go
// internal/devices/create_request_test.go
package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestCreateRequest_AvailableDeviceBecomesRequested(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	requester := newUser(t, app, "requester@example.com")
	device := newDevice(t, app, owner, "Drill", "available")
	notifier := mail.New(app, "https://lending.example.com")

	start := types.NowDateTime()
	req, err := devices.CreateRequest(app, notifier, device, requester, start, types.DateTime{}, "please and thank you")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if req.GetString("status") != "pending" {
		t.Errorf("expected pending status, got %q", req.GetString("status"))
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "requested" {
		t.Errorf("expected device status 'requested', got %q", updatedDevice.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to owner, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != owner.Email() {
		t.Errorf("expected email to owner")
	}
}

func TestCreateRequest_AlreadyRequestedDeviceStaysRequested(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	firstRequester := newUser(t, app, "first@example.com")
	secondRequester := newUser(t, app, "second@example.com")
	device := newDevice(t, app, owner, "Saw", "requested")
	newRequest(t, app, device, firstRequester, "pending")

	notifier := mail.New(app, "https://lending.example.com")
	app.TestMailer.Reset()

	_, err = devices.CreateRequest(app, notifier, device, secondRequester, types.NowDateTime(), types.DateTime{}, "")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "requested" {
		t.Errorf("expected device status to remain 'requested', got %q", updatedDevice.GetString("status"))
	}
	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to owner, got %d", app.TestMailer.TotalSend())
	}
}
```

- [ ] **Step 3: Run tests to verify they fail**

Run: `go test ./internal/devices/... -run TestCreateRequest -v`
Expected: FAIL with "undefined: devices.CreateRequest"

- [ ] **Step 4: Implement create_request.go**

```go
// internal/devices/create_request.go
package devices

import (
	"fmt"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/mail"
)

func pendingRequests(app core.App, deviceID string) ([]*core.Record, error) {
	return app.FindRecordsByFilter(
		"lending_requests",
		"device = {:device} && status = 'pending'",
		"-created",
		0, 0,
		dbx.Params{"device": deviceID},
	)
}

func CreateRequest(
	app core.App,
	notifier *mail.Notifier,
	device, requester *core.Record,
	requestedStart, requestedEnd types.DateTime,
	message string,
) (*core.Record, error) {
	col, err := app.FindCollectionByNameOrId("lending_requests")
	if err != nil {
		return nil, err
	}

	request := core.NewRecord(col)
	request.Set("device", device.Id)
	request.Set("requester", requester.Id)
	request.Set("status", "pending")
	request.Set("requested_start", requestedStart)
	if !requestedEnd.IsZero() {
		request.Set("requested_end", requestedEnd)
	}
	request.Set("message", message)

	if err := app.Save(request); err != nil {
		return nil, fmt.Errorf("saving request: %w", err)
	}

	if device.GetString("status") == "available" {
		device.Set("status", "requested")
		if err := app.Save(device); err != nil {
			return nil, fmt.Errorf("updating device status: %w", err)
		}
	}

	owner, err := app.FindRecordById("users", device.GetString("owner"))
	if err != nil {
		return nil, fmt.Errorf("loading device owner: %w", err)
	}

	if err := notifier.NewRequest(owner, device, requester); err != nil {
		return nil, fmt.Errorf("sending notification: %w", err)
	}

	return request, nil
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/devices/... -run TestCreateRequest -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add device-lending/internal/devices/testfixtures_test.go device-lending/internal/devices/create_request.go device-lending/internal/devices/create_request_test.go
git commit -m "Add CreateRequest service function"
```

---

## Task 18: Service — handover

**Files:**
- Create: `device-lending/internal/devices/handover.go`
- Test: `device-lending/internal/devices/handover_test.go`

**Interfaces:**
- Consumes: `pendingRequests` (Task 17), `mail.Notifier` (Task 12).
- Produces: `func Handover(app core.App, notifier *mail.Notifier, device, chosenRequest *core.Record, lendEnd types.DateTime) error`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/devices/handover_test.go
package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestHandover_AcceptsChosenAndNotifiesOthers(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	chosen := newUser(t, app, "chosen@example.com")
	other := newUser(t, app, "other@example.com")
	device := newDevice(t, app, owner, "Drill", "requested")
	chosenReq := newRequest(t, app, device, chosen, "pending")
	otherReq := newRequest(t, app, device, other, "pending")

	notifier := mail.New(app, "https://lending.example.com")
	app.TestMailer.Reset()

	lendEnd, err := types.ParseDateTime("2026-08-10 00:00:00.000Z")
	if err != nil {
		t.Fatal(err)
	}

	if err := devices.Handover(app, notifier, device, chosenReq, lendEnd); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "lent" {
		t.Errorf("expected device status 'lent', got %q", updatedDevice.GetString("status"))
	}
	if updatedDevice.GetString("current_borrower") != chosen.Id {
		t.Errorf("expected current_borrower %q, got %q", chosen.Id, updatedDevice.GetString("current_borrower"))
	}

	updatedChosenReq, err := app.FindRecordById("lending_requests", chosenReq.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedChosenReq.GetString("status") != "accepted" {
		t.Errorf("expected chosen request status 'accepted', got %q", updatedChosenReq.GetString("status"))
	}

	updatedOtherReq, err := app.FindRecordById("lending_requests", otherReq.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedOtherReq.GetString("status") != "pending" {
		t.Errorf("expected other request to remain 'pending', got %q", updatedOtherReq.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 2 {
		t.Fatalf("expected 2 emails (chosen + other), got %d", app.TestMailer.TotalSend())
	}

	recipients := map[string]bool{}
	for _, m := range app.TestMailer.Messages() {
		recipients[m.To[0].Address] = true
	}
	if !recipients[chosen.Email()] || !recipients[other.Email()] {
		t.Errorf("expected emails to both chosen and other requester, got %v", recipients)
	}
}

func TestHandover_RejectsRequestFromOtherDevice(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	requester := newUser(t, app, "requester@example.com")
	deviceA := newDevice(t, app, owner, "Drill", "requested")
	deviceB := newDevice(t, app, owner, "Saw", "requested")
	reqForB := newRequest(t, app, deviceB, requester, "pending")

	notifier := mail.New(app, "https://lending.example.com")

	err = devices.Handover(app, notifier, deviceA, reqForB, types.DateTime{})
	if err == nil {
		t.Fatal("expected error when request belongs to a different device")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devices/... -run TestHandover -v`
Expected: FAIL with "undefined: devices.Handover"

- [ ] **Step 3: Implement handover.go**

```go
// internal/devices/handover.go
package devices

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/mail"
)

func Handover(app core.App, notifier *mail.Notifier, device, chosenRequest *core.Record, lendEnd types.DateTime) error {
	if chosenRequest.GetString("device") != device.Id {
		return fmt.Errorf("request %s does not belong to device %s", chosenRequest.Id, device.Id)
	}
	if chosenRequest.GetString("status") != "pending" {
		return fmt.Errorf("request %s is not pending", chosenRequest.Id)
	}

	others, err := pendingRequests(app, device.Id)
	if err != nil {
		return fmt.Errorf("loading pending requests: %w", err)
	}

	chosenRequest.Set("status", "accepted")
	chosenRequest.Set("decided_at", types.NowDateTime())
	if err := app.Save(chosenRequest); err != nil {
		return fmt.Errorf("accepting request: %w", err)
	}

	device.Set("status", "lent")
	device.Set("current_borrower", chosenRequest.GetString("requester"))
	device.Set("lend_start", types.NowDateTime())
	if !lendEnd.IsZero() {
		device.Set("lend_end", lendEnd)
	}
	if err := app.Save(device); err != nil {
		return fmt.Errorf("updating device: %w", err)
	}

	requester, err := app.FindRecordById("users", chosenRequest.GetString("requester"))
	if err != nil {
		return fmt.Errorf("loading requester: %w", err)
	}

	lendEndStr := ""
	if !lendEnd.IsZero() {
		lendEndStr = lendEnd.String()
	}

	if err := notifier.HandoverAccepted(requester, device, lendEndStr); err != nil {
		return fmt.Errorf("notifying requester: %w", err)
	}

	remainingOthers := 0
	for _, other := range others {
		if other.Id != chosenRequest.Id {
			remainingOthers++
		}
	}

	for _, other := range others {
		if other.Id == chosenRequest.Id {
			continue
		}
		otherRequester, err := app.FindRecordById("users", other.GetString("requester"))
		if err != nil {
			return fmt.Errorf("loading other requester: %w", err)
		}
		if err := notifier.DeviceUnavailable(otherRequester, device, lendEndStr, remainingOthers-1); err != nil {
			return fmt.Errorf("notifying other requester: %w", err)
		}
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devices/... -run TestHandover -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/devices/handover.go device-lending/internal/devices/handover_test.go
git commit -m "Add Handover service function"
```

---

## Task 19: Service — reject a pending request

**Files:**
- Create: `device-lending/internal/devices/reject.go`
- Test: `device-lending/internal/devices/reject_test.go`

**Interfaces:**
- Consumes: `pendingRequests` (Task 17), `mail.Notifier` (Task 12).
- Produces:
  ```go
  func Reject(app core.App, notifier *mail.Notifier, request *core.Record) error

  // revertDeviceIfNoPending sets device.status back to "available" if it is
  // currently "requested" and has no remaining pending requests. Reused by
  // Task 20's Withdraw.
  func revertDeviceIfNoPending(app core.App, device *core.Record) error
  ```

- [ ] **Step 1: Write the failing tests**

```go
// internal/devices/reject_test.go
package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestReject_LastPendingRevertsDeviceToAvailable(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	requester := newUser(t, app, "requester@example.com")
	device := newDevice(t, app, owner, "Drill", "requested")
	req := newRequest(t, app, device, requester, "pending")

	notifier := mail.New(app, "https://lending.example.com")
	app.TestMailer.Reset()

	if err := devices.Reject(app, notifier, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedReq, err := app.FindRecordById("lending_requests", req.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedReq.GetString("status") != "rejected" {
		t.Errorf("expected status 'rejected', got %q", updatedReq.GetString("status"))
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "available" {
		t.Errorf("expected device status 'available', got %q", updatedDevice.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to requester, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != requester.Email() {
		t.Error("expected email to requester")
	}
}

func TestReject_OtherPendingKeepsDeviceRequested(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	toReject := newUser(t, app, "reject-me@example.com")
	stillPending := newUser(t, app, "still-pending@example.com")
	device := newDevice(t, app, owner, "Drill", "requested")
	req := newRequest(t, app, device, toReject, "pending")
	newRequest(t, app, device, stillPending, "pending")

	notifier := mail.New(app, "https://lending.example.com")

	if err := devices.Reject(app, notifier, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "requested" {
		t.Errorf("expected device status to remain 'requested', got %q", updatedDevice.GetString("status"))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devices/... -run TestReject -v`
Expected: FAIL with "undefined: devices.Reject"

- [ ] **Step 3: Implement reject.go**

```go
// internal/devices/reject.go
package devices

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/mail"
)

func revertDeviceIfNoPending(app core.App, device *core.Record) error {
	if device.GetString("status") != "requested" {
		return nil
	}

	remaining, err := pendingRequests(app, device.Id)
	if err != nil {
		return fmt.Errorf("checking remaining pending requests: %w", err)
	}

	if len(remaining) == 0 {
		device.Set("status", "available")
		if err := app.Save(device); err != nil {
			return fmt.Errorf("reverting device status: %w", err)
		}
	}

	return nil
}

func Reject(app core.App, notifier *mail.Notifier, request *core.Record) error {
	if request.GetString("status") != "pending" {
		return fmt.Errorf("request %s is not pending", request.Id)
	}

	device, err := app.FindRecordById("devices", request.GetString("device"))
	if err != nil {
		return fmt.Errorf("loading device: %w", err)
	}

	request.Set("status", "rejected")
	request.Set("decided_at", types.NowDateTime())
	if err := app.Save(request); err != nil {
		return fmt.Errorf("rejecting request: %w", err)
	}

	if err := revertDeviceIfNoPending(app, device); err != nil {
		return err
	}

	requester, err := app.FindRecordById("users", request.GetString("requester"))
	if err != nil {
		return fmt.Errorf("loading requester: %w", err)
	}

	return notifier.RequestRejected(requester, device)
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devices/... -run TestReject -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/devices/reject.go device-lending/internal/devices/reject_test.go
git commit -m "Add Reject service function"
```

---

## Task 20: Service — withdraw a request

**Files:**
- Create: `device-lending/internal/devices/withdraw.go`
- Test: `device-lending/internal/devices/withdraw_test.go`

**Interfaces:**
- Consumes: `pendingRequests` (Task 17), `revertDeviceIfNoPending` (Task 19), `mail.Notifier` (Task 12).
- Produces: `func Withdraw(app core.App, notifier *mail.Notifier, request *core.Record) error`.

- [ ] **Step 1: Write the failing test**

```go
// internal/devices/withdraw_test.go
package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestWithdraw_NotifiesOwnerAndRevertsDevice(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	requester := newUser(t, app, "requester@example.com")
	device := newDevice(t, app, owner, "Drill", "requested")
	req := newRequest(t, app, device, requester, "pending")

	notifier := mail.New(app, "https://lending.example.com")
	app.TestMailer.Reset()

	if err := devices.Withdraw(app, notifier, req); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedReq, err := app.FindRecordById("lending_requests", req.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedReq.GetString("status") != "withdrawn" {
		t.Errorf("expected status 'withdrawn', got %q", updatedReq.GetString("status"))
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "available" {
		t.Errorf("expected device status 'available', got %q", updatedDevice.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to owner, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != owner.Email() {
		t.Error("expected email to owner")
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devices/... -run TestWithdraw -v`
Expected: FAIL with "undefined: devices.Withdraw"

- [ ] **Step 3: Implement withdraw.go**

```go
// internal/devices/withdraw.go
package devices

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/mail"
)

func Withdraw(app core.App, notifier *mail.Notifier, request *core.Record) error {
	if request.GetString("status") != "pending" {
		return fmt.Errorf("request %s is not pending", request.Id)
	}

	device, err := app.FindRecordById("devices", request.GetString("device"))
	if err != nil {
		return fmt.Errorf("loading device: %w", err)
	}

	request.Set("status", "withdrawn")
	request.Set("decided_at", types.NowDateTime())
	if err := app.Save(request); err != nil {
		return fmt.Errorf("withdrawing request: %w", err)
	}

	if err := revertDeviceIfNoPending(app, device); err != nil {
		return err
	}

	owner, err := app.FindRecordById("users", device.GetString("owner"))
	if err != nil {
		return fmt.Errorf("loading owner: %w", err)
	}
	requester, err := app.FindRecordById("users", request.GetString("requester"))
	if err != nil {
		return fmt.Errorf("loading requester: %w", err)
	}

	return notifier.RequestWithdrawn(owner, device, requester)
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/devices/... -run TestWithdraw -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/devices/withdraw.go device-lending/internal/devices/withdraw_test.go
git commit -m "Add Withdraw service function"
```

---

## Task 21: Service — mark a device returned

**Files:**
- Create: `device-lending/internal/devices/mark_returned.go`
- Test: `device-lending/internal/devices/mark_returned_test.go`

**Interfaces:**
- Consumes: `pendingRequests` (Task 17), `mail.Notifier` (Task 12).
- Produces: `func MarkReturned(app core.App, notifier *mail.Notifier, device *core.Record) error`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/devices/mark_returned_test.go
package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestMarkReturned_NoPendingGoesAvailable(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	device := newDevice(t, app, owner, "Drill", "lent")
	notifier := mail.New(app, "https://lending.example.com")

	if err := devices.MarkReturned(app, notifier, device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "available" {
		t.Errorf("expected device status 'available', got %q", updatedDevice.GetString("status"))
	}
	if updatedDevice.GetString("current_borrower") != "" {
		t.Errorf("expected current_borrower cleared, got %q", updatedDevice.GetString("current_borrower"))
	}
	if app.TestMailer.TotalSend() != 0 {
		t.Errorf("expected no emails when there are no pending requests, got %d", app.TestMailer.TotalSend())
	}
}

func TestMarkReturned_PendingRequestsGoBackToRequestedAndNotified(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newUser(t, app, "owner@example.com")
	waiting := newUser(t, app, "waiting@example.com")
	device := newDevice(t, app, owner, "Drill", "lent")
	newRequest(t, app, device, waiting, "pending")

	notifier := mail.New(app, "https://lending.example.com")
	app.TestMailer.Reset()

	if err := devices.MarkReturned(app, notifier, device); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	updatedDevice, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedDevice.GetString("status") != "requested" {
		t.Errorf("expected device status 'requested', got %q", updatedDevice.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to the waiting requester, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != waiting.Email() {
		t.Error("expected email to the waiting requester")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/devices/... -run TestMarkReturned -v`
Expected: FAIL with "undefined: devices.MarkReturned"

- [ ] **Step 3: Implement mark_returned.go**

```go
// internal/devices/mark_returned.go
package devices

import (
	"fmt"

	"github.com/pocketbase/pocketbase/core"

	"github.com/the-vas/device-lending/internal/mail"
)

func MarkReturned(app core.App, notifier *mail.Notifier, device *core.Record) error {
	if device.GetString("status") != "lent" {
		return fmt.Errorf("device %s is not currently lent", device.Id)
	}

	remaining, err := pendingRequests(app, device.Id)
	if err != nil {
		return fmt.Errorf("loading pending requests: %w", err)
	}

	device.Set("current_borrower", "")
	device.Set("lend_start", "")
	device.Set("lend_end", "")
	if len(remaining) > 0 {
		device.Set("status", "requested")
	} else {
		device.Set("status", "available")
	}
	if err := app.Save(device); err != nil {
		return fmt.Errorf("updating device: %w", err)
	}

	for _, req := range remaining {
		requester, err := app.FindRecordById("users", req.GetString("requester"))
		if err != nil {
			return fmt.Errorf("loading requester: %w", err)
		}
		if err := notifier.DeviceAvailableAgain(requester, device); err != nil {
			return fmt.Errorf("notifying requester: %w", err)
		}
	}

	return nil
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/devices/... -run TestMarkReturned -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/devices/mark_returned.go device-lending/internal/devices/mark_returned_test.go
git commit -m "Add MarkReturned service function"
```

---

## Task 22: Delete-device cascade hook

When a device with pending requests is deleted (per the API rule from Task 5, only possible while `status != "lent"`), auto-reject those pending requests and email each requester.

**Files:**
- Create: `device-lending/internal/devices/delete_cascade.go`
- Test: `device-lending/internal/devices/delete_cascade_test.go`

**Interfaces:**
- Consumes: `pendingRequests` (Task 17), `mail.Notifier` (Task 12).
- Produces: `func BindDeleteCascade(app core.App, notifier *mail.Notifier)`.

- [ ] **Step 1: Write the failing test**

```go
// internal/devices/delete_cascade_test.go
package devices_test

import (
	"testing"

	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestDeleteCascade_RejectsPendingAndNotifies(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	notifier := mail.New(app, "https://lending.example.com")
	devices.BindDeleteCascade(app, notifier)

	owner := newUser(t, app, "owner@example.com")
	waiting := newUser(t, app, "waiting@example.com")
	device := newDevice(t, app, owner, "Drill", "requested")
	req := newRequest(t, app, device, waiting, "pending")

	app.TestMailer.Reset()

	if err := app.Delete(device); err != nil {
		t.Fatalf("unexpected error deleting device: %v", err)
	}

	updatedReq, err := app.FindRecordById("lending_requests", req.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updatedReq.GetString("status") != "rejected" {
		t.Errorf("expected status 'rejected', got %q", updatedReq.GetString("status"))
	}

	if app.TestMailer.TotalSend() != 1 {
		t.Fatalf("expected 1 email to the waiting requester, got %d", app.TestMailer.TotalSend())
	}
	if app.TestMailer.LastMessage().To[0].Address != waiting.Email() {
		t.Error("expected email to the waiting requester")
	}
}
```

Note: this test calls `app.Delete(device)` directly (a Go-level delete, not through the HTTP API), which does **not** pass through `OnRecordDeleteRequest` — that hook only fires for API-triggered deletes. Use `OnRecordAfterDeleteSuccess` instead, which fires for both API and direct `app.Delete()` calls, matching how this cascade must also work when an admin cleans up records directly. Since the record is gone from the DB by the time this fires, capture the pending requests **before** calling delete internally is not possible here (we don't control the call site) — instead, bind on `OnRecordDeleteRequest` for the API path (Step 3 below) **and** query pending requests using the device id already embedded in each `lending_requests` record's `device` field, which remains queryable after the device row is gone (no foreign key cascade is configured, so orphaned `lending_requests` rows simply keep pointing at a now-missing device id). Implement the hook on `OnRecordAfterDeleteSuccess("devices")`, which fires after deletion in both cases and still receives `e.Record` with its original in-memory field values (id, name) for use in the notification email and the `pendingRequests` lookup by id.

- [ ] **Step 2: Run test to verify it fails**

Run: `go test ./internal/devices/... -run TestDeleteCascade -v`
Expected: FAIL with "undefined: devices.BindDeleteCascade"

- [ ] **Step 3: Implement delete_cascade.go**

```go
// internal/devices/delete_cascade.go
package devices

import (
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/mail"
)

func BindDeleteCascade(app core.App, notifier *mail.Notifier) {
	app.OnRecordAfterDeleteSuccess("devices").BindFunc(func(e *core.RecordEvent) error {
		pending, err := pendingRequests(e.App, e.Record.Id)
		if err != nil {
			return err
		}

		for _, req := range pending {
			req.Set("status", "rejected")
			req.Set("decided_at", types.NowDateTime())
			if err := e.App.Save(req); err != nil {
				return err
			}

			requester, err := e.App.FindRecordById("users", req.GetString("requester"))
			if err != nil {
				return err
			}
			if err := notifier.DeviceRemoved(requester, e.Record); err != nil {
				return err
			}
		}

		return e.Next()
	})
}
```

- [ ] **Step 4: Run test to verify it passes**

Run: `go test ./internal/devices/... -run TestDeleteCascade -v`
Expected: PASS

- [ ] **Step 5: Run the full devices package test suite**

Run: `go test ./internal/devices/... -v`
Expected: PASS (all of Tasks 11, 17–22)

- [ ] **Step 6: Wire the cascade hook and delete-guard into main.go**

Add to `main.go`, near the `devices.BindStateFieldGuard(app)` call from Task 11:

```go
	notifier := mail.New(app, cfg.BaseURL)
	devices.BindDeleteCascade(app, notifier)
```

Add `"github.com/the-vas/device-lending/internal/mail"` to imports. `notifier` is also threaded into the route handlers in Tasks 25–27.

- [ ] **Step 7: Run full build and test suite, then commit**

```bash
go build ./... && go test ./...
git add device-lending/internal/devices device-lending/main.go
git commit -m "Add delete-device cascade hook rejecting pending requests"
```

---

## Task 23: Base HTML layout, vendored static assets, and template rendering helper

Establishes the shared page-rendering helper and vendored frontend assets (htmx, Leaflet + OpenStreetMap tiles are self-hosted files, not CDN references, per the design doc). Every page task from Task 24 onward calls `web.Render`.

**Files:**
- Create: `device-lending/internal/web/templates.go`
- Create: `device-lending/internal/web/templates/layout.html`
- Create: `device-lending/internal/web/static/style.css`
- Create: `device-lending/internal/web/static/htmx.min.js` (vendored, Step 1)
- Create: `device-lending/internal/web/static/leaflet/leaflet.js`, `leaflet.css`, and marker images (vendored, Step 1)
- Test: `device-lending/internal/web/templates_test.go`

**Interfaces:**
- Produces:
  ```go
  package web

  //go:embed static
  var StaticFS embed.FS // consumed by main.go's OnServe route registration (Task 24)

  // Render loads layout.html plus the given content template file(s) and
  // executes them against data. data conventionally includes "Title",
  // "CurrentUser" (*core.Record or nil), and "IsAdmin" (bool), on top of
  // whatever page-specific keys the caller adds.
  func Render(contentFiles []string, data map[string]any) (string, error)
  ```

- [ ] **Step 1: Vendor htmx and Leaflet**

```bash
mkdir -p device-lending/internal/web/static/leaflet/images
curl -sL https://unpkg.com/htmx.org@2.0.4/dist/htmx.min.js -o device-lending/internal/web/static/htmx.min.js
curl -sL https://unpkg.com/leaflet@1.9.4/dist/leaflet.js -o device-lending/internal/web/static/leaflet/leaflet.js
curl -sL https://unpkg.com/leaflet@1.9.4/dist/leaflet.css -o device-lending/internal/web/static/leaflet/leaflet.css
for f in marker-icon.png marker-icon-2x.png marker-shadow.png; do
  curl -sL "https://unpkg.com/leaflet@1.9.4/dist/images/$f" -o "device-lending/internal/web/static/leaflet/images/$f"
done
```

These are committed to the repo (not fetched at runtime) so the deployed app has zero external asset dependencies. Note this in a short comment at the top of `leaflet.css` if the vendored file doesn't already reference `images/` relatively (Leaflet's default CSS does, so no edit should be needed — verify by checking the CSS references `images/marker-icon.png` with a relative, not absolute, path).

- [ ] **Step 2: Write layout.html**

```html
<!-- internal/web/templates/layout.html -->
<!doctype html>
<html lang="en">
<head>
<meta charset="utf-8">
<meta name="viewport" content="width=device-width, initial-scale=1">
<title>{{.Title}} · Device Lending</title>
<link rel="stylesheet" href="/static/style.css">
<script src="/static/htmx.min.js"></script>
</head>
<body>
<header class="site-header">
  <a class="brand" href="/">Device Lending</a>
  <nav>
    <a href="/">Browse</a>
    {{if .CurrentUser}}
      <a href="/my/devices">My devices</a>
      <a href="/my/requests">My requests</a>
      {{if .IsAdmin}}<a href="/admin">Admin</a>{{end}}
      <form action="/logout" method="post" class="inline"><button type="submit">Log out</button></form>
    {{else}}
      <a href="/oidc/login">Log in</a>
    {{end}}
  </nav>
</header>
<main>{{block "content" .}}{{end}}</main>
</body>
</html>
```

- [ ] **Step 3: Write style.css**

```css
/* internal/web/static/style.css */
:root {
  --accent: #2b6cb0;
  --border: #d9dde3;
  --text: #1a202c;
  --bg: #ffffff;
  --muted: #6b7280;
}

* { box-sizing: border-box; }

body {
  margin: 0;
  font-family: -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
  color: var(--text);
  background: var(--bg);
  line-height: 1.5;
}

.site-header {
  display: flex;
  justify-content: space-between;
  align-items: center;
  padding: 0.75rem 1.5rem;
  border-bottom: 1px solid var(--border);
}

.site-header .brand { font-weight: 600; text-decoration: none; color: var(--text); }
.site-header nav { display: flex; gap: 1rem; align-items: center; }
.site-header nav a { text-decoration: none; color: var(--text); }
.site-header nav form.inline { display: inline; margin: 0; }

main { max-width: 960px; margin: 0 auto; padding: 1.5rem; }

.device-grid {
  display: grid;
  grid-template-columns: repeat(auto-fill, minmax(220px, 1fr));
  gap: 1rem;
}

.device-card {
  border: 1px solid var(--border);
  border-radius: 8px;
  padding: 0.75rem;
  text-decoration: none;
  color: var(--text);
}

.device-card img { width: 100%; height: 140px; object-fit: cover; border-radius: 4px; }

.status-badge {
  display: inline-block;
  font-size: 0.75rem;
  padding: 0.15rem 0.5rem;
  border-radius: 999px;
  background: #edf2f7;
  color: var(--muted);
}

.status-available { background: #e6fffa; color: #047857; }
.status-requested { background: #fefcbf; color: #92400e; }
.status-lent { background: #fee2e2; color: #b91c1c; }
.status-unavailable { background: #edf2f7; color: var(--muted); }

button, input[type="submit"] {
  background: var(--accent);
  color: white;
  border: none;
  padding: 0.5rem 1rem;
  border-radius: 6px;
  cursor: pointer;
}

form.inline button { background: none; color: var(--muted); text-decoration: underline; padding: 0; }

input, textarea, select {
  padding: 0.4rem;
  border: 1px solid var(--border);
  border-radius: 4px;
  width: 100%;
}

#device-map, .device-map-picker { height: 300px; border-radius: 8px; }
```

- [ ] **Step 4: Write templates.go**

```go
// internal/web/templates.go
package web

import (
	"embed"

	pbtemplate "github.com/pocketbase/pocketbase/tools/template"
)

//go:embed templates
var templatesFS embed.FS

//go:embed static
var StaticFS embed.FS

var registry = pbtemplate.NewRegistry()

func Render(contentFiles []string, data map[string]any) (string, error) {
	files := append([]string{"templates/layout.html"}, contentFiles...)
	renderer := registry.LoadFS(templatesFS, files...)
	return renderer.Render(data)
}
```

- [ ] **Step 5: Write the failing test**

```go
// internal/web/templates_test.go
package web_test

import (
	"strings"
	"testing"

	"github.com/the-vas/device-lending/internal/web"
)

func TestRender_LayoutWithContent(t *testing.T) {
	// index.html isn't written until Task 24; this test exercises the
	// layout alone via a minimal inline content template registered
	// through the same embedded filesystem is not possible here, so it
	// instead asserts on the layout's always-present chrome by rendering
	// with no CurrentUser and checking the anonymous nav state.
	html, err := web.Render(nil, map[string]any{
		"Title":       "Browse",
		"CurrentUser": nil,
		"IsAdmin":     false,
	})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !strings.Contains(html, "Device Lending") {
		t.Error("expected title to contain 'Device Lending'")
	}
	if !strings.Contains(html, `href="/oidc/login"`) {
		t.Error("expected anonymous nav to show a login link")
	}
	if strings.Contains(html, "/my/devices") {
		t.Error("expected anonymous nav to NOT show 'My devices'")
	}
}
```

- [ ] **Step 6: Run test to verify it fails, then implement, then verify it passes**

Run: `go test ./internal/web/... -v` — expect FAIL first (package doesn't exist / compile error), then after Steps 1–4 above are in place, re-run and expect PASS.

- [ ] **Step 7: Wire static file serving into main.go**

Add to the `OnServe` block in `main.go`:

```go
	staticSub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatal(err)
	}
```

(hoist this above `app.OnServe().BindFunc(...)`, since it only needs to run once) and inside the `OnServe` block:

```go
		se.Router.GET("/static/{path...}", apis.Static(staticSub, false))
```

Add `"io/fs"` and `"github.com/the-vas/device-lending/internal/web"` to imports.

- [ ] **Step 8: Run full build and test suite, then commit**

```bash
go build ./... && go test ./...
git add device-lending/internal/web device-lending/main.go
git commit -m "Add base HTML layout, vendored htmx/Leaflet assets, and render helper"
```

---

## Task 24: Browse/search page (`GET /`)

**Important correctness note:** this handler queries the `devices` collection directly via `app.FindRecordsByFilter`, which is a Go-level DB call that bypasses PocketBase's API rules entirely (those only apply to the REST API, which this server-rendered page doesn't use). That means the `PUBLIC_READ` toggle from Task 10 — which only governs the `devices.ListRule`/`ViewRule` API rules — has **no effect** on this page unless the handler re-checks it explicitly. This task's handler takes `publicRead bool` as a parameter and gates the whole page on it, so the same policy applies consistently whether a client hits the REST API or this page.

**Files:**
- Create: `device-lending/internal/web/browse.go`
- Create: `device-lending/internal/web/templates/index.html`
- Test: `device-lending/internal/web/browse_test.go`

**Interfaces:**
- Consumes: `Render` (Task 23).
- Produces: `func BrowseHandler(app core.App, publicRead bool) func(e *core.RequestEvent) error`, registered at `GET /` in Task 30's final route wiring.

- [ ] **Step 1: Write the failing tests**

```go
// internal/web/browse_test.go
package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/web"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func newTestOwner(t *testing.T, app core.App) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(users)
	r.SetEmail("owner@example.com")
	r.SetRandomPassword()
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func newTestDeviceForBrowse(t *testing.T, app core.App, owner *core.Record, name string) *core.Record {
	t.Helper()
	categories, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatal(err)
	}
	category := core.NewRecord(categories)
	category.Set("name", "Category for "+name)
	if err := app.Save(category); err != nil {
		t.Fatal(err)
	}
	devicesCol, err := app.FindCollectionByNameOrId("devices")
	if err != nil {
		t.Fatal(err)
	}
	d := core.NewRecord(devicesCol)
	d.Set("name", name)
	d.Set("category", category.Id)
	d.Set("owner", owner.Id)
	d.Set("status", "available")
	if err := app.Save(d); err != nil {
		t.Fatal(err)
	}
	return d
}

func buildMux(t *testing.T, app *tests.TestApp, register func(e *core.ServeEvent)) http.Handler {
	t.Helper()
	router, err := apis.NewRouter(app)
	if err != nil {
		t.Fatal(err)
	}
	serveEvent := &core.ServeEvent{App: app, Router: router}
	err = app.OnServe().Trigger(serveEvent, func(e *core.ServeEvent) error {
		register(e)
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

func TestBrowseHandler_ListsAndFilters(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestOwner(t, app)
	newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	newTestDeviceForBrowse(t, app, owner, "Circular Saw")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.GET("/", web.BrowseHandler(app, true))
	})

	req := httptest.NewRequest(http.MethodGet, "/?q=Drill", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cordless Drill") {
		t.Error("expected result to contain 'Cordless Drill'")
	}
	if strings.Contains(rec.Body.String(), "Circular Saw") {
		t.Error("expected search filter to exclude 'Circular Saw'")
	}
}

func TestBrowseHandler_RequiresAuthWhenNotPublic(t *testing.T) {
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
	if rec.Header().Get("Location") != "/oidc/login" {
		t.Errorf("expected redirect to /oidc/login, got %s", rec.Header().Get("Location"))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/... -run TestBrowseHandler -v`
Expected: FAIL with "undefined: web.BrowseHandler"

- [ ] **Step 3: Write index.html**

```html
<!-- internal/web/templates/index.html -->
{{define "content"}}
<h1>Devices</h1>
<form method="get" action="/">
  <input type="search" name="q" placeholder="Search devices…" value="{{.Query}}">
</form>
<div class="device-grid">
  {{range .Devices}}
  <a class="device-card" href="/devices/{{.ID}}">
    {{if .PhotoURL}}<img src="{{.PhotoURL}}" alt="{{.Name}}">{{end}}
    <h3>{{.Name}}</h3>
    <span class="status-badge status-{{.Status}}">{{.Status}}</span>
    {{if .LocationLabel}}<p>{{.LocationLabel}}</p>{{end}}
  </a>
  {{else}}
  <p>No devices found.</p>
  {{end}}
</div>
{{end}}
```

- [ ] **Step 4: Implement browse.go**

```go
// internal/web/browse.go
package web

import (
	"fmt"
	"net/http"
	"strings"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type deviceListItem struct {
	ID            string
	Name          string
	Status        string
	LocationLabel string
	PhotoURL      string
}

func devicePhotoURL(r *core.Record) string {
	filename := r.GetString("photo")
	if filename == "" {
		return ""
	}
	return fmt.Sprintf("/api/files/devices/%s/%s", r.Id, filename)
}

func BrowseHandler(app core.App, publicRead bool) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if !publicRead && e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}

		q := strings.TrimSpace(e.Request.URL.Query().Get("q"))

		filter := ""
		params := dbx.Params{}
		if q != "" {
			filter = "name ~ {:q}"
			params["q"] = q
		}

		records, err := app.FindRecordsByFilter("devices", filter, "name", 0, 0, params)
		if err != nil {
			return e.InternalServerError("failed to load devices", err)
		}

		items := make([]deviceListItem, 0, len(records))
		for _, r := range records {
			items = append(items, deviceListItem{
				ID:            r.Id,
				Name:          r.GetString("name"),
				Status:        r.GetString("status"),
				LocationLabel: r.GetString("location_label"),
				PhotoURL:      devicePhotoURL(r),
			})
		}

		isAdmin := e.Auth != nil && e.Auth.GetBool("is_admin")
		html, err := Render([]string{"templates/index.html"}, map[string]any{
			"Title":       "Browse",
			"CurrentUser": e.Auth,
			"IsAdmin":     isAdmin,
			"Devices":     items,
			"Query":       q,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}

		return e.HTML(http.StatusOK, html)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/web/... -run TestBrowseHandler -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add device-lending/internal/web/browse.go device-lending/internal/web/templates/index.html device-lending/internal/web/browse_test.go
git commit -m "Add device browse/search page"
```

---

## Task 25: Device detail page, request-to-borrow, and withdraw

The template also renders owner-only action forms (edit/delete/return/handover/reject) whose routes are registered in Task 27; until then those forms simply 404 on submit, which is fine since this task's tests only exercise the detail view, request, and withdraw paths.

**Files:**
- Create: `device-lending/internal/web/device_detail.go`
- Create: `device-lending/internal/web/templates/device_detail.html`
- Test: `device-lending/internal/web/device_detail_test.go`

**Interfaces:**
- Consumes: `Render` (Task 23), `devicePhotoURL` (Task 24), `devices.CreateRequest`/`devices.Withdraw` (Tasks 17, 20), `mail.Notifier` (Task 12).
- Produces:
  ```go
  func DeviceDetailHandler(app core.App, publicRead bool) func(e *core.RequestEvent) error
  func RequestDeviceHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error
  func WithdrawRequestHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error
  ```
  Registered at `GET /devices/{id}`, `POST /devices/{id}/request`, `POST /devices/{id}/requests/{reqId}/withdraw` in Task 30.

- [ ] **Step 1: Write the failing tests**

```go
// internal/web/device_detail_test.go
package web_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"
	"github.com/the-vas/device-lending/internal/web"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func newTestUserFor(t *testing.T, app core.App, email string) *core.Record {
	t.Helper()
	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(users)
	r.SetEmail(email)
	r.SetRandomPassword()
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func TestDeviceDetailHandler_RendersDevice(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.GET("/devices/{id}", web.DeviceDetailHandler(app, true))
	})

	req := httptest.NewRequest(http.MethodGet, "/devices/"+device.Id, nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cordless Drill") {
		t.Error("expected page to contain device name")
	}
}

func TestDeviceDetailHandler_HidesBorrowerFromAnonymous(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	borrower := newTestUserFor(t, app, "borrower@example.com")
	borrower.Set("name", "Secret Borrower")
	if err := app.Save(borrower); err != nil {
		t.Fatal(err)
	}
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	device.Set("status", "lent")
	device.Set("current_borrower", borrower.Id)
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}

	anonMux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.GET("/devices/{id}", web.DeviceDetailHandler(app, true))
	})
	req := httptest.NewRequest(http.MethodGet, "/devices/"+device.Id, nil)
	rec := httptest.NewRecorder()
	anonMux.ServeHTTP(rec, req)
	if strings.Contains(rec.Body.String(), "Secret Borrower") {
		t.Error("expected anonymous visitor to NOT see the borrower's name")
	}

	authedMux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.GET("/devices/{id}", web.DeviceDetailHandler(app, true))
	})
	req2 := httptest.NewRequest(http.MethodGet, "/devices/"+device.Id, nil)
	rec2 := httptest.NewRecorder()
	authedMux.ServeHTTP(rec2, req2)
	if !strings.Contains(rec2.Body.String(), "Secret Borrower") {
		t.Error("expected an authenticated visitor to see the borrower's name")
	}
}

func TestRequestDeviceHandler_CreatesPendingRequest(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	requester := newTestUserFor(t, app, "requester@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	notifier := mail.New(app, "https://lending.example.com")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.POST("/devices/{id}/request", web.RequestDeviceHandler(app, notifier))
	})

	form := url.Values{"requested_start": {"2026-08-01"}, "message": {"pretty please"}}
	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/request", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")

	// simulate an authenticated session by setting e.Auth directly is not
	// possible from outside the handler chain, so bind a tiny middleware
	// that stands in for webauth.LoadSession in this isolated test.
	router2 := mux
	_ = router2
	rec := httptest.NewRecorder()

	// Re-register with an auth-injecting middleware ahead of the route.
	mux2 := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = requester
			return re.Next()
		})
		e.Router.POST("/devices/{id}/request", web.RequestDeviceHandler(app, notifier))
	})
	mux2.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	pending, err := app.FindRecordsByFilter("lending_requests", "device = {:d} && requester = {:r}", "", 0, 0, map[string]any{"d": device.Id, "r": requester.Id})
	if err != nil {
		t.Fatal(err)
	}
	if len(pending) != 1 {
		t.Fatalf("expected 1 request created, got %d", len(pending))
	}
}

func TestWithdrawRequestHandler_ForbidsOtherUsers(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	requester := newTestUserFor(t, app, "requester@example.com")
	intruder := newTestUserFor(t, app, "intruder@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	device.Set("status", "requested")
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}
	requestsCol, err := app.FindCollectionByNameOrId("lending_requests")
	if err != nil {
		t.Fatal(err)
	}
	request := core.NewRecord(requestsCol)
	request.Set("device", device.Id)
	request.Set("requester", requester.Id)
	request.Set("status", "pending")
	request.Set("requested_start", "2026-08-01 00:00:00.000Z")
	if err := app.Save(request); err != nil {
		t.Fatal(err)
	}

	notifier := mail.New(app, "https://lending.example.com")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = intruder
			return re.Next()
		})
		e.Router.POST("/devices/{id}/requests/{reqId}/withdraw", web.WithdrawRequestHandler(app, notifier))
	})

	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/requests/"+request.Id+"/withdraw", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/... -run "TestDeviceDetailHandler|TestRequestDeviceHandler|TestWithdrawRequestHandler" -v`
Expected: FAIL with "undefined: web.DeviceDetailHandler"

- [ ] **Step 3: Write device_detail.html**

```html
<!-- internal/web/templates/device_detail.html -->
{{define "content"}}
<h1>{{.Device.GetString "name"}}</h1>
<span class="status-badge status-{{.Device.GetString "status"}}">{{.Device.GetString "status"}}</span>
{{if .PhotoURL}}<p><img src="{{.PhotoURL}}" alt="{{.Device.GetString "name"}}" style="max-width:300px"></p>{{end}}
<p>{{.Device.GetString "description"}}</p>
<p>Category: {{.CategoryName}}</p>
<p>Owner: {{.OwnerName}}</p>
{{if .LocationLabel}}<p>Location: {{.LocationLabel}}</p>{{end}}
{{if .BorrowerName}}<p>Currently with: {{.BorrowerName}}</p>{{end}}

<link rel="stylesheet" href="/static/leaflet/leaflet.css">
<div id="device-map"></div>
<script src="/static/leaflet/leaflet.js"></script>
<script>
  var map = L.map('device-map').setView([{{.Lat}}, {{.Lon}}], 14);
  L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {maxZoom: 19}).addTo(map);
  L.marker([{{.Lat}}, {{.Lon}}]).addTo(map);
</script>

{{if .CanRequest}}
<h2>Request to borrow</h2>
<form method="post" action="/devices/{{.Device.Id}}/request">
  <label>Start date <input type="date" name="requested_start" required></label>
  <label>End date (optional) <input type="date" name="requested_end"></label>
  <label>Message <textarea name="message"></textarea></label>
  <button type="submit">Request</button>
</form>
{{end}}

{{if .MyPendingRequest}}
<p>You have a pending request for this device.</p>
<form method="post" action="/devices/{{.Device.Id}}/requests/{{.MyPendingRequest.Id}}/withdraw">
  <button type="submit">Withdraw request</button>
</form>
{{end}}

{{if .IsOwner}}
<h2>Owner actions</h2>
<p><a href="/devices/{{.Device.Id}}/edit">Edit device</a></p>
<form method="post" action="/devices/{{.Device.Id}}/delete" class="inline"><button type="submit">Delete device</button></form>

{{if eq (.Device.GetString "status") "lent"}}
<form method="post" action="/devices/{{.Device.Id}}/return" class="inline"><button type="submit">Mark returned</button></form>
{{end}}

{{if .PendingRequests}}
<h3>Pending requests</h3>
<ul>
{{range .PendingRequests}}
<li>
  {{.RequesterName}} ({{.RequesterEmail}}) — {{.RequestedStart}} to {{.RequestedEnd}} — {{.Message}}
  <form method="post" action="/devices/{{$.Device.Id}}/handover" class="inline">
    <input type="hidden" name="request_id" value="{{.ID}}">
    <input type="date" name="lend_end">
    <button type="submit">Hand over to this person</button>
  </form>
  <form method="post" action="/devices/{{$.Device.Id}}/requests/{{.ID}}/reject" class="inline">
    <button type="submit">Reject</button>
  </form>
</li>
{{end}}
</ul>
{{end}}
{{end}}
{{end}}
```

- [ ] **Step 4: Implement device_detail.go**

```go
// internal/web/device_detail.go
package web

import (
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"
)

type pendingRequestView struct {
	ID             string
	RequesterName  string
	RequesterEmail string
	RequestedStart string
	RequestedEnd   string
	Message        string
}

func DeviceDetailHandler(app core.App, publicRead bool) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if !publicRead && e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}

		id := e.Request.PathValue("id")
		device, err := app.FindRecordById("devices", id)
		if err != nil {
			return e.NotFoundError("device not found", err)
		}

		category, err := app.FindRecordById("categories", device.GetString("category"))
		if err != nil {
			return e.InternalServerError("failed to load category", err)
		}
		owner, err := app.FindRecordById("users", device.GetString("owner"))
		if err != nil {
			return e.InternalServerError("failed to load owner", err)
		}

		var myPendingRequest *core.Record
		if e.Auth != nil {
			myPendingRequest, _ = app.FindFirstRecordByFilter(
				"lending_requests",
				"device = {:device} && requester = {:requester} && status = 'pending'",
				dbx.Params{"device": device.Id, "requester": e.Auth.Id},
			)
		}

		isOwner := e.Auth != nil && device.GetString("owner") == e.Auth.Id
		isAdmin := e.Auth != nil && e.Auth.GetBool("is_admin")
		status := device.GetString("status")

		// Borrower identity is only shown to authenticated users (design
		// doc requirement), never to anonymous visitors even when
		// PUBLIC_READ is enabled.
		borrowerName := ""
		if e.Auth != nil && status == "lent" {
			if borrower, err := app.FindRecordById("users", device.GetString("current_borrower")); err == nil {
				borrowerName = borrower.GetString("name")
			}
		}

		data := map[string]any{
			"Title":            device.GetString("name"),
			"CurrentUser":      e.Auth,
			"IsAdmin":          isAdmin,
			"Device":           device,
			"CategoryName":     category.GetString("name"),
			"OwnerName":        owner.GetString("name"),
			"BorrowerName":     borrowerName,
			"PhotoURL":         devicePhotoURL(device),
			"IsOwner":          isOwner || isAdmin,
			"CanRequest":       e.Auth != nil && !isOwner && myPendingRequest == nil && (status == "available" || status == "requested"),
			"MyPendingRequest": myPendingRequest,
			"Lat":              device.GetGeoPoint("location_point").Lat,
			"Lon":              device.GetGeoPoint("location_point").Lon,
			"LocationLabel":    device.GetString("location_label"),
		}

		if isOwner || isAdmin {
			pending, err := app.FindRecordsByFilter(
				"lending_requests",
				"device = {:device} && status = 'pending'",
				"created", 0, 0,
				dbx.Params{"device": device.Id},
			)
			if err != nil {
				return e.InternalServerError("failed to load pending requests", err)
			}

			views := make([]pendingRequestView, 0, len(pending))
			for _, p := range pending {
				requester, err := app.FindRecordById("users", p.GetString("requester"))
				if err != nil {
					return e.InternalServerError("failed to load requester", err)
				}
				views = append(views, pendingRequestView{
					ID:             p.Id,
					RequesterName:  requester.GetString("name"),
					RequesterEmail: requester.Email(),
					RequestedStart: p.GetDateTime("requested_start").String(),
					RequestedEnd:   p.GetDateTime("requested_end").String(),
					Message:        p.GetString("message"),
				})
			}
			data["PendingRequests"] = views
		}

		html, err := Render([]string{"templates/device_detail.html"}, data)
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}

		return e.HTML(http.StatusOK, html)
	}
}

func RequestDeviceHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}

		id := e.Request.PathValue("id")
		device, err := app.FindRecordById("devices", id)
		if err != nil {
			return e.NotFoundError("device not found", err)
		}

		if err := e.Request.ParseForm(); err != nil {
			return e.BadRequestError("invalid form data", err)
		}

		start, err := types.ParseDateTime(e.Request.FormValue("requested_start"))
		if err != nil {
			return e.BadRequestError("invalid start date", err)
		}

		var end types.DateTime
		if v := e.Request.FormValue("requested_end"); v != "" {
			end, err = types.ParseDateTime(v)
			if err != nil {
				return e.BadRequestError("invalid end date", err)
			}
		}

		if _, err := devices.CreateRequest(app, notifier, device, e.Auth, start, end, e.Request.FormValue("message")); err != nil {
			return e.InternalServerError("failed to submit request", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}

func WithdrawRequestHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}

		reqID := e.Request.PathValue("reqId")
		request, err := app.FindRecordById("lending_requests", reqID)
		if err != nil {
			return e.NotFoundError("request not found", err)
		}

		if request.GetString("requester") != e.Auth.Id && !e.Auth.GetBool("is_admin") {
			return e.ForbiddenError("not your request", nil)
		}

		if err := devices.Withdraw(app, notifier, request); err != nil {
			return e.InternalServerError("failed to withdraw request", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+request.GetString("device"))
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/web/... -run "TestDeviceDetailHandler|TestRequestDeviceHandler|TestWithdrawRequestHandler" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add device-lending/internal/web/device_detail.go device-lending/internal/web/templates/device_detail.html device-lending/internal/web/device_detail_test.go
git commit -m "Add device detail page with request/withdraw actions"
```

---

## Task 26: Device create/edit form (map picker + photo upload)

**Files:**
- Create: `device-lending/internal/web/device_form.go`
- Create: `device-lending/internal/web/templates/device_form.html`
- Test: `device-lending/internal/web/device_form_test.go`

**Interfaces:**
- Consumes: `Render` (Task 23).
- Produces:
  ```go
  func NewDeviceFormHandler(app core.App) func(e *core.RequestEvent) error
  func CreateDeviceHandler(app core.App) func(e *core.RequestEvent) error
  func EditDeviceFormHandler(app core.App) func(e *core.RequestEvent) error
  func UpdateDeviceHandler(app core.App) func(e *core.RequestEvent) error
  ```
  Registered at `GET/POST /devices/new` and `GET/POST /devices/{id}/edit` in Task 30.

- [ ] **Step 1: Write the failing tests**

```go
// internal/web/device_form_test.go
package web_test

import (
	"bytes"
	"mime/multipart"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/web"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func newTestCategory(t *testing.T, app core.App, name string) *core.Record {
	t.Helper()
	col, err := app.FindCollectionByNameOrId("categories")
	if err != nil {
		t.Fatal(err)
	}
	r := core.NewRecord(col)
	r.Set("name", name)
	if err := app.Save(r); err != nil {
		t.Fatal(err)
	}
	return r
}

func multipartDeviceForm(t *testing.T, fields map[string]string) (*bytes.Buffer, string) {
	t.Helper()
	var buf bytes.Buffer
	w := multipart.NewWriter(&buf)
	for k, v := range fields {
		if err := w.WriteField(k, v); err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	return &buf, w.FormDataContentType()
}

func TestCreateDeviceHandler_SetsOwnerAndSaves(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	category := newTestCategory(t, app, "Power tools")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.POST("/devices/new", web.CreateDeviceHandler(app))
	})

	body, contentType := multipartDeviceForm(t, map[string]string{
		"name":           "Cordless Drill",
		"description":    "18V, cordless",
		"category":       category.Id,
		"location_label": "Garage",
		"lat":            "52.5",
		"lon":            "13.4",
	})

	req := httptest.NewRequest(http.MethodPost, "/devices/new", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	created, err := app.FindRecordsByFilter("devices", "name = {:n}", "", 0, 0, map[string]any{"n": "Cordless Drill"})
	if err != nil {
		t.Fatal(err)
	}
	if len(created) != 1 {
		t.Fatalf("expected 1 device created, got %d", len(created))
	}
	if created[0].GetString("owner") != owner.Id {
		t.Errorf("expected owner %q, got %q", owner.Id, created[0].GetString("owner"))
	}
	if created[0].GetString("status") != "available" {
		t.Errorf("expected default status 'available', got %q", created[0].GetString("status"))
	}
	if created[0].GetGeoPoint("location_point").Lat != 52.5 {
		t.Errorf("expected lat 52.5, got %v", created[0].GetGeoPoint("location_point").Lat)
	}
}

func TestUpdateDeviceHandler_ForbidsNonOwner(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	intruder := newTestUserFor(t, app, "intruder@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = intruder
			return re.Next()
		})
		e.Router.POST("/devices/{id}/edit", web.UpdateDeviceHandler(app))
	})

	body, contentType := multipartDeviceForm(t, map[string]string{"name": "Hacked name"})
	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/edit", body)
	req.Header.Set("Content-Type", contentType)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/... -run "TestCreateDeviceHandler|TestUpdateDeviceHandler" -v`
Expected: FAIL with "undefined: web.CreateDeviceHandler"

- [ ] **Step 3: Write device_form.html**

```html
<!-- internal/web/templates/device_form.html -->
{{define "content"}}
<h1>{{if .IsEdit}}Edit device{{else}}New device{{end}}</h1>
<form method="post" action="{{if .IsEdit}}/devices/{{.Device.Id}}/edit{{else}}/devices/new{{end}}" enctype="multipart/form-data">
  <label>Name <input type="text" name="name" required value="{{if .Device}}{{.Device.GetString "name"}}{{end}}"></label>
  <label>Description <textarea name="description">{{if .Device}}{{.Device.GetString "description"}}{{end}}</textarea></label>
  <label>Category
    <select name="category" required>
      {{range .Categories}}<option value="{{.Id}}">{{.GetString "name"}}</option>{{end}}
    </select>
  </label>
  <label>Location label <input type="text" name="location_label" value="{{if .Device}}{{.Device.GetString "location_label"}}{{end}}"></label>

  <link rel="stylesheet" href="/static/leaflet/leaflet.css">
  <div id="location-picker" class="device-map-picker"></div>
  <input type="hidden" name="lat" id="lat-input">
  <input type="hidden" name="lon" id="lon-input">
  <script src="/static/leaflet/leaflet.js"></script>
  <script>
    var pickMap = L.map('location-picker').setView([51.505, -0.09], 4);
    L.tileLayer('https://tile.openstreetmap.org/{z}/{x}/{y}.png', {maxZoom: 19}).addTo(pickMap);
    var marker;
    pickMap.on('click', function(ev) {
      document.getElementById('lat-input').value = ev.latlng.lat;
      document.getElementById('lon-input').value = ev.latlng.lng;
      if (marker) { marker.setLatLng(ev.latlng); } else { marker = L.marker(ev.latlng).addTo(pickMap); }
    });
  </script>

  <label>Photo <input type="file" name="photo" accept="image/*"></label>
  <button type="submit">Save</button>
</form>
{{end}}
```

- [ ] **Step 4: Implement device_form.go**

```go
// internal/web/device_form.go
package web

import (
	"errors"
	"net/http"
	"strconv"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"
)

func loadCategories(app core.App) ([]*core.Record, error) {
	return app.FindRecordsByFilter("categories", "", "name", 0, 0)
}

func bindDeviceForm(e *core.RequestEvent, record *core.Record) error {
	if err := e.Request.ParseMultipartForm(10 << 20); err != nil {
		return err
	}

	record.Set("name", e.Request.FormValue("name"))
	record.Set("description", e.Request.FormValue("description"))
	record.Set("category", e.Request.FormValue("category"))
	record.Set("location_label", e.Request.FormValue("location_label"))

	latStr, lonStr := e.Request.FormValue("lat"), e.Request.FormValue("lon")
	if latStr != "" && lonStr != "" {
		lat, err := strconv.ParseFloat(latStr, 64)
		if err != nil {
			return err
		}
		lon, err := strconv.ParseFloat(lonStr, 64)
		if err != nil {
			return err
		}
		record.Set("location_point", types.GeoPoint{Lat: lat, Lon: lon})
	}

	files, err := e.FindUploadedFiles("photo")
	if err != nil && !errors.Is(err, http.ErrMissingFile) {
		return err
	}
	if len(files) > 0 {
		record.Set("photo", files)
	}

	return nil
}

func NewDeviceFormHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}

		categories, err := loadCategories(app)
		if err != nil {
			return e.InternalServerError("failed to load categories", err)
		}

		html, err := Render([]string{"templates/device_form.html"}, map[string]any{
			"Title":       "New device",
			"CurrentUser": e.Auth,
			"IsAdmin":     e.Auth.GetBool("is_admin"),
			"Categories":  categories,
			"Device":      nil,
			"IsEdit":      false,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}
		return e.HTML(http.StatusOK, html)
	}
}

func CreateDeviceHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}

		collection, err := app.FindCollectionByNameOrId("devices")
		if err != nil {
			return e.InternalServerError("failed to load devices collection", err)
		}

		record := core.NewRecord(collection)
		record.Set("owner", e.Auth.Id)
		record.Set("status", "available")

		if err := bindDeviceForm(e, record); err != nil {
			return e.BadRequestError("invalid form data", err)
		}
		if err := app.Save(record); err != nil {
			return e.BadRequestError("failed to save device", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+record.Id)
	}
}

func EditDeviceFormHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}

		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if device.GetString("owner") != e.Auth.Id && !e.Auth.GetBool("is_admin") {
			return e.ForbiddenError("not your device", nil)
		}

		categories, err := loadCategories(app)
		if err != nil {
			return e.InternalServerError("failed to load categories", err)
		}

		html, err := Render([]string{"templates/device_form.html"}, map[string]any{
			"Title":       "Edit " + device.GetString("name"),
			"CurrentUser": e.Auth,
			"IsAdmin":     e.Auth.GetBool("is_admin"),
			"Categories":  categories,
			"Device":      device,
			"IsEdit":      true,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}
		return e.HTML(http.StatusOK, html)
	}
}

func UpdateDeviceHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}

		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if device.GetString("owner") != e.Auth.Id && !e.Auth.GetBool("is_admin") {
			return e.ForbiddenError("not your device", nil)
		}

		if err := bindDeviceForm(e, device); err != nil {
			return e.BadRequestError("invalid form data", err)
		}
		if err := app.Save(device); err != nil {
			return e.BadRequestError("failed to save device", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}
```

Note: `bindDeviceForm` only ever sets `name`, `description`, `category`, `location_label`, `location_point`, `photo` — never `status`, `current_borrower`, `lend_start`, or `lend_end` — so this legitimate edit path never collides with Task 11's state-field guard (which only inspects requests hitting the public REST API update endpoint, not this Go code's own `app.Save` calls).

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/web/... -run "TestCreateDeviceHandler|TestUpdateDeviceHandler" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add device-lending/internal/web/device_form.go device-lending/internal/web/templates/device_form.html device-lending/internal/web/device_form_test.go
git commit -m "Add device create/edit form with map picker and photo upload"
```

---

## Task 27: Owner action routes (handover, mark returned, reject, unavailable toggle, delete)

Each handler re-implements the owner-or-admin authorization check in Go, for the same reason as Tasks 24–26: these routes call the service layer / `app.Save` / `app.Delete` directly, bypassing PocketBase's REST API rules entirely. `DeleteDeviceHandler` calls `app.Delete()` directly (not through the REST API), which is exactly why Task 22's cascade hook was bound on `OnRecordAfterDeleteSuccess` rather than `OnRecordDeleteRequest` — this is the call site that only the after-success hook variant would catch.

**Files:**
- Create: `device-lending/internal/web/owner_actions.go`
- Test: `device-lending/internal/web/owner_actions_test.go`

**Interfaces:**
- Consumes: `devices.Handover`/`Reject`/`MarkReturned` (Tasks 18, 19, 21), `mail.Notifier` (Task 12).
- Produces:
  ```go
  func HandoverHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error
  func MarkReturnedHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error
  func RejectRequestHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error
  func ToggleUnavailableHandler(app core.App) func(e *core.RequestEvent) error
  func DeleteDeviceHandler(app core.App) func(e *core.RequestEvent) error
  ```
  Registered at `POST /devices/{id}/handover`, `POST /devices/{id}/return`, `POST /devices/{id}/requests/{reqId}/reject`, `POST /devices/{id}/toggle-unavailable`, `POST /devices/{id}/delete` in Task 30.

- [ ] **Step 1: Write the failing tests**

```go
// internal/web/owner_actions_test.go
package web_test

import (
	"net/http"
	"net/http/httptest"
	"net/url"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/mail"
	"github.com/the-vas/device-lending/internal/web"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestHandoverHandler_OwnerHandsOverToRequester(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	requester := newTestUserFor(t, app, "requester@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	device.Set("status", "requested")
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}
	requestsCol, err := app.FindCollectionByNameOrId("lending_requests")
	if err != nil {
		t.Fatal(err)
	}
	request := core.NewRecord(requestsCol)
	request.Set("device", device.Id)
	request.Set("requester", requester.Id)
	request.Set("status", "pending")
	request.Set("requested_start", "2026-08-01 00:00:00.000Z")
	if err := app.Save(request); err != nil {
		t.Fatal(err)
	}

	notifier := mail.New(app, "https://lending.example.com")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.POST("/devices/{id}/handover", web.HandoverHandler(app, notifier))
	})

	form := url.Values{"request_id": {request.Id}, "lend_end": {"2026-08-10"}}
	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/handover", strings.NewReader(form.Encode()))
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	updated, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetString("status") != "lent" {
		t.Errorf("expected status 'lent', got %q", updated.GetString("status"))
	}
	if updated.GetString("current_borrower") != requester.Id {
		t.Errorf("expected current_borrower %q, got %q", requester.Id, updated.GetString("current_borrower"))
	}
}

func TestDeleteDeviceHandler_BlocksWhileLent(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	device.Set("status", "lent")
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.POST("/devices/{id}/delete", web.DeleteDeviceHandler(app))
	})

	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/delete", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}

	stillThere, err := app.FindRecordById("devices", device.Id)
	if err != nil || stillThere == nil {
		t.Error("expected device to still exist after a blocked delete")
	}
}

func TestToggleUnavailableHandler_RoundTrips(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.POST("/devices/{id}/toggle-unavailable", web.ToggleUnavailableHandler(app))
	})

	req := httptest.NewRequest(http.MethodPost, "/devices/"+device.Id+"/toggle-unavailable", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)
	if rec.Code != http.StatusFound {
		t.Fatalf("expected 302, got %d: %s", rec.Code, rec.Body.String())
	}

	updated, err := app.FindRecordById("devices", device.Id)
	if err != nil {
		t.Fatal(err)
	}
	if updated.GetString("status") != "unavailable" {
		t.Errorf("expected status 'unavailable', got %q", updated.GetString("status"))
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/... -run "TestHandoverHandler|TestDeleteDeviceHandler|TestToggleUnavailableHandler" -v`
Expected: FAIL with "undefined: web.HandoverHandler"

- [ ] **Step 3: Implement owner_actions.go**

```go
// internal/web/owner_actions.go
package web

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tools/types"

	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"
)

func requireOwnerOrAdmin(e *core.RequestEvent, device *core.Record) error {
	if e.Auth == nil {
		return e.Redirect(http.StatusFound, "/oidc/login")
	}
	if device.GetString("owner") != e.Auth.Id && !e.Auth.GetBool("is_admin") {
		return e.ForbiddenError("not your device", nil)
	}
	return nil
}

func HandoverHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if err := requireOwnerOrAdmin(e, device); err != nil {
			return err
		}

		if err := e.Request.ParseForm(); err != nil {
			return e.BadRequestError("invalid form data", err)
		}

		request, err := app.FindRecordById("lending_requests", e.Request.FormValue("request_id"))
		if err != nil {
			return e.NotFoundError("request not found", err)
		}

		var lendEnd types.DateTime
		if v := e.Request.FormValue("lend_end"); v != "" {
			lendEnd, err = types.ParseDateTime(v)
			if err != nil {
				return e.BadRequestError("invalid lend_end date", err)
			}
		}

		if err := devices.Handover(app, notifier, device, request, lendEnd); err != nil {
			return e.BadRequestError("failed to hand over device", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}

func MarkReturnedHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if err := requireOwnerOrAdmin(e, device); err != nil {
			return err
		}

		if err := devices.MarkReturned(app, notifier, device); err != nil {
			return e.BadRequestError("failed to mark device returned", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}

func RejectRequestHandler(app core.App, notifier *mail.Notifier) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		request, err := app.FindRecordById("lending_requests", e.Request.PathValue("reqId"))
		if err != nil {
			return e.NotFoundError("request not found", err)
		}
		device, err := app.FindRecordById("devices", request.GetString("device"))
		if err != nil {
			return e.InternalServerError("failed to load device", err)
		}
		if err := requireOwnerOrAdmin(e, device); err != nil {
			return err
		}

		if err := devices.Reject(app, notifier, request); err != nil {
			return e.BadRequestError("failed to reject request", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}

func ToggleUnavailableHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if err := requireOwnerOrAdmin(e, device); err != nil {
			return err
		}

		switch device.GetString("status") {
		case "unavailable":
			device.Set("status", "available")
		case "available":
			device.Set("status", "unavailable")
		default:
			return e.BadRequestError("device must be available or unavailable to toggle", nil)
		}

		if err := app.Save(device); err != nil {
			return e.InternalServerError("failed to update device", err)
		}

		return e.Redirect(http.StatusFound, "/devices/"+device.Id)
	}
}

func DeleteDeviceHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if err := requireOwnerOrAdmin(e, device); err != nil {
			return err
		}
		if device.GetString("status") == "lent" {
			return e.BadRequestError("cannot delete a device that is currently lent", nil)
		}

		if err := app.Delete(device); err != nil {
			return e.InternalServerError("failed to delete device", err)
		}

		return e.Redirect(http.StatusFound, "/my/devices")
	}
}
```

- [ ] **Step 4: Run tests to verify they pass**

Run: `go test ./internal/web/... -run "TestHandoverHandler|TestDeleteDeviceHandler|TestToggleUnavailableHandler" -v`
Expected: PASS

- [ ] **Step 5: Commit**

```bash
git add device-lending/internal/web/owner_actions.go device-lending/internal/web/owner_actions_test.go
git commit -m "Add owner action routes: handover, return, reject, toggle, delete"
```

---

## Task 28: My devices / My requests pages

**Files:**
- Create: `device-lending/internal/web/my_pages.go`
- Create: `device-lending/internal/web/templates/my_devices.html`
- Create: `device-lending/internal/web/templates/my_requests.html`
- Test: `device-lending/internal/web/my_pages_test.go`

**Interfaces:**
- Produces: `func MyDevicesHandler(app core.App) func(e *core.RequestEvent) error`, `func MyRequestsHandler(app core.App) func(e *core.RequestEvent) error`. Registered at `GET /my/devices`, `GET /my/requests` in Task 30.

- [ ] **Step 1: Write the failing tests**

```go
// internal/web/my_pages_test.go
package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/web"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestMyDevicesHandler_ListsOwnDevices(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	other := newTestUserFor(t, app, "other@example.com")
	newTestDeviceForBrowse(t, app, owner, "My Drill")
	newTestDeviceForBrowse(t, app, other, "Someone Else's Saw")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = owner
			return re.Next()
		})
		e.Router.GET("/my/devices", web.MyDevicesHandler(app))
	})

	req := httptest.NewRequest(http.MethodGet, "/my/devices", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "My Drill") {
		t.Error("expected page to list owner's own device")
	}
	if strings.Contains(rec.Body.String(), "Someone Else's Saw") {
		t.Error("expected page to NOT list another owner's device")
	}
}

func TestMyRequestsHandler_ListsOwnRequests(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	owner := newTestUserFor(t, app, "owner@example.com")
	requester := newTestUserFor(t, app, "requester@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")

	requestsCol, err := app.FindCollectionByNameOrId("lending_requests")
	if err != nil {
		t.Fatal(err)
	}
	request := core.NewRecord(requestsCol)
	request.Set("device", device.Id)
	request.Set("requester", requester.Id)
	request.Set("status", "pending")
	request.Set("requested_start", "2026-08-01 00:00:00.000Z")
	if err := app.Save(request); err != nil {
		t.Fatal(err)
	}

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = requester
			return re.Next()
		})
		e.Router.GET("/my/requests", web.MyRequestsHandler(app))
	})

	req := httptest.NewRequest(http.MethodGet, "/my/requests", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cordless Drill") {
		t.Error("expected page to list the requested device's name")
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/... -run "TestMyDevicesHandler|TestMyRequestsHandler" -v`
Expected: FAIL with "undefined: web.MyDevicesHandler"

- [ ] **Step 3: Write templates**

```html
<!-- internal/web/templates/my_devices.html -->
{{define "content"}}
<h1>My devices</h1>
<p><a href="/devices/new">+ Add a device</a></p>
<ul>
{{range .Devices}}
<li><a href="/devices/{{.ID}}">{{.Name}}</a> — {{.Status}}{{if .PendingCount}} ({{.PendingCount}} pending request(s)){{end}}</li>
{{else}}
<li>You haven't listed any devices yet.</li>
{{end}}
</ul>
{{end}}
```

```html
<!-- internal/web/templates/my_requests.html -->
{{define "content"}}
<h1>My requests</h1>
<ul>
{{range .Requests}}
<li><a href="/devices/{{.DeviceID}}">{{.DeviceName}}</a> — {{.Status}}</li>
{{else}}
<li>You haven't requested any devices yet.</li>
{{end}}
</ul>
{{end}}
```

- [ ] **Step 4: Implement my_pages.go**

```go
// internal/web/my_pages.go
package web

import (
	"net/http"

	"github.com/pocketbase/dbx"
	"github.com/pocketbase/pocketbase/core"
)

type myDeviceItem struct {
	ID           string
	Name         string
	Status       string
	PendingCount int
}

func MyDevicesHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}

		records, err := app.FindRecordsByFilter("devices", "owner = {:owner}", "name", 0, 0, dbx.Params{"owner": e.Auth.Id})
		if err != nil {
			return e.InternalServerError("failed to load devices", err)
		}

		items := make([]myDeviceItem, 0, len(records))
		for _, r := range records {
			pending, err := app.FindRecordsByFilter(
				"lending_requests",
				"device = {:device} && status = 'pending'",
				"", 0, 0,
				dbx.Params{"device": r.Id},
			)
			if err != nil {
				return e.InternalServerError("failed to load pending requests", err)
			}
			items = append(items, myDeviceItem{ID: r.Id, Name: r.GetString("name"), Status: r.GetString("status"), PendingCount: len(pending)})
		}

		html, err := Render([]string{"templates/my_devices.html"}, map[string]any{
			"Title":       "My devices",
			"CurrentUser": e.Auth,
			"IsAdmin":     e.Auth.GetBool("is_admin"),
			"Devices":     items,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}
		return e.HTML(http.StatusOK, html)
	}
}

type myRequestItem struct {
	ID         string
	DeviceID   string
	DeviceName string
	Status     string
}

func MyRequestsHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if e.Auth == nil {
			return e.Redirect(http.StatusFound, "/oidc/login")
		}

		records, err := app.FindRecordsByFilter("lending_requests", "requester = {:r}", "-created", 0, 0, dbx.Params{"r": e.Auth.Id})
		if err != nil {
			return e.InternalServerError("failed to load requests", err)
		}

		items := make([]myRequestItem, 0, len(records))
		for _, r := range records {
			device, err := app.FindRecordById("devices", r.GetString("device"))
			if err != nil {
				return e.InternalServerError("failed to load device", err)
			}
			items = append(items, myRequestItem{ID: r.Id, DeviceID: device.Id, DeviceName: device.GetString("name"), Status: r.GetString("status")})
		}

		html, err := Render([]string{"templates/my_requests.html"}, map[string]any{
			"Title":       "My requests",
			"CurrentUser": e.Auth,
			"IsAdmin":     e.Auth.GetBool("is_admin"),
			"Requests":    items,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}
		return e.HTML(http.StatusOK, html)
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/web/... -run "TestMyDevicesHandler|TestMyRequestsHandler" -v`
Expected: PASS

- [ ] **Step 6: Commit**

```bash
git add device-lending/internal/web/my_pages.go device-lending/internal/web/templates/my_devices.html device-lending/internal/web/templates/my_requests.html device-lending/internal/web/my_pages_test.go
git commit -m "Add My devices and My requests pages"
```

---

## Task 29: Admin cleanup view

**Files:**
- Create: `device-lending/internal/web/admin.go`
- Create: `device-lending/internal/web/templates/admin.html`
- Test: `device-lending/internal/web/admin_test.go`

**Interfaces:**
- Produces:
  ```go
  func AdminHandler(app core.App) func(e *core.RequestEvent) error
  func AdminDeleteDeviceHandler(app core.App) func(e *core.RequestEvent) error
  func AdminDeleteRequestHandler(app core.App) func(e *core.RequestEvent) error
  ```
  Registered at `GET /admin`, `POST /admin/devices/{id}/delete`, `POST /admin/requests/{id}/delete` in Task 30. `AdminDeleteDeviceHandler` still blocks deletion while `status == "lent"` (per the design doc, admins bypass the request-history retention rule but not the "never delete a currently-lent device" rule); `AdminDeleteRequestHandler` has no such restriction, matching the design doc's explicit admin bypass for `lending_requests`.

- [ ] **Step 1: Write the failing tests**

```go
// internal/web/admin_test.go
package web_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	"github.com/the-vas/device-lending/internal/web"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func TestAdminHandler_ForbidsNonAdmin(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	regular := newTestUserFor(t, app, "regular@example.com")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = regular
			return re.Next()
		})
		e.Router.GET("/admin", web.AdminHandler(app))
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusForbidden {
		t.Fatalf("expected 403, got %d: %s", rec.Code, rec.Body.String())
	}
}

func TestAdminHandler_ListsAllDevices(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	admin := newTestUserFor(t, app, "admin@example.com")
	admin.Set("is_admin", true)
	if err := app.Save(admin); err != nil {
		t.Fatal(err)
	}
	owner := newTestUserFor(t, app, "owner@example.com")
	newTestDeviceForBrowse(t, app, owner, "Cordless Drill")

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = admin
			return re.Next()
		})
		e.Router.GET("/admin", web.AdminHandler(app))
	})

	req := httptest.NewRequest(http.MethodGet, "/admin", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", rec.Code, rec.Body.String())
	}
	if !strings.Contains(rec.Body.String(), "Cordless Drill") {
		t.Error("expected admin page to list all devices, including ones the admin doesn't own")
	}
}

func TestAdminDeleteDeviceHandler_BlocksWhileLent(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	admin := newTestUserFor(t, app, "admin@example.com")
	admin.Set("is_admin", true)
	if err := app.Save(admin); err != nil {
		t.Fatal(err)
	}
	owner := newTestUserFor(t, app, "owner@example.com")
	device := newTestDeviceForBrowse(t, app, owner, "Cordless Drill")
	device.Set("status", "lent")
	if err := app.Save(device); err != nil {
		t.Fatal(err)
	}

	mux := buildMux(t, app, func(e *core.ServeEvent) {
		e.Router.BindFunc(func(re *core.RequestEvent) error {
			re.Auth = admin
			return re.Next()
		})
		e.Router.POST("/admin/devices/{id}/delete", web.AdminDeleteDeviceHandler(app))
	})

	req := httptest.NewRequest(http.MethodPost, "/admin/devices/"+device.Id+"/delete", nil)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", rec.Code, rec.Body.String())
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

Run: `go test ./internal/web/... -run TestAdmin -v`
Expected: FAIL with "undefined: web.AdminHandler"

- [ ] **Step 3: Write admin.html**

```html
<!-- internal/web/templates/admin.html -->
{{define "content"}}
<h1>Admin cleanup</h1>
<h2>Devices</h2>
<ul>
{{range .Devices}}
<li>
  <a href="/devices/{{.Id}}">{{.GetString "name"}}</a> — {{.GetString "status"}}
  <form method="post" action="/admin/devices/{{.Id}}/delete" class="inline"><button type="submit">Delete</button></form>
</li>
{{else}}
<li>No devices.</li>
{{end}}
</ul>
<h2>Lending requests</h2>
<ul>
{{range .Requests}}
<li>
  {{.Id}} — {{.GetString "status"}}
  <form method="post" action="/admin/requests/{{.Id}}/delete" class="inline"><button type="submit">Delete</button></form>
</li>
{{else}}
<li>No requests.</li>
{{end}}
</ul>
{{end}}
```

- [ ] **Step 4: Implement admin.go**

```go
// internal/web/admin.go
package web

import (
	"net/http"

	"github.com/pocketbase/pocketbase/core"
)

func requireAdmin(e *core.RequestEvent) error {
	if e.Auth == nil {
		return e.Redirect(http.StatusFound, "/oidc/login")
	}
	if !e.Auth.GetBool("is_admin") {
		return e.ForbiddenError("admin only", nil)
	}
	return nil
}

func AdminHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if err := requireAdmin(e); err != nil {
			return err
		}

		devicesRecords, err := app.FindRecordsByFilter("devices", "", "name", 0, 0)
		if err != nil {
			return e.InternalServerError("failed to load devices", err)
		}
		requestsRecords, err := app.FindRecordsByFilter("lending_requests", "", "-created", 0, 0)
		if err != nil {
			return e.InternalServerError("failed to load requests", err)
		}

		html, err := Render([]string{"templates/admin.html"}, map[string]any{
			"Title":       "Admin",
			"CurrentUser": e.Auth,
			"IsAdmin":     true,
			"Devices":     devicesRecords,
			"Requests":    requestsRecords,
		})
		if err != nil {
			return e.InternalServerError("failed to render page", err)
		}
		return e.HTML(http.StatusOK, html)
	}
}

func AdminDeleteDeviceHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if err := requireAdmin(e); err != nil {
			return err
		}

		device, err := app.FindRecordById("devices", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("device not found", err)
		}
		if device.GetString("status") == "lent" {
			return e.BadRequestError("cannot delete a device that is currently lent", nil)
		}

		if err := app.Delete(device); err != nil {
			return e.InternalServerError("failed to delete device", err)
		}
		return e.Redirect(http.StatusFound, "/admin")
	}
}

func AdminDeleteRequestHandler(app core.App) func(e *core.RequestEvent) error {
	return func(e *core.RequestEvent) error {
		if err := requireAdmin(e); err != nil {
			return err
		}

		request, err := app.FindRecordById("lending_requests", e.Request.PathValue("id"))
		if err != nil {
			return e.NotFoundError("request not found", err)
		}

		if err := app.Delete(request); err != nil {
			return e.InternalServerError("failed to delete request", err)
		}
		return e.Redirect(http.StatusFound, "/admin")
	}
}
```

- [ ] **Step 5: Run tests to verify they pass**

Run: `go test ./internal/web/... -run TestAdmin -v`
Expected: PASS

- [ ] **Step 6: Run the full web package test suite, then commit**

```bash
go test ./internal/web/... -v
git add device-lending/internal/web/admin.go device-lending/internal/web/templates/admin.html device-lending/internal/web/admin_test.go
git commit -m "Add admin cleanup view for devices and requests"
```

---

## Task 30: Final route wiring, Docker deployment, and README

Consolidates every route from Tasks 1–29 into `main.go`, then adds the Docker/Compose deployment artifacts and setup documentation.

**Files:**
- Modify: `device-lending/main.go`
- Create: `device-lending/Dockerfile`
- Create: `device-lending/docker-compose.yml`
- Create: `device-lending/.env.example`
- Create: `device-lending/README.md`
- Create: `device-lending/.dockerignore`

**Interfaces:**
- Consumes: every handler/hook produced by Tasks 1–29.

- [ ] **Step 1: Rewrite main.go with the full route table**

```go
// main.go
package main

import (
	"io/fs"
	"log"
	"os"

	"github.com/pocketbase/pocketbase"
	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/plugins/migratecmd"

	"github.com/the-vas/device-lending/internal/authsetup"
	"github.com/the-vas/device-lending/internal/config"
	"github.com/the-vas/device-lending/internal/devices"
	"github.com/the-vas/device-lending/internal/mail"
	"github.com/the-vas/device-lending/internal/oidcdiscovery"
	"github.com/the-vas/device-lending/internal/web"
	"github.com/the-vas/device-lending/internal/webauth"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func main() {
	cfg, err := config.Load(os.Getenv)
	if err != nil {
		log.Fatalf("configuration error: %v", err)
	}

	app := pocketbase.New()

	migratecmd.MustRegister(app, app.RootCmd, migratecmd.Config{
		Automigrate: true,
	})

	authsetup.RegisterOIDCScopes("groups")

	app.OnBootstrap().BindFunc(func(e *core.BootstrapEvent) error {
		if err := e.Next(); err != nil {
			return err
		}
		if err := authsetup.ConfigureOAuth2(e.App, cfg, oidcdiscovery.Fetch); err != nil {
			return err
		}
		authsetup.BindAdminSync(e.App, cfg.OIDCAdminGroup)
		return authsetup.ApplyReadRules(e.App, cfg.PublicRead)
	})

	devices.BindStateFieldGuard(app)

	notifier := mail.New(app, cfg.BaseURL)
	devices.BindDeleteCascade(app, notifier)

	staticSub, err := fs.Sub(web.StaticFS, "static")
	if err != nil {
		log.Fatal(err)
	}

	app.OnServe().BindFunc(func(se *core.ServeEvent) error {
		signer := webauth.NewSigner(cfg.SessionSecret)

		se.Router.BindFunc(webauth.LoadSession(signer))

		se.Router.GET("/healthz", func(e *core.RequestEvent) error {
			return e.String(200, "ok")
		})
		se.Router.GET("/static/{path...}", apis.Static(staticSub, false))

		se.Router.GET("/oidc/login", webauth.LoginHandler(cfg.BaseURL, signer))
		se.Router.GET("/oidc/callback", webauth.CallbackHandler(cfg.BaseURL, signer, webauth.RouterExchanger))
		se.Router.POST("/logout", webauth.LogoutHandler())

		se.Router.GET("/", web.BrowseHandler(app, cfg.PublicRead))
		se.Router.GET("/devices/new", web.NewDeviceFormHandler(app))
		se.Router.POST("/devices/new", web.CreateDeviceHandler(app))
		se.Router.GET("/devices/{id}", web.DeviceDetailHandler(app, cfg.PublicRead))
		se.Router.GET("/devices/{id}/edit", web.EditDeviceFormHandler(app))
		se.Router.POST("/devices/{id}/edit", web.UpdateDeviceHandler(app))
		se.Router.POST("/devices/{id}/delete", web.DeleteDeviceHandler(app))
		se.Router.POST("/devices/{id}/request", web.RequestDeviceHandler(app, notifier))
		se.Router.POST("/devices/{id}/requests/{reqId}/withdraw", web.WithdrawRequestHandler(app, notifier))
		se.Router.POST("/devices/{id}/requests/{reqId}/reject", web.RejectRequestHandler(app, notifier))
		se.Router.POST("/devices/{id}/handover", web.HandoverHandler(app, notifier))
		se.Router.POST("/devices/{id}/return", web.MarkReturnedHandler(app, notifier))
		se.Router.POST("/devices/{id}/toggle-unavailable", web.ToggleUnavailableHandler(app))

		se.Router.GET("/my/devices", web.MyDevicesHandler(app))
		se.Router.GET("/my/requests", web.MyRequestsHandler(app))

		se.Router.GET("/admin", web.AdminHandler(app))
		se.Router.POST("/admin/devices/{id}/delete", web.AdminDeleteDeviceHandler(app))
		se.Router.POST("/admin/requests/{id}/delete", web.AdminDeleteRequestHandler(app))

		return se.Next()
	})

	if err := app.Start(); err != nil {
		log.Fatal(err)
	}
}
```

- [ ] **Step 2: Run the full build and test suite**

Run: `go build ./... && go test ./... -v`
Expected: PASS across every package from Tasks 1–29.

- [ ] **Step 3: Write the Dockerfile**

```dockerfile
# Dockerfile
FROM golang:1.23-alpine AS build
WORKDIR /src
COPY go.mod go.sum ./
RUN go mod download
COPY . .
RUN CGO_ENABLED=0 go build -o /out/device-lending .

FROM alpine:3.20
RUN apk add --no-cache ca-certificates
WORKDIR /app
COPY --from=build /out/device-lending ./device-lending
VOLUME ["/app/pb_data"]
EXPOSE 8090
ENTRYPOINT ["./device-lending", "serve", "--http=0.0.0.0:8090", "--dir=/app/pb_data"]
```

- [ ] **Step 4: Write .dockerignore**

```
# .dockerignore
pb_data/
*.db
.git/
```

- [ ] **Step 5: Write docker-compose.yml**

```yaml
# docker-compose.yml
services:
  device-lending:
    build: .
    ports:
      - "8090:8090"
    volumes:
      - pb_data:/app/pb_data
    environment:
      OIDC_ISSUER: ${OIDC_ISSUER}
      OIDC_CLIENT_ID: ${OIDC_CLIENT_ID}
      OIDC_CLIENT_SECRET: ${OIDC_CLIENT_SECRET}
      OIDC_ADMIN_GROUP: ${OIDC_ADMIN_GROUP:-}
      BASE_URL: ${BASE_URL}
      SESSION_SECRET: ${SESSION_SECRET}
      PUBLIC_READ: ${PUBLIC_READ:-false}
      HTTP_ADDR: 0.0.0.0:8090
    restart: unless-stopped

volumes:
  pb_data:
```

- [ ] **Step 6: Write .env.example**

```
# .env.example — copy to .env and fill in real values
OIDC_ISSUER=https://auth.example.com/application/o/device-lending/
OIDC_CLIENT_ID=changeme
OIDC_CLIENT_SECRET=changeme
OIDC_ADMIN_GROUP=admin
BASE_URL=https://lending.example.com
SESSION_SECRET=change-me-to-a-random-32-plus-character-secret
PUBLIC_READ=false
```

- [ ] **Step 7: Write README.md**

```markdown
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
| `BASE_URL` | yes | Public URL this app is reachable at, no trailing slash |
| `SESSION_SECRET` | yes | Random string, 32+ characters, used to sign session cookies |
| `PUBLIC_READ` | no (default `false`) | `true` to let anyone browse/view devices without logging in; requesting a device always requires login regardless |

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

## Becoming an admin

Log into the app once via `${BASE_URL}/oidc/login` so your user record
exists, then make sure your account is a member of the `OIDC_ADMIN_GROUP`
group in your OIDC provider. Admin status syncs on every login.
```

- [ ] **Step 8: Build the Docker image and smoke-test it**

```bash
cd device-lending
docker build -t device-lending .
docker run --rm -p 8091:8090 \
  -e OIDC_ISSUER=https://auth.example.com \
  -e OIDC_CLIENT_ID=test \
  -e OIDC_CLIENT_SECRET=test \
  -e BASE_URL=http://localhost:8091 \
  -e SESSION_SECRET=local-smoke-test-secret-32-chars-min \
  device-lending &
sleep 2
curl -sf http://localhost:8091/healthz
docker stop $(docker ps -q --filter ancestor=device-lending)
```

Expected: `curl` prints `ok`. (This step requires the real `OIDC_ISSUER` to resolve its discovery document at boot per Task 8 — if using a placeholder issuer that isn't reachable, boot will fail; substitute a real or locally-mocked issuer, or skip this step in an offline environment and rely on Step 2's test suite instead.)

- [ ] **Step 9: Commit**

```bash
git add device-lending/main.go device-lending/Dockerfile device-lending/docker-compose.yml device-lending/.env.example device-lending/README.md device-lending/.dockerignore
git commit -m "Wire full route table and add Docker deployment artifacts"
```
