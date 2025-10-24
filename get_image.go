package pluto

import (
	"bytes"
	"context"
	"fmt"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/chai2010/webp"
	"github.com/gin-gonic/gin"
	"github.com/lib/pq"
)

func getImageHandler(gc *gin.Context) {
	params := gc.Request.URL.Query()
	var paramData [11]Param // Same order as paramKeys

	idStr := gc.Param("id")
	id, err := strconv.Atoi(idStr)
	if err != nil {
		gc.JSON(http.StatusBadRequest, gin.H{"error": fmt.Sprintf("invalid image id: %v", err)})
		return
	}

	for i, key := range paramKeys {
		if values, exists := params[key]; exists && len(values) > 0 {
			paramData[i] = Param{
				Exist: true,
				Value: values[0],
			}
		}
	}

	Singleton.Log("getImageHandler 1")

	modeStr := getOrDefault(paramData[paramIndex["modeStr"]], "center")
	typeStr := getOrDefault(paramData[paramIndex["type"]], "")
	quality := atoiOrDefaultClamped(paramData[paramIndex["quality"]], 85, 0, 100)
	width := atoiOrDefaultClamped(paramData[paramIndex["width"]], 0, 0, 4096)
	height := atoiOrDefaultClamped(paramData[paramIndex["height"]], 0, 0, 4096)
	ratioStr := getOrDefault(paramData[paramIndex["ratio"]], "")
	focusX := atoiOrDefaultClamped(paramData[paramIndex["focusx"]], 0, -100, 100)
	focusY := atoiOrDefaultClamped(paramData[paramIndex["focusy"]], 0, -100, 100)
	_, lossless := gc.GetQuery("lossless")

	Singleton.Log(modeStr)

	var aspectRatio float64
	if !(width > 0 && height > 0) && ratioStr != "" {
		aspectRatio, err = ParseAspectRatio(ratioStr)
		if err != nil || aspectRatio <= 0 {
			gc.String(http.StatusBadRequest, "Invalid ratio format. Use format like '3by2'")
			return
		}
	}

	var paramCode, paramValues string
	for i, key := range paramKeys {
		if paramData[i].Exist {
			paramCode += paramShortCodes[key]
			switch i {
			case paramIndex["modeStr"]:
				if modeStr == "cover" {
					paramValues += "01"
				} else {
					paramValues += "00"
				}
			case paramIndex["type"]:
				switch typeStr {
				case "png":
					paramValues += "01"
				case "webp":
					paramValues += "02"
				default:
					paramValues += "00"
				}
			case paramIndex["quality"]:
				paramValues += fmt.Sprintf("%02x", quality)
			case paramIndex["width"]:
				paramValues += fmt.Sprintf("%04x", width)
			case paramIndex["height"]:
				paramValues += fmt.Sprintf("%04x", height)
			case paramIndex["ratio"]:
				parts := strings.Split(ratioStr, "by")
				if len(parts) == 2 {
					num1, err1 := strconv.Atoi(parts[0])
					num2, err2 := strconv.Atoi(parts[1])
					if err1 == nil && err2 == nil {
						paramValues += fmt.Sprintf("%04x%04x", num1, num2)
					}
				}
			case paramIndex["focusx"]:
				paramValues += fmt.Sprintf("%04x", focusX)
			case paramIndex["focusy"]:
				paramValues += fmt.Sprintf("%04x", focusY)
			case paramIndex["brightness"], paramIndex["contrast"], paramIndex["saturation"]:
				paramValues += fmt.Sprintf("%03x", 1000)
			}
		}
	}

	imageReceipt := fmt.Sprintf("%x_%s_%s", id, paramCode, paramValues)
	cacheFileName := imageReceipt + "." + typeStr
	cacheFilePath := filepath.Join(Singleton.Config.PlutoCacheDir, cacheFileName)

	Singleton.Log(imageReceipt)

	if _, err := os.Stat(cacheFilePath); err == nil {
		gc.Header("Content-Disposition", `inline; filename="`+cacheFileName+`"`)
		gc.File(cacheFilePath)
		return
	}

	Singleton.Log("Render")

	var fileName, genFileName, mimeType string
	query := fmt.Sprintf(`SELECT file_name, gen_file_name, mime_type FROM %s.pluto_image WHERE id = $1`, pq.QuoteIdentifier(Singleton.Config.DbSchema))
	err = Singleton.Db.QueryRow(context.Background(), query, id).Scan(&fileName, &genFileName, &mimeType)
	if err != nil {
		gc.String(http.StatusBadRequest, "Image not found")
		return
	}

	if typeStr == "" {
		typeStr = mimeType
	}

	imgPath := filepath.Join(Singleton.Config.PlutoImageDir, genFileName)
	Singleton.Log("imgPath")
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

	if width > 0 || height > 0 || aspectRatio > 0 {
		img = CropToAspectAdvanced(img, modeStr, aspectRatio, float64(focusX)/10000, float64(focusY)/10000, width, height)
	}

	var buf bytes.Buffer
	switch typeStr {
	case "jpeg", "jpg":
		typeStr = "jpg"
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	case "png":
		err = png.Encode(&buf, img)
	case "webp":
		options := &webp.Options{Lossless: lossless}
		if !lossless {
			options.Quality = float32(quality)
		}
		err = webp.Encode(&buf, img, options)
	default:
		gc.String(http.StatusUnsupportedMediaType, "Unsupported image format")
		return
	}
	if err != nil {
		gc.String(http.StatusInternalServerError, "Failed to encode image")
		return
	}

	// Save to cache
	err = os.WriteFile(cacheFilePath, buf.Bytes(), 0644)
	if err == nil {
		query = fmt.Sprintf(`
			INSERT INTO %s.pluto_cache (receipt, image_id, mime_type)
			VALUES ($1, $2, $3)`,
			pq.QuoteIdentifier(Singleton.Config.DbSchema))
		_, _ = Singleton.Db.Exec(context.Background(), query, imageReceipt, id, typeStr)
	}

	gc.Header("Content-Type", "image/"+typeStr)
	gc.Header("Content-Disposition", `inline; filename="`+cacheFileName+`"`)
	gc.Data(http.StatusOK, "image/"+typeStr, buf.Bytes())
}
