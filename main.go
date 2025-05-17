package main

import (
	"Pluto/app"
	"Pluto/pluto-image"
	"bytes"
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/chai2010/webp"
	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/rwcarlsen/goexif/tiff"
	"image"
	_ "image/gif"
	"image/jpeg"
	_ "image/jpeg"
	"image/png"
	_ "image/png"
	"io"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
)

type ImageMetadata struct {
	CreatedBy string
	Copyright string
	License   string
	AltText   string
	FocusX    float64
	FocusY    float64
	UserID    string
}

func uploadHandler(c *gin.Context) {
	// Parse form values from the POST request
	meta := ImageMetadata{
		License:   c.PostForm("license"),
		CreatedBy: c.PostForm("created_by"),
		Copyright: c.PostForm("copyright"),
		AltText:   c.PostForm("alt_text"),
		UserID:    c.PostForm("user_id"),
	}

	// Parse focus_x and focus_y as float64
	var err error
	if focusXStr := c.PostForm("focus_x"); focusXStr != "" {
		meta.FocusX, err = strconv.ParseFloat(focusXStr, 64)
		if err != nil {
			c.String(http.StatusBadRequest, "Invalid focus_x")
			return
		}
	}
	if focusYStr := c.PostForm("focus_y"); focusYStr != "" {
		meta.FocusY, err = strconv.ParseFloat(focusYStr, 64)
		if err != nil {
			c.String(http.StatusBadRequest, "Invalid focus_y")
			return
		}
	}

	// Debug print
	fmt.Printf("Metadata received: %+v\n", meta)

	file, err := c.FormFile("file_input")
	if err != nil {
		c.String(400, "File upload error: %s", err.Error())
		return
	}

	// Open the uploaded file
	src, err := file.Open()
	if err != nil {
		c.String(500, "Failed to open file: %s", err.Error())
		return
	}
	defer src.Close()

	// Read file into buffer for reuse
	buf := new(bytes.Buffer)
	if _, err := io.Copy(buf, src); err != nil {
		c.String(500, "Failed to read file: %s", err.Error())
		return
	}

	// Decode image config (dimensions and format)
	cfg, format, err := image.DecodeConfig(bytes.NewReader(buf.Bytes()))
	if err != nil {
		c.String(500, "Invalid image: %s", err.Error())
		return
	}

	// Decode EXIF if present
	exifData := make(map[string]string)
	if x, err := exif.Decode(bytes.NewReader(buf.Bytes())); err == nil {
		x.Walk(&exifWalker{m: exifData})
	}

	// Extract original and sanitize the filename
	originalFileName := filepath.Base(file.Filename)

	// Generate secure filename
	generatedFileName, err := app.GenerateImageFilename(originalFileName)
	if err != nil {
		c.String(500, "Failed to generate image filename: %s", err.Error())
		return
	}

	// Save image to disk
	dstPath := app.GApp.Config.ImageUploadDir + "/" + generatedFileName
	if err := os.WriteFile(dstPath, buf.Bytes(), 0644); err != nil {
		c.String(500, "Failed to save file: %s", err.Error())
		return
	}

	userId := 13 // TODO!
	// Insert metadata into DB
	_, err = app.GApp.ImageDB.Exec(context.Background(), `
        INSERT INTO pluto.image (file_name, gen_file_name, width, height, mime_type, exif, license, created_by, copyright, alt_text, user_id)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
    `, originalFileName, generatedFileName, cfg.Width, cfg.Height, format, exifData, meta.License, meta.CreatedBy, meta.Copyright, meta.AltText, userId)
	if err != nil {
		c.String(500, "DB insert failed: %s", err.Error())
		return
	}

	c.String(200, "✅ Uploaded: %s (saved as %s)", originalFileName, generatedFileName)
}

type exifWalker struct {
	m map[string]string
}

func (w *exifWalker) Walk(name exif.FieldName, tag *tiff.Tag) error {
	w.m[string(name)] = tag.String()
	return nil
}

type Param struct {
	Exist bool
	Value string
}

// Order of parameters (fixed positions)
var paramKeys = []string{
	"mode", "type", "quality", "width", "height",
	"ratio", "focusx", "focusy", "brightness", "contrast", "saturation", // ← 11 parameters
}

var paramIndex = map[string]int{
	"mode": 0, "type": 1, "quality": 2, "width": 3, "height": 4,
	"ratio": 5, "focusx": 6, "focusy": 7, "brightness": 8, "contrast": 9, "saturation": 10,
}

// Short codes mapped by param name
var paramShortCodes = map[string]string{
	"mode":       "m",
	"type":       "t",
	"quality":    "q",
	"width":      "w",
	"height":     "h",
	"ratio":      "r",
	"focusx":     "x",
	"focusy":     "y",
	"brightness": "b",
	"contrast":   "c",
	"saturation": "s",
}

func getOrDefault(param Param, def string) string {
	if param.Exist {
		return param.Value
	}
	return def
}

func atoiOrDefault(param Param, def int) int {
	if param.Exist {
		if v, err := strconv.Atoi(param.Value); err == nil {
			return v
		}
	}
	return def
}

func atoiOrDefaultClamped(param Param, def, min, max int) int {
	if param.Exist {
		if v, err := strconv.Atoi(param.Value); err == nil {
			if v < min {
				return min
			}
			if v > max {
				return max
			}
			return v
		}
	}
	return def
}

func atofOrDefault(param Param, def float64) float64 {
	if param.Exist {
		if v, err := strconv.ParseFloat(param.Value, 64); err == nil {
			return v
		}
	}
	return def
}

func serveImageByID(c *gin.Context) {
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
		aspectRatio, err = app.ParseAspectRatio(ratioStr)
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
	err = app.GApp.ImageDB.QueryRow(context.Background(), `
				SELECT id, mime_type FROM pluto.cache WHERE receipt = $1
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
		cachedImgPath := filepath.Join(app.GApp.Config.ImageCacheDir, imageReceipt) + "." + mimeType
		c.Header("Content-Disposition", `inline; filename="`+imageReceipt+`"`)
		c.File(cachedImgPath)
		return
	}

	// Fetch file metadata
	var fileName, genFileName string
	err = app.GApp.ImageDB.QueryRow(context.Background(), `
		SELECT file_name, gen_file_name, mime_type FROM pluto.image WHERE id = $1
	`, id).Scan(&fileName, &genFileName, &mimeType)
	if err != nil {
		c.String(404, "Image not found: %s", err.Error())
		return
	}

	if typeStr == "" {
		typeStr = mimeType
	}

	// Read and decode image
	imgPath := filepath.Join(app.GApp.Config.ImageUploadDir, genFileName)
	fileBytes, err := os.ReadFile(imgPath)
	if err != nil {
		c.String(500, "Failed to read image")
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
		cachedImgPath := filepath.Join(app.GApp.Config.ImageCacheDir, imageReceipt) + "." + typeStr

		// Save the generated image bytes to the cache path
		err := os.WriteFile(cachedImgPath, buf.Bytes(), 0644)
		if err != nil {
			// Log but don’t fail the response (optional)
			fmt.Printf("Failed to write cached image: %v\n", err)
		} else {
			// Insert new cache record in DB
			_, err = app.GApp.ImageDB.Exec(context.Background(), `
            INSERT INTO pluto.cache (receipt, image_id, mime_type)
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

func main() {
	configFileName := "config.json" // Default config file name
	if len(os.Args) > 1 {
		configFileName = os.Args[1] // Get config file name from args
	}

	err := app.GApp.LoadConfig(configFileName)
	if err != nil {
		panic(err)
	}

	err = app.GApp.PrepareSql()
	if err != nil {
		panic(err)
	}

	app.GApp.InitDB()
	defer app.GApp.ImageDB.Close()

	// Create upload dirs
	os.MkdirAll(app.GApp.Config.ImageUploadDir, os.ModePerm)

	// Create a Gin router
	r := gin.Default()
	gin.SetMode(gin.ReleaseMode)

	// Tell Gin where to find HTML templates
	r.LoadHTMLGlob("templates/*")

	// Route to serve upload form
	r.GET("/upload-image", func(c *gin.Context) {
		c.HTML(http.StatusOK, "upload-image.html", nil)
	})

	// Routes
	r.POST("/upload", uploadHandler)
	r.GET("/image", serveImageByID)

	fmt.Println("🚀 Server running at http://localhost:8080/upload-image")
	err = r.Run(":8080")
	if err != nil {
		log.Fatalf("Failed to run server: %v", err)
	}
}
