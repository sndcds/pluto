package pluto

import (
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/chai2010/webp"
	"github.com/gin-gonic/gin"
	pluto_image "github.com/sndcds/pluto/pluto-image"
	"image"
	"image/jpeg"
	"image/png"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

func serveImageById(c *gin.Context) {
	params := c.Request.URL.Query()
	var paramData [11]Param // Same order as paramKeys

	// Fill paramData
	for i, key := range paramKeys {
		if values, exists := params[key]; exists && len(values) > 0 {
			paramData[i] = Param{
				Exist: true,
				Value: values[0],
			}
		}
	}

	id, err := strconv.Atoi(c.Query("id"))
	if err != nil {
		c.String(400, "Invalid image ID")
		return
	}

	modeStr := getOrDefault(paramData[paramIndex["modeStr"]], "center")
	typeStr := getOrDefault(paramData[paramIndex["type"]], "")
	quality := atoiOrDefaultClamped(paramData[paramIndex["quality"]], 85, 0, 100)
	width := atoiOrDefaultClamped(paramData[paramIndex["width"]], 0, 0, 4096)
	height := atoiOrDefaultClamped(paramData[paramIndex["height"]], 0, 0, 4096)
	ratioStr := getOrDefault(paramData[paramIndex["ratio"]], "")
	focusX := atoiOrDefaultClamped(paramData[paramIndex["focusx"]], 5000, 0, 10000)
	focusY := atoiOrDefaultClamped(paramData[paramIndex["focusy"]], 5000, 0, 10000)

	_, lossless := c.GetQuery("lossless") // true if present, even with no value

	// Parse ratio only if needed
	var aspectRatio float64
	if !(width > 0 && height > 0) && ratioStr != "" {
		aspectRatio, err = ParseAspectRatio(ratioStr)
		if err != nil || aspectRatio <= 0 {
			c.String(400, "Invalid ratio format. Use format like '3by2'")
			return
		}
	}

	// Now build paramCode in fixed order
	var paramCode string
	var paramValues string

	for i, key := range paramKeys {
		if paramData[i].Exist {
			paramCode += paramShortCodes[key]
			switch i {
			case paramIndex["modeStr"]:
				switch modeStr {
				case "cover":
					paramValues += "01"
				default:
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
					} else {
						// handle error
					}
				} else {
					// handle invalid format
				}
			case paramIndex["focusx"]:
				paramValues += fmt.Sprintf("%04x", focusX)
			case paramIndex["focusy"]:
				paramValues += fmt.Sprintf("%04x", focusY)
			case paramIndex["brightness"]:
				paramValues += fmt.Sprintf("%03x", 1000)
			case paramIndex["contrast"]:
				paramValues += fmt.Sprintf("%03x", 1000)
			case paramIndex["saturation"]:
				paramValues += fmt.Sprintf("%03x", 1000)
			}
		}
	}

	imageReceipt := fmt.Sprintf("%4x_%s_%s", id, paramCode, paramValues)

	// fmt.Printf("paramData: %+v\n", paramData)
	// fmt.Println("paramCode:", paramCode)
	// fmt.Println("paramValues:", paramValues)
	fmt.Println("imageReceipt:", imageReceipt)

	// Check if cached version exists
	var cacheId int = -1
	var mimeType string
	err = Singleton.Db.QueryRow(context.Background(), `
				SELECT id, mime_type FROM uranus.pluto_cache WHERE receipt = $1
			`, imageReceipt).Scan(&cacheId, &mimeType)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			// c.String(404, "Cached image not found")
		} else {
			// c.String(500, "Database error: "+err.Error())
		}
	}

	fmt.Println("cacheId:", cacheId)
	fmt.Println("id:", id)

	if cacheId >= 0 {
		cachedImgPath := filepath.Join(Singleton.Config.PlutoCacheDir, imageReceipt) + "." + mimeType
		c.Header("Content-Disposition", `inline; filename="`+imageReceipt+`"`)
		c.File(cachedImgPath)
		return
	}

	// Fetch file metadata
	var fileName, genFileName string
	err = Singleton.Db.QueryRow(context.Background(), `
		SELECT file_name, gen_file_name, mime_type FROM uranus.pluto_image WHERE id = $1
	`, id).Scan(&fileName, &genFileName, &mimeType)
	if err != nil {
		c.String(404, "Image not found: %s", err.Error())
		return
	}

	if typeStr == "" {
		typeStr = mimeType
	}

	// Read and decode image
	imgPath := filepath.Join(Singleton.Config.PlutoImageDir, genFileName)
	fileBytes, err := os.ReadFile(imgPath)
	if err != nil {
		c.String(500, "Failed to read image: %s", imgPath)
		return
	}
	img, _, err := image.Decode(bytes.NewReader(fileBytes))
	if err != nil {
		c.String(500, "Invalid image format")
		return
	}

	// Apply cropping
	if width > 0 || height > 0 || aspectRatio > 0 {
		img = pluto_image.CropToAspectAdvanced(img, modeStr, aspectRatio, float64(focusX)/10000, float64(focusY)/10000, width, height)
	}

	var buf bytes.Buffer
	var ext string
	switch typeStr {
	case "jpeg", "jpg":
		ext = ".jpg"
		typeStr = "jpg"
		options := &jpeg.Options{Quality: quality}
		jpeg.Encode(&buf, img, options)
	case "png":
		ext = ".png"
		png.Encode(&buf, img)
	case "webp":
		ext = ".webp"
		var options *webp.Options
		if lossless {
			options = &webp.Options{Lossless: true}
		} else {
			// Use lossy with specified quality
			options = &webp.Options{
				Lossless: false,
				Quality:  float32(quality),
			}
		}
		err := webp.Encode(&buf, img, options)
		if err != nil {
			c.String(http.StatusInternalServerError, "WebP encode failed")
			return
		}
	default:
		c.String(415, "Unsupported image format")
		return
	}

	if cacheId < 0 {
		cachedImgPath := filepath.Join(Singleton.Config.PlutoImageDir, imageReceipt) + "." + typeStr

		// Save the generated image bytes to the cache path
		err := os.WriteFile(cachedImgPath, buf.Bytes(), 0644)
		if err != nil {
			// Log but don’t fail the response (optional)
			fmt.Printf("Failed to write cached image: %v\n", err)
		} else {
			// Insert new cache record in DB
			_, err = Singleton.Db.Exec(context.Background(), `
            INSERT INTO uranus.pluto_cache (receipt, image_id, mime_type)
            VALUES ($1, $2, $3)
        `, imageReceipt, id, typeStr)
			if err != nil {
				// Log insert error, but continue serving
				fmt.Printf("Failed to insert cache record: %v\n", err)
			}
		}
	}

	// Replace extension in fileName (if any) with correct one
	baseName := strings.TrimSuffix(fileName, filepath.Ext(fileName))
	finalFileName := baseName + ext

	// Serve image
	c.Header("Content-Type", "image/"+typeStr)
	c.Header("Content-Disposition", `inline; filename="`+finalFileName+`"`)
	c.Data(200, "image/"+typeStr, buf.Bytes())
}
