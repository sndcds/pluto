package pluto

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/sndcds/grains/grains_api"
)

func getImageCache(gc *gin.Context) {
	ctx := gc.Request.Context()
	dbPool := PlutoInstance.DbPool
	dbSchema := PlutoInstance.DbSchema
	apiRequest := grains_api.NewRequest(gc, "get-pluto-image-cache")

	imageId, ok := ParamInt(gc, "imageId")
	if !ok {
		apiRequest.Error(http.StatusBadRequest, "imageId is required")
		return
	}

	query := fmt.Sprintf(`
        SELECT id, receipt, image_id, created_at, mime_type
        FROM %s.pluto_cache
        WHERE image_id = $1
        ORDER BY created_at DESC
    `, dbSchema)

	rows, err := dbPool.Query(ctx, query, imageId)
	if err != nil {
		apiRequest.DatabaseError()
		return
	}
	defer rows.Close()

	var cacheEntries []CacheEntry
	for rows.Next() {
		var entry CacheEntry
		err := rows.Scan(&entry.Id, &entry.Receipt, &entry.ImageId, &entry.CreatedAt, &entry.MimeType)
		if err != nil {
			apiRequest.DatabaseError()
			return
		}
		cacheEntries = append(cacheEntries, entry)
	}

	apiRequest.Success(http.StatusOK, cacheEntries, "")
}
