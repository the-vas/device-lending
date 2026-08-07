package migrations_test

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/pocketbase/pocketbase/apis"
	"github.com/pocketbase/pocketbase/core"
	"github.com/pocketbase/pocketbase/tests"

	_ "github.com/the-vas/device-lending/internal/migrations"
)

func usersAPIMux(t *testing.T, app *tests.TestApp) http.Handler {
	t.Helper()

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
	return mux
}

func newPlainUser(t *testing.T, app core.App, email string) *core.Record {
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

// TestUsersCollection_AnonymousSignupRefused is the regression guard for the
// unauthenticated privilege escalation: public self-signup on users would let
// anyone create an account with is_admin = true and then satisfy every
// "@request.auth.is_admin = true" rule in the app.
func TestUsersCollection_AnonymousSignupRefused(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	mux := usersAPIMux(t, app)

	body := `{"email":"attacker@example.com","password":"attacker-password-123","passwordConfirm":"attacker-password-123","is_admin":true}`
	req := httptest.NewRequest(http.MethodPost, "/api/collections/users/records", strings.NewReader(body))
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("anonymous signup succeeded: %s", rec.Body.String())
	}
	if rec.Code != http.StatusBadRequest && rec.Code != http.StatusForbidden {
		t.Fatalf("expected 400 or 403, got %d: %s", rec.Code, rec.Body.String())
	}

	if _, err := app.FindAuthRecordByEmail("users", "attacker@example.com"); err == nil {
		t.Fatal("expected no user record to have been created")
	}
}

// TestUsersCollection_SelfPromotionRefused covers the second half of the
// escalation: an existing (OIDC-created) user PATCHing their own record with
// is_admin = true, bypassing the OIDC groups-claim sync entirely.
func TestUsersCollection_SelfPromotionRefused(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	user := newPlainUser(t, app, "regular@example.com")
	token, err := user.NewAuthToken()
	if err != nil {
		t.Fatal(err)
	}

	mux := usersAPIMux(t, app)

	req := httptest.NewRequest(
		http.MethodPatch,
		"/api/collections/users/records/"+user.Id,
		strings.NewReader(`{"is_admin":true}`),
	)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", token)
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("self-promotion succeeded: %s", rec.Body.String())
	}

	reloaded, err := app.FindRecordById("users", user.Id)
	if err != nil {
		t.Fatal(err)
	}
	if reloaded.GetBool("is_admin") {
		t.Fatal("expected is_admin to still be false after the refused PATCH")
	}
}

// TestUsersCollection_PasswordAuthDisabled asserts the OIDC-only auth model:
// there is no local login path into the app.
func TestUsersCollection_PasswordAuthDisabled(t *testing.T) {
	app, err := tests.NewTestApp()
	if err != nil {
		t.Fatal(err)
	}
	defer app.Cleanup()

	users, err := app.FindCollectionByNameOrId("users")
	if err != nil {
		t.Fatal(err)
	}
	if users.PasswordAuth.Enabled {
		t.Error("expected password auth to be disabled on the users collection")
	}
	if users.OTP.Enabled {
		t.Error("expected OTP auth to be disabled on the users collection")
	}

	password := "local-login-password-123"
	user := core.NewRecord(users)
	user.SetEmail("local@example.com")
	user.Set("name", "local")
	user.SetPassword(password)
	if err := app.Save(user); err != nil {
		t.Fatal(err)
	}

	mux := usersAPIMux(t, app)

	req := httptest.NewRequest(
		http.MethodPost,
		"/api/collections/users/auth-with-password",
		strings.NewReader(`{"identity":"local@example.com","password":"`+password+`"}`),
	)
	req.Header.Set("Content-Type", "application/json")
	rec := httptest.NewRecorder()
	mux.ServeHTTP(rec, req)

	if rec.Code == http.StatusOK {
		t.Fatalf("password auth succeeded even though it is disabled: %s", rec.Body.String())
	}
}

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
