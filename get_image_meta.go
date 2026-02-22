package pluto

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/sndcds/pluto/api"
)

// API: GET /image/:context/:contextId/:identifier/meta
func getImageMeta(gc *gin.Context) {
	ctx := gc.Request.Context()
	apiReponseType := "pluto-image-meta"
	dbPool := PlutoInstance.DbPool
	dbSchema := PlutoInstance.DbSchema

	context := gc.Param("context")
	if context == "" {
		api.JSONError(gc, apiReponseType, http.StatusBadRequest, "context is required")
		return
	}

	contextId, ok := ParamInt(gc, "contextId")
	if !ok {
		api.JSONError(gc, apiReponseType, http.StatusBadRequest, "contextId is required")
		return
	}

	identifier := gc.Param("identifier")
	if identifier == "" {
		api.JSONError(gc, apiReponseType, http.StatusBadRequest, "identifier is required")
		return
	}

	query := fmt.Sprintf(`
        SELECT
            pi.id, 
            pi.file_name, 
            pi.width, 
            pi.height, 
            pi.mime_type, 
            pi.alt_text, 
            pi.description,
            pi.license, 
            pi.exif, 
            pi.expiration_date, 
            pi.creator_name, 
            pi.copyright,
            pi.focus_x, 
            pi.focus_y, 
            pi.margin_left, 
            pi.margin_right, 
            pi.margin_top, 
            pi.margin_bottom
        FROM %s.pluto_image_link pil
        LEFT JOIN %s.pluto_image pi ON pi.id = pil.pluto_image_id
        WHERE pil.context = $1 AND pil.context_id = $2 AND pil.identifier = $3
    `, dbSchema, dbSchema)

	var meta ImageMeta
	err := dbPool.QueryRow(ctx, query, context, contextId, identifier).Scan(
		&meta.Id,
		&meta.FileName,
		&meta.Width,
		&meta.Height,
		&meta.MimeType,
		&meta.Alt,
		&meta.Description,
		&meta.License,
		&meta.Exif,
		&meta.Expiration,
		&meta.Creator,
		&meta.Copyright,
		&meta.FocusX,
		&meta.FocusY,
		&meta.MarginLeft,
		&meta.MarginRight,
		&meta.MarginTop,
		&meta.MarginBottom,
	)

	if meta.Id == nil {
		api.JSONError(gc, apiReponseType, http.StatusNotFound, "image not found")
		return
	}

	if err != nil {
		if err == pgx.ErrNoRows {
			// No image found for this entity + index
			api.JSONError(gc, apiReponseType, http.StatusNotFound, "mage not found")
			return
		}

		api.JSONDatabaseError(gc, apiReponseType)
		return
	}

	api.JSONSuccess(gc, apiReponseType, meta, nil)
}
