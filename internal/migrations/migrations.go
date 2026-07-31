// Package migrations registers this app's custom PocketBase collections.
// Files in this package call migrations.Register(up, down) in an init()
// function; main.go blank-imports this package so they run automatically.
package migrations
