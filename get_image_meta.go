package pluto

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"

	"github.com/sndcds/grains/grains_api"
)

// API: GET /image/:context/:contextId/:identifier/meta
func getImageMeta(gc *gin.Context) {
	apiRequest := grains_api.NewRequest(gc, "get-pluto-image-meta")
	ctx := gc.Request.Context()
	dbPool := PlutoInstance.DbPool
	dbSchema := PlutoInstance.DbSchema

	context := gc.Param("context")
	if context == "" {
		apiRequest.Error(http.StatusBadRequest, "context is required")
		return
	}

	contextUuid := gc.Param("contextUuid")
	if contextUuid == "" {
		apiRequest.Error(http.StatusBadRequest, "contextUuid is required")
		return
	}

	identifier := gc.Param("identifier")
	if identifier == "" {
		apiRequest.Error(http.StatusBadRequest, "identifier is required")
		return
	}

	query := fmt.Sprintf(`
        SELECT
            pi.uuid, 
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
            pi.focus_y
        FROM %s.pluto_image_link pil
        LEFT JOIN %s.pluto_image pi ON pi.uuid = pil.pluto_image_uuid
        WHERE pil.context = $1 AND pil.context_uuid = $2::uuid AND pil.identifier = $3
    `, dbSchema, dbSchema)

	var meta ImageMeta
	err := dbPool.QueryRow(ctx, query, context, contextUuid, identifier).Scan(
		&meta.Uuid,
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
	)

	if meta.Uuid == nil {
		apiRequest.Error(http.StatusNotFound, "image not found")
		return
	}

	if err != nil {
		if err == pgx.ErrNoRows {
			// No image found for this entity + index
			apiRequest.Error(http.StatusNotFound, "image not found")
			return
		}

		apiRequest.DatabaseError()
		return
	}

	apiRequest.Success(http.StatusOK, meta, "")
}
