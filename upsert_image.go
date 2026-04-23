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

	"github.com/chai2010/webp"
	"github.com/disintegration/imaging"
	"github.com/gen2brain/avif"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/rwcarlsen/goexif/exif"
	"github.com/sndcds/grains/grains_file"
	"github.com/sndcds/grains/grains_uuid"
)

type UpsertImageResult struct {
	HttpStatus        int
	Message           string
	FileRemovedFlag   bool
	CacheFilesRemoved int
	ImageUuid         string
}

func UpsertImage(
	gc *gin.Context,
	context string,
	contextUuid string,
	identifier string,
	fileNamePrefix *string,
	userUuid string,
	postCallback TxFunc,
) (UpsertImageResult, error) {
	ctx := gc.Request.Context()
	dbSchema := PlutoInstance.DbSchema

	maxUploadSize := PlutoInstance.Config.PlutoMaxImageSize
	maxWidth := PlutoInstance.Config.PlutoMaxImagePx
	maxHeight := PlutoInstance.Config.PlutoMaxImagePx
	compressionQuality := PlutoInstance.Config.PlutoDefaultQuality

	var result UpsertImageResult
	genFileName := ""
	prevGenFileName := ""
	deleteCacheImageUuid := ""

	payloadStr := gc.PostForm("payload")
	if payloadStr == "" {
		result.HttpStatus = http.StatusBadRequest
		result.Message = "payload field is required"
		return result, errors.New("payload field is required")
	}

	// Unmarshal JSON into struct
	var meta ImageMeta
	if err := json.Unmarshal([]byte(payloadStr), &meta); err != nil {
		result.HttpStatus = http.StatusBadRequest
		result.Message = "invalid payload"
		return result, errors.New("invalid payload")
	}

	altText := &meta.AltText
	copyright := &meta.Copyright
	creatorName := &meta.Creator
	description := &meta.Description
	focusX := meta.FocusX
	focusY := meta.FocusY
	license := &meta.License

	imageUuid := ""
	insertImageFlag := false

	txErr := WithTransaction(ctx, PlutoInstance.DbPool, func(tx pgx.Tx) *ApiTxError {
		// Check context/identifier rules
		var contextMaxWidth *int
		var contextMaxHeight *int
		var contextMaxFileSize *int64
		var contextCompression *int
		contextRuleFound := true
		query := fmt.Sprintf(
			`SELECT max_width, max_height, max_file_size, compression
		         FROM %s.pluto_context_rules
        		 WHERE context = $1 AND identifier = $2`,
			PlutoInstance.DbSchema,
		)
		err := tx.QueryRow(ctx, query, context, identifier).Scan(
			&contextMaxWidth, &contextMaxHeight, &contextMaxFileSize, &contextCompression)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				contextRuleFound = false
				imageUuid = ""
			} else {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  errors.New("failed to get pluto context rule"),
				}
			}
		}
		if contextRuleFound {
			if contextMaxWidth != nil {
				maxWidth = *contextMaxWidth
			}
			if contextMaxHeight != nil {
				maxHeight = *contextMaxHeight
			}
			if contextMaxFileSize != nil {
				maxUploadSize = *contextMaxFileSize
			}
			if contextCompression != nil {
				compressionQuality = *contextCompression
			}
		}

		// Get imageUuid
		query = fmt.Sprintf(
			`SELECT pluto_image_uuid
		         FROM %s.pluto_image_link
        		 WHERE context = $1 AND context_uuid = $2::uuid AND identifier = $3`,
			PlutoInstance.DbSchema,
		)

		err = tx.QueryRow(ctx, query, context, contextUuid, identifier).Scan(&imageUuid)
		if err != nil {
			if errors.Is(err, pgx.ErrNoRows) {
				imageUuid = ""
				insertImageFlag = true
			} else {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  errors.New("failed to get pluto_image_uuid"),
				}
			}
		}

		file, err := gc.FormFile("file")
		if file != nil {
			// Check file size
			if file.Size > maxUploadSize {
				return &ApiTxError{
					Code: http.StatusBadRequest,
					Err: fmt.Errorf(
						"file too large, max size %.2f MB, file has %.2f MB",
						float64(maxUploadSize)/(1<<20),
						float64(file.Size)/(1<<20))}
			}

			// Read file into buffer for multiple uses
			buf := new(bytes.Buffer)
			src, err := file.Open()
			if err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  errors.New("failed to open uploaded file"),
				}
			}
			defer src.Close()

			if _, err := io.Copy(buf, src); err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  errors.New("failed to read uploaded file"),
				}
			}

			// Detect MIME type (use only first 512 bytes for detection)
			head := buf.Bytes()
			if len(head) > 512 {
				head = head[:512]
			}
			mimeType := http.DetectContentType(head)
			isAVIF := grains_file.IsAVIF(head)

			fmt.Printf("mimeType: %s\n", mimeType)
			fmt.Printf("isAVIF: %t\n", isAVIF)

			// Decode EXIF metadata if present
			exifData := make(map[string]string)
			x, err := exif.Decode(bytes.NewReader(buf.Bytes()))
			if err == nil {
				x.Walk(&exifWalker{m: exifData})
			}

			var img image.Image
			switch {
			case isAVIF:
				img, err = avif.Decode(bytes.NewReader(buf.Bytes()))
				mimeType = "image/webp"
			case mimeType == "image/webp":
				img, err = webp.Decode(bytes.NewReader(buf.Bytes()))
			default:
				img, _, err = image.Decode(bytes.NewReader(buf.Bytes()))
			}

			if err != nil {
				return &ApiTxError{
					Code: http.StatusBadRequest,
					Err:  errors.New("invalid image"),
				}
			}

			imageWidth := img.Bounds().Dx()
			imageHeight := img.Bounds().Dy()

			// Downscale if needed
			if imageWidth > maxWidth || imageHeight > maxHeight {
				img = imaging.Fit(img, maxWidth, maxHeight, imaging.Lanczos)
			}
			// Encode back into buffer (overwrite original!)
			buf.Reset()

			var fileExt string
			switch mimeType {
			case "image/png":
				err = imaging.Encode(buf, img, imaging.PNG)
				fileExt = ".png"
			case "image/jpeg":
				err = imaging.Encode(buf, img, imaging.JPEG, imaging.JPEGQuality(compressionQuality))
				fileExt = ".jpg"
			case "image/webp":
				fmt.Printf("eoncode as webp\n")
				err = webp.Encode(buf, img, &webp.Options{
					Quality: float32(compressionQuality),
				})
				fileExt = ".webp"
			default:
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  errors.New("unsupported image format"),
				}
			}

			fmt.Printf("fileExt: %s\n", fileExt)

			if err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  errors.New("image encoding failed"),
				}
			}

			imageWidth = img.Bounds().Dx()
			imageHeight = img.Bounds().Dy()

			// Generate uuid if neccessary
			if imageUuid == "" {
				imageUuid, err = grains_uuid.Uuidv7String()
				if err != nil {
					return &ApiTxError{
						Code: http.StatusInternalServerError,
						Err:  errors.New("failed to generate uuid"),
					}
				}
			}

			// Sanitize and generate filename
			originalFileName := filepath.Base(file.Filename)
			genFileName = fmt.Sprintf("%s%s", imageUuid, fileExt)

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
				genFileName = fmt.Sprintf("%s_%s", *fileNamePrefix, genFileName)
			}

			savePath := filepath.Join(imageDir, genFileName)
			err = os.WriteFile(savePath, buf.Bytes(), 0644)
			if err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  fmt.Errorf("failed to save file: %v", err),
				}
			}

			if insertImageFlag {
				// Insert new pluto image
				query := fmt.Sprintf(`
					INSERT INTO %s.pluto_image (uuid, file_name, gen_file_name, width, height, mime_type, exif, created_by)
					VALUES ($1::uuid, $2, $3, $4, $5, $6, $7, $8::uuid) RETURNING uuid`,
					dbSchema)

				_, err = tx.Exec(
					ctx, query,
					imageUuid,
					originalFileName,
					genFileName,
					imageWidth,
					imageHeight,
					mimeType,
					exifData,
					userUuid)
				if err != nil {
					return &ApiTxError{
						Code: http.StatusInternalServerError,
						Err:  fmt.Errorf("failed to insert pluto image: %v", err),
					}
				}
				result.Message = "image inserted successfully"
			} else {
				err := validateUuid(imageUuid)
				if err != nil {
					return &ApiTxError{
						Code: http.StatusInternalServerError,
						Err:  fmt.Errorf("invalid uuid: %s, %v", imageUuid, err),
					}
				}

				// Update existing pluto image
				query := fmt.Sprintf(`
WITH image AS (SELECT gen_file_name FROM %s.pluto_image WHERE uuid = $1::uuid)
UPDATE %s.pluto_image SET file_name = $2, gen_file_name = $3, width = $4, height = $5, mime_type = $6, exif = $7
FROM image WHERE %s.pluto_image.uuid = $1::uuid RETURNING image.gen_file_name
					`, dbSchema, dbSchema, dbSchema)

				err = tx.QueryRow(
					ctx, query,
					imageUuid,
					originalFileName,
					genFileName,
					imageWidth,
					imageHeight,
					mimeType,
					exifData,
				).Scan(&prevGenFileName)
				if err != nil {
					return &ApiTxError{
						Code: http.StatusInternalServerError,
						Err:  fmt.Errorf("failed to update pluto image: %v", err),
					}
				}
				result.Message = "image updated successfully"
				deleteCacheImageUuid = imageUuid
			}
		}

		// Check if cached images must be removed, if focus point changes
		prevFocusX, prevFocusY, err := GetImageFocusTx(ctx, tx, imageUuid)
		if err != nil {
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("get focus failed: %v", err),
			}
		}
		if !FloatPtrEqual(focusX, prevFocusX) || !FloatPtrEqual(focusY, prevFocusY) {
			deleteCacheImageUuid = imageUuid
		}

		query = fmt.Sprintf(
			`UPDATE %s.pluto_image
			SET alt_text = $1, copyright = $2, creator_name = $3, license = $4, description = $5, focus_x = $6, focus_y = $7
			WHERE uuid = $8`,
			dbSchema)

		// Update pluto_image
		_, err = tx.Exec(
			ctx, query,
			altText,
			copyright,
			creatorName,
			license,
			description,
			focusX,
			focusY,
			imageUuid)
		if err != nil {
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("update pluto_image failed: %v", err),
			}
		}

		// Insert/update pluto_image_link
		query = fmt.Sprintf(
			`INSERT INTO %s.pluto_image_link
				(pluto_image_uuid, context, context_uuid, identifier)
			VALUES ($1::uuid, $2, $3::uuid, $4)
			ON CONFLICT (context, context_uuid, identifier)
			DO UPDATE SET
				pluto_image_uuid = EXCLUDED.pluto_image_uuid`,
			PlutoInstance.DbSchema)

		_, err = tx.Exec(
			ctx,
			query,
			imageUuid,
			context,
			contextUuid,
			identifier,
		)
		if err != nil {
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("update pluto_image_link failed: %v", err),
			}
		}

		// Call the callback inside the transaction
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

	// Filesystem cleanup (post-commit)
	cleanup, err := CleanupPlutoImageFiles(deleteCacheImageUuid, "")
	if err == nil {
		result.CacheFilesRemoved = cleanup.CacheFilesRemoved
		result.FileRemovedFlag = cleanup.ImageFileRemoved
	}

	result.HttpStatus = http.StatusOK
	result.ImageUuid = imageUuid

	return result, nil
}

func validateUuid(u string) error {
	if _, err := uuid.Parse(u); err != nil {
		return fmt.Errorf("invalid UUID: %s", u)
	}
	return nil
}
