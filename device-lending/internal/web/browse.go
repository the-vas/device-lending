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
