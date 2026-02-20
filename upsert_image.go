package pluto

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"image"
	"io"
	"net/http"
	"os"
	"path/filepath"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/rwcarlsen/goexif/exif"
)

type UpsertImageResult struct {
	HttpStatus        int
	Message           string
	FileRemovedFlag   bool
	CacheFilesRemoved int
	ImageId           int
}

func UpsertImage(
	gc *gin.Context,
	context string,
	contextId int,
	identifier string,
	fileNamePrefix *string,
	userId int,
	postCallback TxFunc,
) (UpsertImageResult, error) {
	ctx := gc.Request.Context()
	dbSchema := PlutoInstance.DbSchema

	fmt.Println("UpsertImage 1")

	var result UpsertImageResult
	var previousGenFileName *string
	deleteCacheImageId := -1

	// Get meta JSON from form
	payloadStr := gc.PostForm("payload")
	fmt.Println("payloadStr", payloadStr)
	if payloadStr == "" {
		result.HttpStatus = http.StatusBadRequest
		result.Message = "meta field is required"
		return result, fmt.Errorf("meta field is required")
	}

	// Unmarshal JSON into struct
	var meta ImageMeta
	if err := json.Unmarshal([]byte(payloadStr), &meta); err != nil {
		result.HttpStatus = http.StatusBadRequest
		result.Message = fmt.Sprintf("invalid meta JSON: %v", err)
		return result, fmt.Errorf("invalid meta JSON: %v", err)
	}

	altText := &meta.Alt
	copyright := &meta.Copyright
	creatorName := &meta.Creator
	description := &meta.Description
	focusX := meta.FocusX
	focusY := meta.FocusY
	licenseType := &meta.LicenseType

	fmt.Println("altText:", altText)
	fmt.Println("copyright:", copyright)
	fmt.Println("creatorName:", creatorName)
	fmt.Println("description:", description)
	fmt.Println("focusX:", focusX)
	fmt.Println("focusY:", focusY)
	fmt.Println("licenseType:", licenseType)

	imageId := -1
	insertImageFlag := true

	txErr := WithTransaction(ctx, PlutoInstance.DbPool, func(tx pgx.Tx) *ApiTxError {

		// Get imageId
		query := fmt.Sprintf(
			`SELECT pluto_image_id
		         FROM %s.pluto_image_link
        		 WHERE context = $1 AND context_id = $2 AND identifier = $3`,
			PlutoInstance.DbSchema,
		)
		fmt.Println("UpsertImage query", query)

		err := tx.QueryRow(
			ctx,
			query,
			context,
			contextId,
			identifier,
		).Scan(&imageId)

		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				imageId = -1
			} else {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  fmt.Errorf("failed to get image_id: %v", err),
				}
			}
		}

		fmt.Println("   context: ", context)
		fmt.Println("   contextId: ", contextId)
		fmt.Println("   identifier: ", identifier)

		insertImageFlag = imageId < 0

		file, err := gc.FormFile("file")
		if file != nil {
			fmt.Println("file", file.Filename)
			// Upload a new file
			// Read file into buffer for multiple uses
			buf := new(bytes.Buffer)
			src, err := file.Open()
			if err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  fmt.Errorf("failed to open uploaded file: %v", err),
				}
			}
			defer src.Close()

			if _, err := io.Copy(buf, src); err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  fmt.Errorf("failed to read uploaded file: %v", err),
				}
			}

			// Detect MIME type (use only first 512 bytes for detection)
			mimeType := http.DetectContentType(buf.Bytes()[:512])

			// Decode image config for dimensions
			cfg, _, err := image.DecodeConfig(bytes.NewReader(buf.Bytes()))
			if err != nil {
				return &ApiTxError{
					Code: http.StatusBadRequest,
					Err:  fmt.Errorf("invalid image: %v", err),
				}
			}

			// Decode EXIF metadata if present
			exifData := make(map[string]string)
			x, err := exif.Decode(bytes.NewReader(buf.Bytes()))
			if err == nil {
				x.Walk(&exifWalker{m: exifData})
			}

			// Sanitize and generate filename
			originalFileName := filepath.Base(file.Filename)
			generatedFileName, err := GenerateImageFilename(originalFileName)
			if err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  fmt.Errorf("failed to generate filename: %v", err),
				}
			}

			// Ensure upload directory exists
			imageDir := PlutoInstance.Config.PlutoImageDir
			if err := os.MkdirAll(imageDir, os.ModePerm); err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  fmt.Errorf("failed to create directory: %v", err),
				}
			}

			if fileNamePrefix != nil {
				generatedFileName = fmt.Sprintf("%s_%s", *fileNamePrefix, generatedFileName)
			}

			savePath := filepath.Join(imageDir, generatedFileName)
			if err = os.WriteFile(savePath, buf.Bytes(), 0644); err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  fmt.Errorf("failed to save file: %v", err),
				}
			}

			if insertImageFlag {
				// Insert new pluto image
				query := fmt.Sprintf(`
INSERT INTO %s.pluto_image (file_name, gen_file_name, width, height, mime_type, exif, user_id)
VALUES ($1, $2, $3, $4, $5, $6, $7) RETURNING id`,
					dbSchema)

				err = tx.QueryRow(
					ctx, query,
					originalFileName,
					generatedFileName,
					cfg.Width, cfg.Height,
					mimeType,
					exifData,
					userId).
					Scan(&imageId)
				if err != nil {
					return &ApiTxError{
						Code: http.StatusInternalServerError,
						Err:  fmt.Errorf("failed to insert pluto image: %v", err),
					}
				}
				result.Message = "image inserted successfully"
			} else {
				// Update existing pluto image
				query := fmt.Sprintf(`
WITH image AS (SELECT gen_file_name FROM %s.pluto_image WHERE id = $7)
UPDATE %s.pluto_image SET file_name = $1, gen_file_name = $2, width = $3, height = $4, mime_type = $5, exif = $6
FROM image WHERE %s.pluto_image.id = $7 RETURNING image.gen_file_name
					`, dbSchema, dbSchema, dbSchema)

				fmt.Println("Update query: ", query)
				err := tx.QueryRow(
					ctx, query,
					originalFileName,
					generatedFileName,
					cfg.Width,
					cfg.Height,
					mimeType,
					exifData,
					imageId,
				).Scan(&previousGenFileName)
				if err != nil {
					return &ApiTxError{
						Code: http.StatusInternalServerError,
						Err:  fmt.Errorf("failed to update pluto image: %v", err),
					}
				}
				result.Message = "image updated successfully"
				deleteCacheImageId = imageId
			}
		}

		// Check if cached images must be removed, if focus point changes
		prevFocusX, prevFocusY, err := GetImageFocusTx(ctx, tx, imageId)
		if err != nil {
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("get focus failed: %v", err),
			}
		}
		if !FloatPtrEqual(focusX, prevFocusX) || !FloatPtrEqual(focusY, prevFocusY) {
			deleteCacheImageId = imageId
		}

		fmt.Println("prevFocus:", prevFocusX, prevFocusY)

		query = fmt.Sprintf(
			`UPDATE %s.pluto_image
			SET alt_text = $1, copyright = $2, creator_name = $3, license_type = $4, description = $5, focus_x = $6, focus_y = $7
			WHERE id = $8`,
			dbSchema)
		fmt.Println("query:", query)
		fmt.Println("altText:", altText)
		fmt.Println("copyright:", copyright)
		fmt.Println("creatorName:", creatorName)
		fmt.Println("licenseType:", licenseType)
		fmt.Println("description:", description)
		fmt.Println("focusX:", focusX)
		fmt.Println("focusY:", focusY)
		fmt.Println("imageId:", imageId)

		// Update pluto_image
		_, err = tx.Exec(
			ctx, query,
			altText,
			copyright,
			creatorName,
			licenseType,
			description,
			focusX,
			focusY,
			imageId)
		if err != nil {
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("update pluto_image failed: %v", err),
			}
		}

		// Update pluto_image_link
		query = fmt.Sprintf(
			`INSERT INTO %s.pluto_image_link
				(pluto_image_id, context, context_id, identifier)
			VALUES ($1, $2, $3, $4)
			ON CONFLICT (context, context_id, identifier)
			DO UPDATE SET
				pluto_image_id = EXCLUDED.pluto_image_id
			RETURNING id`,
			PlutoInstance.DbSchema)
		fmt.Println("query:", query)
		fmt.Println("imageId:", imageId)
		fmt.Println("context:", context)
		fmt.Println("contextId:", contextId)
		fmt.Println("identifier:", identifier)

		var plutoImageLinkId int
		err = tx.QueryRow(
			ctx,
			query,
			imageId,
			context,
			contextId,
			identifier,
		).Scan(&plutoImageLinkId)
		if err != nil {
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("update pluto_image_link failed: %v", err),
			}
		}

		// 🔑 call the callback INSIDE the transaction
		if postCallback != nil {
			if err := postCallback(ctx, tx); err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  fmt.Errorf("post callback function failed: %v", err),
				}
			}
		}

		return nil
	})
	if txErr != nil {
		result.HttpStatus = txErr.Code
		result.Message = txErr.Err.Error()
		return result, txErr.Err
	}

	fmt.Println("cleanup")

	// Filesystem cleanup (post-commit)
	cleanup, err := CleanupPlutoImageFiles(deleteCacheImageId, previousGenFileName)
	if err == nil {
		result.CacheFilesRemoved = cleanup.CacheFilesRemoved
		result.FileRemovedFlag = cleanup.ImageFileRemoved
	}

	result.HttpStatus = http.StatusOK
	result.ImageId = imageId

	fmt.Println("imageId", imageId)

	return result, nil
}
