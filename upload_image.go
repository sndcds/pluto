package pluto

import (
	"bytes"
	"context"
	"fmt"
	"github.com/gin-gonic/gin"
	"github.com/rwcarlsen/goexif/exif"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
)

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

	fmt.Println("exifData:", exifData)

	// Extract original and sanitize the filename
	originalFileName := filepath.Base(file.Filename)

	fmt.Println("originalFileName:", originalFileName)

	// Generate secure filename
	generatedFileName, err := GenerateImageFilename(originalFileName)
	fmt.Println("generatedFileName:", generatedFileName)
	if err != nil {
		c.String(500, "Failed to generate image filename: %s", err.Error())
		return
	}

	// Save image to disk
	dstPath := Singleton.Config.PlutoImageDir + "/" + generatedFileName
	if err := os.WriteFile(dstPath, buf.Bytes(), 0644); err != nil {
		c.String(500, "Failed to save file: %s", err.Error())
		return
	}

	fmt.Println("originalFileName:", originalFileName)
	fmt.Println("generatedFileName:", generatedFileName)
	fmt.Println("cfg.Width:", cfg.Width)
	fmt.Println("cfg.Height:", cfg.Height)
	fmt.Println("format:", format)
	fmt.Println("exifData:", exifData)
	fmt.Println("meta.License:", meta.License)
	fmt.Println("meta.CreatedBy:", meta.CreatedBy)
	fmt.Println("meta.Copyright:", meta.Copyright)
	fmt.Println("meta.AltText:", meta.AltText)

	userId := 13 // TODO!
	// Insert metadata into DB
	_, err = Singleton.Db.Exec(context.Background(), `
        INSERT INTO uranus.pluto_image (file_name, gen_file_name, width, height, mime_type, exif, license, created_by, copyright, alt_text, user_id)
        VALUES ($1, $2, $3, $4, $5, $6, $7, $8, $9, $10, $11)
    `, originalFileName, generatedFileName, cfg.Width, cfg.Height, format, exifData, meta.License, meta.CreatedBy, meta.Copyright, meta.AltText, userId)
	if err != nil {
		c.String(500, "DB insert failed: %s", err.Error())
		return
	}

	fmt.Println("INSERT INTO uranus.pluto_image done")

	c.String(200, "✅ Uploaded: %s (saved as %s)", originalFileName, generatedFileName)
}
