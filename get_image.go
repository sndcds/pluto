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

func getImageHandler(gc *gin.Context) {
	ctx := gc.Request.Context()

	imageId, ok := ParamInt(gc, "id")
	if !ok {
		gc.JSON(http.StatusBadRequest, gin.H{"error": "invalid image ID"})
		return
	}

	pType := GetUrlQueryParam(gc, "type", String)
	pFit := GetUrlQueryParam(gc, "fit", String)
	pQuality := GetUrlQueryParam(gc, "quality", Int)
	pWidth := GetUrlQueryParam(gc, "width", Int)
	pHeight := GetUrlQueryParam(gc, "height", Int)
	pRatio := GetUrlQueryParam(gc, "ratio", Rational)
	pFocusX := GetUrlQueryParam(gc, "focusx", Float)
	pFocusY := GetUrlQueryParam(gc, "focusy", Float)
	pLossless := GetUrlQueryParam(gc, "lossless", Boolean)

	typeStr := pType.ValueOr("jpg")
	quality := pQuality.IntOr(100)

	fmt.Println("..................")
	pType.Println()
	pFit.Println()
	pQuality.Println()
	pWidth.Println()
	pHeight.Println()
	pRatio.Println()
	pFocusX.Println()
	pFocusY.Println()
	pLossless.Println()

	if pRatio.Exist && (!pWidth.Exist || !pHeight.Exist) {
		aspectRatio := pRatio.Float
		fmt.Println("aspectRatio: ", aspectRatio)
		if aspectRatio <= 0.0001 {
			gc.String(http.StatusBadRequest, "Invalid ratio format. Use format like '16:9'")
			return
		}
		if pWidth.Exist {
			pHeight = UrlQueryParam{
				Name:  "height",
				Type:  Int,
				Exist: true,
				Int64: int64(float64(pWidth.Int64) / aspectRatio),
			}
			fmt.Println("New height ...", pHeight.Int())
		} else if pHeight.Exist {
			pWidth = UrlQueryParam{
				Name:  "width",
				Type:  Int,
				Exist: true,
				Int64: int64(float64(pHeight.Int64) / aspectRatio),
			}
			fmt.Println("New width ...", pWidth.Int())
		} else {
			gc.String(http.StatusBadRequest, "Either width or height must be provided if using ratio")
			return
		}
	}

	var paramCode, paramValues string
	if pFit.Exist {
		paramCode += "f"
		switch pFit.Value {
		case "cover":
			paramValues += "01"
		default:
			paramValues += "00"
		}
	}
	if pType.Exist {
		paramCode += "t"
		switch pType.Value {
		case "png":
			paramValues += "01"
		case "webp":
			paramValues += "02"
		default:
			paramValues += "00"
		}
	}
	if pQuality.Exist {
		paramCode += "q"
		paramValues += fmt.Sprintf("_%02x_", pQuality.Int)
	}
	if pWidth.Exist {
		paramCode += "w"
		paramValues += fmt.Sprintf("_%04x_", pWidth.Int)
	}
	if pHeight.Exist {
		paramCode += "h"
		paramValues += fmt.Sprintf("_%04x_", pHeight.Int)
	}
	if pFocusX.Exist {
		paramCode += "x"
		paramValues += fmt.Sprintf("_%03x_", pFocusX.Int)
	}
	if pFocusY.Exist {
		paramCode += "y"
		paramValues += fmt.Sprintf("_%03x_", pFocusY.Int)
	}

	imageReceipt := fmt.Sprintf("%x_%s_%s", imageId, paramCode, paramValues)
	cacheFileName := imageReceipt + "." + typeStr
	cacheFilePath := filepath.Join(Singleton.Config.PlutoCacheDir, cacheFileName)

	fmt.Println("imageReceipt: ", imageReceipt)
	fmt.Println("cacheFileName: ", cacheFileName)
	fmt.Println("cacheFilePath: ", cacheFilePath)

	if _, err := os.Stat(cacheFilePath); err == nil {
		gc.Header("Content-Disposition", `inline; filename="`+cacheFileName+`"`)
		gc.File(cacheFilePath)
		fmt.Println("cacheFile used!")
		return
	}

	var fileName, genFileName, mimeType string
	sql := fmt.Sprintf(`SELECT file_name, gen_file_name, mime_type FROM %s.pluto_image WHERE id = $1`, pq.QuoteIdentifier(Singleton.Config.DbSchema))
	fmt.Println(sql)
	err := Singleton.Db.QueryRow(ctx, sql, imageId).Scan(&fileName, &genFileName, &mimeType)
	if err != nil {
		gc.String(http.StatusBadRequest, "Image not found")
		return
	}

	imgPath := filepath.Join(Singleton.Config.PlutoImageDir, genFileName)

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

	if pWidth.Exist || pHeight.Exist || pRatio.Exist {
		img = CropToAspectAdvanced(
			img,
			pFit.Value,
			pRatio.Float,
			pFocusX.Float,
			pFocusY.Float,
			pWidth.Int(),
			pHeight.Int())
	}

	var buf bytes.Buffer
	switch typeStr {
	case "image/jpeg", "jpeg", "jpg":
		typeStr = "jpg"
		err = jpeg.Encode(&buf, img, &jpeg.Options{Quality: quality})
	case "image/png", "png":
		typeStr = "png"
		err = png.Encode(&buf, img)
	case "image/webp", "webp":
		typeStr = "webp"
		options := &webp.Options{Lossless: pLossless.Bool}
		if !pLossless.Bool {
			options.Quality = float32(quality)
		}
		err = webp.Encode(&buf, img, options)
	default:
		gc.JSON(http.StatusUnsupportedMediaType, gin.H{"error": fmt.Sprintf("Unsupported image format: image/%s", typeStr)})
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
		_, _ = Singleton.Db.Exec(ctx, sql, imageReceipt, imageId, typeStr)
	}

	gc.Header("Content-Type", "image/"+typeStr)
	gc.Header("Content-Disposition", `inline; filename="`+cacheFileName+`"`)
	gc.Data(http.StatusOK, "image/"+typeStr, buf.Bytes())

}
