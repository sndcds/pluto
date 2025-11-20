package pluto

import (
	"fmt"
	"mime"
	"os"
	"path/filepath"
	"strings"

	"github.com/gin-gonic/gin"
)

func getFile(gc *gin.Context) {
	file := gc.Param("file") // e.g. "abc123.webp"

	// Normalize and join path
	cacheFilePath := filepath.Join(Singleton.Config.PlutoCacheDir, filepath.Clean(file))
	fmt.Println("Serving file:", file)
	fmt.Println("Resolved path:", cacheFilePath)

	// Security: Disallow path traversal attempts
	if strings.Contains(file, "..") || filepath.IsAbs(file) {
		gc.AbortWithStatusJSON(400, gin.H{"error": "Invalid file path"})
		return
	}

	// Check if file exists
	if stat, err := os.Stat(cacheFilePath); err != nil || stat.IsDir() {
		gc.AbortWithStatusJSON(404, gin.H{"error": "File not found"})
		return
	}

	// Optionally set proper content type
	ext := filepath.Ext(file)
	mime := mime.TypeByExtension(ext)
	if mime != "" {
		gc.Header("Content-Type", mime)
	}

	// Optional: force download (if needed)
	// gc.Header("Content-Disposition", fmt.Sprintf("inline; filename=\"%s\"", file))

	// Serve file
	gc.File(cacheFilePath)
}
