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
	ImageId           int
}

// DeleteImage deletes an image by context/contextId/identifier
func DeleteImage(
	gc *gin.Context,
	context string,
	contextId int,
	identifier string,
	postCallback TxFunc,
) (DeleteImageResult, error) {
	ctx := gc.Request.Context()
	dbSchema := PlutoInstance.DbSchema

	var result DeleteImageResult
	var genFileName *string
	imageId := -1

	txErr := WithTransaction(ctx, PlutoInstance.DbPool, func(tx pgx.Tx) *ApiTxError {
		// Get the linked image ID and generated file name
		query := fmt.Sprintf(
			`SELECT i.id, i.gen_file_name
			 FROM %s.pluto_image_link l
			 JOIN %s.pluto_image i ON i.id = l.pluto_image_id
			 WHERE l.context = $1 AND l.context_id = $2 AND l.identifier = $3`,
			dbSchema, dbSchema,
		)
		err := tx.QueryRow(ctx, query, context, contextId, identifier).Scan(&imageId, &genFileName)
		if err != nil {
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
			 WHERE context = $1 AND context_id = $2 AND identifier = $3`,
			dbSchema,
		)
		if _, err := tx.Exec(ctx, query, context, contextId, identifier); err != nil {
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("failed to delete pluto_image_link: %v", err),
			}
		}

		// Optionally delete the image row itself if no other links exist
		var linkCount int
		query = fmt.Sprintf(
			`SELECT COUNT(*) FROM %s.pluto_image_link WHERE pluto_image_id = $1`,
			dbSchema,
		)
		err = tx.QueryRow(ctx, query, imageId).Scan(&linkCount)
		if err != nil {
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("failed to count image links: %v", err),
			}
		}

		if linkCount == 0 {
			query = fmt.Sprintf(
				`DELETE FROM %s.pluto_image WHERE id = $1`,
				dbSchema,
			)
			if _, err := tx.Exec(ctx, query, imageId); err != nil {
				return &ApiTxError{
					Code: http.StatusInternalServerError,
					Err:  fmt.Errorf("failed to delete pluto_image: %v", err),
				}
			}
		}

		_, err = DeleteCacheTx(ctx, tx, imageId)
		if err != nil {
			return &ApiTxError{
				Code: http.StatusInternalServerError,
				Err:  fmt.Errorf("failed to delete cached files: %v", err),
			}
		}

		// Call optional post-transaction callback
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
		return result, txErr.Err
	}

	// Filesystem cleanup (post-commit)
	cleanup, err := CleanupPlutoImageFiles(imageId, genFileName)
	if err == nil {
		result.CacheFilesRemoved = cleanup.CacheFilesRemoved
		result.FileRemovedFlag = cleanup.ImageFileRemoved
	}

	result.HttpStatus = http.StatusOK
	result.Message = "image deleted successfully"
	result.ImageId = imageId

	return result, nil
}
