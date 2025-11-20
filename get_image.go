package pluto

import (
	"bytes"

	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"

	"github.com/chai2010/webp"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

func getImage(gc *gin.Context) {
	ctx := gc.Request.Context()
	pool := Singleton.Db

	imageId, ok := ParamInt(gc, "id")
	if !ok {
		gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid image ID"})
		return
	}

	fileTypeStr := gc.DefaultQuery("type", "jpg")
	if fileTypeStr != "jpg" && fileTypeStr != "png" && fileTypeStr != "webp" {
		gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid image type"})
		return
	}
	fmt.Println("fileTypeStr:", fileTypeStr)

	fitStr := gc.DefaultQuery("fit", "")
	if fitStr != "" && fitStr != "cover" {
		// TODO: Support mor  fit types like in CSS
		gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid fit mode"})
		return
	}
	fmt.Println("fitStr:", fitStr)

	quality := QueryIntDefault(gc, "quality", 80)
	if quality < 0 {
		quality = 0
	} else if quality > 100 {
		quality = 100
	}
	fmt.Println("quality:", quality)

	width := QueryIntDefault(gc, "width", 0)
	height := QueryIntDefault(gc, "height", 0)

	ratioStr, ok := gc.GetQuery("ratio")
	ratio := 1.0
	hasRatio := false
	if ok {
		var err error
		ratio, err = ParseAspectRatio(ratioStr)
		if err != nil {
			gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid ratio"})
			return
		}
		hasRatio = true
	} else {
		ratio = 1.0
	}
	fmt.Sprintln("ratio:", ratio)

	knownEdges := 0
	if width > 0 {
		knownEdges++
	}
	if height > 0 {
		knownEdges++
	}
	fmt.Println("Known edges:", knownEdges)

	lossless := false // TODO: Implement

	if knownEdges == 2 {
		f := float64(0.0)
		if width > Singleton.Config.PlutoMaxImagePx {
			f = float64(width) / float64(Singleton.Config.PlutoMaxImagePx)
		}
		if height > Singleton.Config.PlutoMaxImagePx {
			f = float64(height) / float64(Singleton.Config.PlutoMaxImagePx)
		}
		fmt.Println("f: ", f)
	} else if hasRatio && knownEdges == 1 {
		if ratio <= 0.0001 {
			gc.String(http.StatusBadRequest, "Invalid ratio format. Use format like '16:9'")
			return
		}
		if width > 0 {
			height = int(float64(width) / ratio)
			fmt.Println("New height; ", height)
		} else if height > 0 {
			width = int(float64(height) / ratio)
			fmt.Println("New width: ", width)
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

	if ratioStr != "" {
		paramCode += "r"
		paramValues += "_" + ratioStr
	}

	imageReceipt := fmt.Sprintf("%x_%s_%s", imageId, paramCode, paramValues)
	fmt.Println("imageReceipt: ", imageReceipt)

	cacheFileName := imageReceipt + "." + fileTypeStr
	fmt.Println("cacheFileName: ", cacheFileName)

	cacheFilePath := filepath.Join(Singleton.Config.PlutoCacheDir, cacheFileName)
	fmt.Println("cacheFilePath: ", cacheFilePath)

	if _, err := os.Stat(cacheFilePath); err == nil {
		gc.Header("Content-Disposition", `inline; filename="`+cacheFileName+`"`)
		gc.File(cacheFilePath)
		fmt.Println("cacheFile used!")
		return
	}

	var fileName, genFileName, mimeType string
	var focusX, focusY *float64
	sql := fmt.Sprintf(`
		SELECT file_name, gen_file_name, mime_type, focus_x, focus_y FROM %s.pluto_image WHERE id = $1`,
		pq.QuoteIdentifier(Singleton.Config.DbSchema))
	fmt.Println(sql)
	err := pool.QueryRow(ctx, sql, imageId).Scan(&fileName, &genFileName, &mimeType, &focusX, &focusY)
	if err != nil {
		gc.String(http.StatusBadRequest, "Image not found")
		return
	}

	imgPath := filepath.Join(Singleton.Config.PlutoImageDir, genFileName)
	fmt.Println("read from imgPath: ", imgPath)
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
		fx := 0.5
		fy := 0.5
		if focusX != nil {
			fx = *focusX
		}
		if focusY != nil {
			fy = *focusX
		}
		img = CropToAspectAdvanced(img, fitStr, ratio, fx, fy, width, height)
	}

	var buf bytes.Buffer
	switch fileTypeStr {
	case "jpg":
		fileTypeStr = "jpg"
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	case "png":
		fileTypeStr = "png"
		err = png.Encode(&buf, img)
	case "webp":
		fileTypeStr = "webp"
		options := &webp.Options{Lossless: lossless}
		if !lossless {
			options.Quality = float32(quality)
		}
		err = webp.Encode(&buf, img, options)
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
			pq.QuoteIdentifier(Singleton.Config.DbSchema))
		_, _ = pool.Exec(ctx, sql, imageReceipt, imageId, fileTypeStr)
	}

	gc.Header("Content-Type", "image/"+fileTypeStr)
	gc.Header("Content-Disposition", `inline; filename="`+cacheFileName+`"`)
	gc.Data(http.StatusOK, "image/"+fileTypeStr, buf.Bytes())
}
