package pluto

import (
	"bytes"
	"database/sql"
	"errors"

	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"

	"github.com/chai2010/webp"
	"github.com/gin-gonic/gin"
)

func GetImageIdByByContext(
	gc *gin.Context,
	context string,
	contextId int,
	identifier string,
) (int, bool) {
	ctx := gc.Request.Context()

	query := fmt.Sprintf(
		`SELECT pluto_image_id
         FROM %s.pluto_image_link
         WHERE context = $1 AND context_id = $2 AND identifier = $3`,
		PlutoInstance.DbSchema,
	)

	var imageId *int
	err := PlutoInstance.DbPool.
		QueryRow(ctx, query, context, contextId, identifier).
		Scan(&imageId)

	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// no row found — valid state
			return -1, true
		}

		// Real DB error
		return -1, false
	}

	if imageId == nil {
		// Row exists but pluto_image_id is NULL
		return -1, true
	}

	return *imageId, true
}

func getImage(gc *gin.Context) {
	ctx := gc.Request.Context()
	pool := PlutoInstance.DbPool

	imageId, ok := ParamInt(gc, "id")
	if !ok {
		gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid image id"})
		return
	}

	fileTypeStr := gc.DefaultQuery("type", "jpg")
	if fileTypeStr != "jpg" && fileTypeStr != "png" && fileTypeStr != "webp" {
		gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid image type"})
		return
	}

	fitStr := gc.DefaultQuery("fit", "")
	if fitStr != "" && fitStr != "cover" {
		// TODO: Support mor  fit types like in CSS
		gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid fit mode"})
		return
	}

	quality, ok := GetQueryIntDefault(gc, "quality", 80)
	if !ok {
		gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid quality"})
		return
	}
	if quality < 0 {
		quality = 0
	} else if quality > 100 {
		quality = 100
	}

	width, ok := GetQueryIntDefault(gc, "width", 0)
	if !ok {
		gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid width"})
		return
	}

	height, ok := GetQueryIntDefault(gc, "height", 0)
	if !ok {
		gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid height"})
		return
	}

	ratioStr, hasRatio := gc.GetQuery("ratio")
	ratio := float32(0.0)
	if hasRatio {
		var err error
		ratio, err = ParseAspectRatio(ratioStr)
		if err != nil {
			gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid ratio"})
			return
		}
	}

	lossless, ok := GetQueryBoolDefault(gc, "lossless", false)

	knownEdges := 0
	if width > 0 {
		knownEdges++
	}
	if height > 0 {
		knownEdges++
	}

	if hasRatio && knownEdges == 1 {
		if ratio <= 0.0001 {
			gc.String(http.StatusBadRequest, "Invalid ratio format. Use format like '16:9'")
			return
		}
		if width > 0 {
			height = int(float32(width) / ratio)
		} else if height > 0 {
			width = int(float32(height) * ratio)
		} else {
			gc.String(http.StatusBadRequest, "Either width or height must be provided if using ratio")
			return
		}
	}

	var paramCode, paramValues string
	if fitStr != "" {
		paramCode += "f"
		switch fitStr {
		case "cover":
			paramValues += "01"
		default:
			paramValues += "00"
		}
	}
	if quality < 100 {
		paramCode += "q"
		paramValues += fmt.Sprintf("%02x", quality) // 0 - 99
	}
	if width > 0 {
		// TODO: Limit to configured max size
		paramCode += "w"
		paramValues += fmt.Sprintf("%04x", width) // max 65535 pixel
	}
	if height > 0 {
		// TODO: Limit to configured max size
		paramCode += "h"
		paramValues += fmt.Sprintf("%04x", height) // max 65535 pixel
	}

	if hasRatio {
		paramCode += "r"
		paramValues += "_" + EncodeFloat32ForPath(ratio)
	}

	imageReceipt := fmt.Sprintf("%x_%s_%s", imageId, paramCode, paramValues)
	cacheFileName := imageReceipt + "." + fileTypeStr
	cacheFilePath := filepath.Join(PlutoInstance.Config.PlutoCacheDir, cacheFileName)

	/* Previous version without using ETag in Header
	if _, err := os.Stat(cacheFilePath); err == nil {
		gc.Header("Content-Disposition", `inline; filename="`+cacheFileName+`"`)
		gc.File(cacheFilePath)
		return
	}
	*/

	if info, err := os.Stat(cacheFilePath); err == nil {
		etag := fmt.Sprintf(`"%x-%x"`, info.ModTime().Unix(), info.Size())

		if match := gc.GetHeader("If-None-Match"); match == etag {
			gc.Status(http.StatusNotModified)
			return
		}

		gc.Header("ETag", etag)
		gc.Header("Cache-Control", "no-cache")
		gc.Header("Content-Disposition", `inline; filename="`+cacheFileName+`"`)

		gc.File(cacheFilePath)
		return
	}

	var fileName, genFileName, mimeType string
	var focusX, focusY *float32
	sql := fmt.Sprintf(`
		SELECT file_name, gen_file_name, mime_type, focus_x, focus_y FROM %s.pluto_image WHERE id = $1`,
		PlutoInstance.DbSchema)
	err := pool.QueryRow(ctx, sql, imageId).Scan(&fileName, &genFileName, &mimeType, &focusX, &focusY)
	if err != nil {
		gc.String(http.StatusBadRequest, "Image not found")
		return
	}

	imgPath := filepath.Join(PlutoInstance.Config.PlutoImageDir, genFileName)
	fileBytes, err := os.ReadFile(imgPath)
	if err != nil {
		gc.String(http.StatusInternalServerError, "Failed to read image")
		return
	}

	img, _, err := image.Decode(bytes.NewReader(fileBytes))
	if err != nil {
		gc.String(http.StatusInternalServerError, "Invalid image format")
		return
	}

	if width > 0 || height > 0 || hasRatio {
		fx := float32(0.5)
		fy := float32(0.5)
		if focusX != nil {
			fx = *focusX
		}
		if focusY != nil {
			fy = *focusY
		}
		img = CropWithFocus(img, ratio, fx, fy, width, height)
	}

	var buf bytes.Buffer
	switch fileTypeStr {
	case "jpg":
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	case "png":
		err = png.Encode(&buf, img)
	case "webp":
		var options webp.Options
		if lossless {
			options = webp.Options{Lossless: true}
		} else {
			options = webp.Options{Quality: float32(quality), Lossless: false}
		}
		err = webp.Encode(&buf, img, &options)
	default:
		gc.JSON(http.StatusUnsupportedMediaType, gin.H{"error": fmt.Sprintf("Unsupported image format: image/%s", fileTypeStr)})
		return
	}
	if err != nil {
		gc.JSON(http.StatusInternalServerError, gin.H{"error": fmt.Sprintf("Failed to encode image: /%v", err.Error())})
		return
	}

	// Save to cache
	err = os.WriteFile(cacheFilePath, buf.Bytes(), 0644)
	if err == nil {
		sql = fmt.Sprintf(`
				INSERT INTO %s.pluto_cache (receipt, image_id, mime_type)
				VALUES ($1, $2, $3)`,
			PlutoInstance.DbSchema)
		_, _ = pool.Exec(ctx, sql, imageReceipt, imageId, fileTypeStr)
	}

	gc.Header("Content-Type", "image/"+fileTypeStr)
	gc.Header("Content-Disposition", `inline; filename="`+cacheFileName+`"`)
	gc.Data(http.StatusOK, "image/"+fileTypeStr, buf.Bytes())
}
