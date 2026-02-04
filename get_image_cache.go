package pluto

import (
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
)

func getImageCache(gc *gin.Context) {
	ctx := gc.Request.Context()
	dbPool := PlutoInstance.DbPool
	dbSchema := PlutoInstance.DbSchema

	imageId, ok := ParamInt(gc, "imageId")
	if !ok {
		gc.JSON(http.StatusInternalServerError, gin.H{"error": "imageId is required"})
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
		gc.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}
	defer rows.Close()

	var cacheEntries []CacheEntry
	for rows.Next() {
		var entry CacheEntry
		err := rows.Scan(&entry.Id, &entry.Receipt, &entry.ImageId, &entry.CreatedAt, &entry.MimeType)
		if err != nil {
			gc.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
			return
		}
		cacheEntries = append(cacheEntries, entry)
	}

	gc.JSON(http.StatusOK, cacheEntries)
}
