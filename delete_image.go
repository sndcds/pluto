package pluto

import (
	"errors"
	"fmt"
	"net/http"

	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
)

// DeleteImageResult mirrors UpsertImageResult
type DeleteImageResult struct {
	HttpStatus        int
	Message           string
	FileRemovedFlag   bool
	CacheFilesRemoved int
	ImageUuid         string
}

// DeleteImage deletes an image by context/contextId/identifier
func DeleteImage(
	gc *gin.Context,
	context string,
	contextUuid string,
	identifier string,
	postCallback TxFunc,
) (DeleteImageResult, error) {
	ctx := gc.Request.Context()
	dbSchema := PlutoInstance.DbSchema

	var result DeleteImageResult
	var genFileName *string
	imageUuid := ""

	txErr := WithTransaction(ctx, PlutoInstance.DbPool, func(tx pgx.Tx) *ApiTxError {
		// Get the linked imageUuid and generated file name
		query := fmt.Sprintf(
			`SELECT i.uuid, i.gen_file_name
			 FROM %s.pluto_image_link l
			 JOIN %s.pluto_image i ON i.uuid = l.pluto_image_uuid
			 WHERE l.context = $1 AND l.context_uuid = $2::uuid AND l.identifier = $3`,
			dbSchema, dbSchema,
		)
		err := tx.QueryRow(ctx, query, context, contextUuid, identifier).Scan(&imageUuid, &genFileName)
		if err != nil {
			fmt.Print("Error 1: %v\n", err)
			if errors.Is(err, pgx.ErrNoRows) {
				// Image does not exist, nothing to delete
				result.HttpStatus = http.StatusNotFound
				result.Message = "no image found for deletion"
				return nil
			}
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("failed to get image for deletion: %v", err),
			}
		}

		// Delete the link first
		query = fmt.Sprintf(
			`DELETE FROM %s.pluto_image_link
			 WHERE context = $1 AND context_uuid = $2::uuid AND identifier = $3`,
			dbSchema,
		)
		_, err = tx.Exec(ctx, query, context, contextUuid, identifier)
		if err != nil {
			fmt.Print("Error 2: %v\n", err)
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("failed to delete pluto_image_link: %v", err),
			}
		}

		// Optionally delete the image row itself if no other links exist
		var linkCount int
		query = fmt.Sprintf(
			`SELECT COUNT(*) FROM %s.pluto_image_link WHERE pluto_image_uuid = $1::uuid`,
			dbSchema,
		)
		err = tx.QueryRow(ctx, query, imageUuid).Scan(&linkCount)
		if err != nil {
			fmt.Print("Error 3: %v\n", err)
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("failed to count image links: %v", err),
			}
		}

		if linkCount == 0 {
			query = fmt.Sprintf(`DELETE FROM %s.pluto_image WHERE uuid = $1::uuid`, dbSchema)
			_, err := tx.Exec(ctx, query, imageUuid)
			if err != nil {
				fmt.Print("Error 4: %v\n", err)
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  fmt.Errorf("failed to delete pluto_image: %v", err),
				}
			}
		}

		_, err = DeleteCacheTx(ctx, tx, imageUuid)
		if err != nil {
			fmt.Print("Error 5: %v\n", err)
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("failed to delete cached files: %v", err),
			}
		}

		// Call optional post-transaction callback
		if postCallback != nil {
			if err := postCallback(ctx, tx); err != nil {
				fmt.Print("Error 6: %v\n", err)
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
		return result, txErr.Err
	}

	// Filesystem cleanup (post-commit)
	cleanup, err := CleanupPlutoImageFiles(imageUuid, genFileName)
	if err == nil {
		result.CacheFilesRemoved = cleanup.CacheFilesRemoved
		result.FileRemovedFlag = cleanup.ImageFileRemoved
	}

	result.HttpStatus = http.StatusOK
	result.Message = "image deleted successfully"
	result.ImageUuid = imageUuid

	return result, nil
}
