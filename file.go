package pluto

import (
	"fmt"
	"net/http"
	"os"
	"time"

	"github.com/gin-gonic/gin"
)

// serveCacheFile serves the cached file and updates its access time.
func serveCacheFile(gc *gin.Context, cacheFilePath, cacheFileName string) {
	info, err := os.Stat(cacheFilePath)
	if err != nil {
		// File does not exist, handle accordingly (404 or generate)
		gc.Status(http.StatusNotFound)
		return
	}

	etag := fmt.Sprintf(`"%x-%x"`, info.ModTime().Unix(), info.Size())

	// Handle conditional GET
	if match := gc.GetHeader("If-None-Match"); match == etag {
		gc.Status(http.StatusNotModified)
		return
	}

	gc.Header("ETag", etag)
	gc.Header("Cache-Control", "no-cache")
	gc.Header("Content-Disposition", `inline; filename="`+cacheFileName+`"`)

	// Safely touch the file
	touchFile(cacheFilePath, info.ModTime())

	// Serve the file
	gc.File(cacheFilePath)
}

// touchFile updates the access time of the given file safely.
// Logs a warning if it fails, but does not stop execution.
func touchFile(path string, mtime time.Time) {
	now := time.Now()
	if err := os.Chtimes(path, now, mtime); err != nil {
		fmt.Printf("Warning: failed to touch file %s: %v\n", path, err)
	}
}
