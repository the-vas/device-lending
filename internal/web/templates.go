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
