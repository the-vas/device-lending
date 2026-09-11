// internal/web/loginpath.go
package web

// LoginPath is the path unauthenticated visitors are redirected to, and what
// the layout template renders as the "Log in" link. Defaults to the real
// OIDC login route; main.go overrides it to "/dev/login" when DEV_AUTH is
// enabled. Set once at boot before the app starts serving — never mutated
// while requests are in flight, so no synchronization is needed.
var LoginPath = "/oidc/login"
