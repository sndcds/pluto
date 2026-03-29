package pluto

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"
	"image"
	"image/draw"
	"math"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"github.com/jackc/pgx/v5"
	"github.com/nfnt/resize"
)

const eps = 1e-5

func ParamInt(gc *gin.Context, key string) (int, bool) {
	str := gc.Param(key)
	val, err := strconv.Atoi(str)
	if err != nil {
		return 0, false
	}
	return val, true
}

func GetQueryInt(gc *gin.Context, key string) (int, bool) {
	str, ok := gc.GetQuery(key)
	if !ok {
		return 0, false
	}
	val, err := strconv.Atoi(str)
	if err != nil {
		return 0, false
	}
	return val, true
}

func GetQueryIntDefault(gc *gin.Context, key string, def int) (int, bool) {
	str, ok := gc.GetQuery(key)
	if !ok {
		return def, true
	}
	val, err := strconv.Atoi(str)
	if err != nil {
		return 0, false
	}
	return val, true
}

func GetQueryBoolDefault(gc *gin.Context, key string, def bool) (bool, bool) {
	str, ok := gc.GetQuery(key)
	if !ok {
		return def, true
	}
	if str == "" {
		return true, true
	}
	val, err := strconv.ParseBool(str)
	if err != nil {
		return false, false
	}
	return val, true
}

func ParseAspectRatio(s string) (float32, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, errors.New("invalid ratio")
	}
	w, err1 := strconv.ParseFloat(parts[0], 64)
	h, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || h == 0 {
		return 0, errors.New("invalid ratio")
	}
	return float32(w) / float32(h), nil
}

func ResizeToWidth(img image.Image, width int, ratio string) image.Image {
	// Parse ratio like "7by5"
	var height uint
	if parts := strings.Split(ratio, "by"); len(parts) == 2 {
		rw, _ := strconv.Atoi(parts[0])
		rh, _ := strconv.Atoi(parts[1])
		if rw > 0 && rh > 0 {
			height = uint(float64(width) * float64(rh) / float64(rw))
		}
	}

	if height == 0 {
		// Maintain original aspect ratio
		return resize.Resize(uint(width), 0, img, resize.Lanczos3)
	}

	return resize.Resize(uint(width), height, img, resize.Lanczos3)
}

func cropImage(img image.Image, rect image.Rectangle) image.Image {
	return img.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(rect)
}

// CropWithFocus crops an image to a target aspect ratio and optionally resizes,
// centered on the focus point, while preserving alpha transparency.
func CropWithFocus(
	img image.Image,
	targetRatio float32, // desired width/height ratio (ignored if targetW/targetH set)
	focusX, focusY float32, // normalized [0,1] focus point
	targetW, targetH int, // optional final size; 0 to keep crop size
) image.Image {
	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	srcRatio := float32(srcW) / float32(srcH)

	// Clamp focus values to [0,1]
	focusX = clampNormalized(focusX)
	focusY = clampNormalized(focusY)

	// Compute target ratio if targetW and targetH are set
	if targetW > 0 && targetH > 0 {
		targetRatio = float32(targetW) / float32(targetH)
	} else if targetRatio <= 0.00001 {
		targetRatio = srcRatio
	}

	// Determine crop size (never larger than source)
	var cropW, cropH int
	if srcRatio > targetRatio {
		// Source is wider than target ratio → limit width
		cropH = srcH
		cropW = int(float32(cropH) * targetRatio)
	} else {
		// Source is taller than target ratio → limit height
		cropW = srcW
		cropH = int(float32(cropW) / targetRatio)
	}

	// Compute top-left of crop rectangle, centered on focus
	centerX := int(focusX * float32(srcW))
	centerY := int(focusY * float32(srcH))
	x0 := clampInt(centerX-cropW/2, 0, srcW-cropW)
	y0 := clampInt(centerY-cropH/2, 0, srcH-cropH)
	cropRect := image.Rect(x0, y0, x0+cropW, y0+cropH)

	// Crop into NRGBA to preserve alpha
	var cropped *image.NRGBA
	if nrgbaImg, ok := img.(*image.NRGBA); ok {
		cropped = imaging.Crop(nrgbaImg, cropRect)
	} else {
		dst := imaging.New(cropW, cropH, image.Transparent)
		draw.Draw(dst, dst.Bounds(), img, cropRect.Min, draw.Over)
		cropped = dst
	}

	// Optional resizing: scale down only, never exceed cropped size
	finalW, finalH := cropW, cropH
	if targetW > 0 {
		scale := float64(targetW) / float64(cropW)
		if scale < 1 {
			finalW = targetW
			finalH = int(float64(cropH) * scale)
		}
	}
	if targetH > 0 {
		scale := float64(targetH) / float64(cropH)
		if scale < 1 {
			finalH = targetH
			finalW = int(float64(cropW) * scale)
		}
	}

	// Resize only if needed
	if finalW != cropW || finalH != cropH {
		cropped = imaging.Resize(cropped, finalW, finalH, imaging.Lanczos)
	}

	return cropped
}

func CropWithFocusWithoutAlpha(
	img image.Image,
	targetRatio float32,
	focusX, focusY float32,
	targetW, targetH int,
) image.Image {
	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	srcRatio := float32(srcW) / float32(srcH)

	// Clamp focus values to [0,1]
	focusX = clampNormalized(focusX)
	focusY = clampNormalized(focusY)

	// Determine target ratio
	if targetW > 0 && targetH > 0 {
		targetRatio = float32(targetW) / float32(targetH)
	} else if targetRatio <= eps {
		targetRatio = srcRatio
	}

	// Determine crop size based on target aspect ratio
	var cropW, cropH int
	if srcRatio > targetRatio {
		// Source is wider → crop width
		cropH = srcH
		cropW = int(float32(cropH) * targetRatio)
	} else {
		// Source is taller → crop height
		cropW = srcW
		cropH = int(float32(cropW) / targetRatio)
	}

	// Compute top-left corner so that focus point is centered
	centerX := int(focusX * float32(srcW))
	centerY := int(focusY * float32(srcH))
	x0 := clampInt(centerX-cropW/2, 0, srcW-cropW)
	y0 := clampInt(centerY-cropH/2, 0, srcH-cropH)

	cropped := imaging.Crop(img, image.Rect(x0, y0, x0+cropW, y0+cropH))

	// --- Compute final size ---
	finalW, finalH := cropW, cropH

	if targetW > 0 {
		// Width is driving value
		finalW = targetW
		finalH = int(float32(finalW) / float32(cropW) * float32(cropH))
	}
	if targetH > 0 && finalH > targetH {
		// Make sure height does not exceed targetH
		finalH = targetH
		finalW = int(float32(finalH) / float32(cropH) * float32(cropW))
	}

	// Resize if needed
	if finalW != cropW || finalH != cropH {
		cropped = imaging.Resize(cropped, finalW, finalH, imaging.Lanczos)
	}

	return cropped
}

func clampNormalized(v float32) float32 {
	if v < 0.0 {
		return 0.0
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}

func clampInt(value, minVal, maxVal int) int {
	if value < minVal {
		return minVal
	}
	if value > maxVal {
		return maxVal
	}
	return value
}

// EncodeFloat32ForPath converts a float32 into a safe ASCII filename string.
// Output contains only [0-9a-f] and is exactly 8 characters long.
func EncodeFloat32ForPath(f float32) string {
	bits := math.Float32bits(f) // Get IEEE 754 binary representation
	buf := make([]byte, 4)
	buf[0] = byte(bits >> 24)
	buf[1] = byte(bits >> 16)
	buf[2] = byte(bits >> 8)
	buf[3] = byte(bits)
	return hex.EncodeToString(buf) // 4 bytes -> 8 hex chars
}

// DecodeFloat32FromPath reverses the encoding back to float32.
func DecodeFloat32FromPath(s string) (float32, error) {
	b, err := hex.DecodeString(s)
	if err != nil || len(b) != 4 {
		return 0, errors.New("invalid hex float string")
	}
	bits := uint32(b[0])<<24 | uint32(b[1])<<16 | uint32(b[2])<<8 | uint32(b[3])
	return math.Float32frombits(bits), nil
}

type ImageCleanupResult struct {
	CacheFilesRemoved int
	ImageFileRemoved  bool
}

func CleanupPlutoImageFiles(
	imageUuid string,
	fileName *string,
) (*ImageCleanupResult, error) {
	result := &ImageCleanupResult{}

	// Always clean cache (safe even if image doesn't exist)
	cacheFilesRemoved, err := CleanupPlutoCache(imageUuid)
	if err != nil {
		return result, err
	}
	result.CacheFilesRemoved = cacheFilesRemoved

	// Only clean image file if we actually have a filename
	if fileName != nil && *fileName != "" {
		imageFileRemoved, err := CleanupPlutoImage(*fileName)
		if err != nil {
			return result, err
		}
		result.ImageFileRemoved = imageFileRemoved
	}

	return result, nil
}

// Delete original image file
func CleanupPlutoImage(imageFileName string) (bool, error) {
	if imageFileName != "" {
		path := filepath.Join(PlutoInstance.Config.PlutoImageDir, imageFileName)
		if err := RemoveFile(path); err != nil {
			return false, err
		}
	}
	return true, nil
}

// Delete cache files
func CleanupPlutoCache(imageUuid string) (int, error) {
	prefix := fmt.Sprintf("%s_", imageUuid)
	cacheFilesRemoved, err := DeleteFilesWithPrefix(PlutoInstance.Config.PlutoCacheDir, prefix)
	if err != nil {
		return 0, err
	}
	return cacheFilesRemoved, nil
}

// DeleteFilesWithPrefix deletes all files in a directory that start with the given prefix.
// Returns the number of deleted files and an error (if any).
func DeleteFilesWithPrefix(dir string, prefix string) (int, error) {
	if len(prefix) < 1 {
		return 0, errors.New("prefix required")
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		return 0, fmt.Errorf("failed to read directory: %w", err)
	}
	deletedCount := 0
	for _, entry := range entries {
		if entry.IsDir() {
			continue
		}
		name := entry.Name()
		if strings.HasPrefix(name, prefix) {
			fullPath := filepath.Join(dir, name)
			if err := os.Remove(fullPath); err != nil {
				return deletedCount, fmt.Errorf("failed to delete %s: %w", fullPath, err)
			}
			deletedCount++
		}
	}
	return deletedCount, nil
}

// RemoveFile deletes a file at the given path.
// Returns an error if the file cannot be deleted.
func RemoveFile(path string) error {
	if path == "" {
		return errors.New("file path required")
	}

	// Check if file exists first (optional)
	if _, err := os.Stat(path); os.IsNotExist(err) {
		return fmt.Errorf("file does not exist: %s", path)
	}

	if err := os.Remove(path); err != nil {
		return fmt.Errorf("failed to delete file %s: %w", path, err)
	}

	return nil
}

// GetImageFocus returns focus_x and focus_y for an image
func GetImageFocusTx(
	ctx context.Context,
	tx pgx.Tx,
	imageUuid string,
) (focusX *float64, focusY *float64, err error) {

	query := fmt.Sprintf(
		`SELECT focus_x, focus_y FROM %s.pluto_image WHERE uuid = $1::uuid`,
		PlutoInstance.DbSchema,
	)

	var fx, fy *float64
	err = tx.QueryRow(ctx, query, imageUuid).Scan(&fx, &fy)
	if err != nil {
		return nil, nil, err
	}

	return fx, fy, nil
}

// Deletes image + cache DB entries, returns filename to delete from disk
func DeleteImageTx(
	ctx context.Context,
	tx pgx.Tx,
	imageUuid string,
) (deletedFileName string, cacheRows int64, err error) {
	schema := PlutoInstance.DbSchema

	// Delete cache rows
	cacheRowsAffected, err := DeleteCacheTx(ctx, tx, imageUuid)
	if err != nil {
		return "", 0, err
	}

	// Delete image row
	imageQuery := fmt.Sprintf(`DELETE FROM %s.pluto_image WHERE uuid = $1::uuid RETURNING gen_file_name`, schema)
	err = tx.QueryRow(ctx, imageQuery, imageUuid).Scan(&deletedFileName)
	if err != nil {
		return "", cacheRowsAffected, err
	}

	return deletedFileName, cacheRowsAffected, nil
}

// Deletes cache DB entries, return number of affected rows
func DeleteCacheTx(
	ctx context.Context,
	tx pgx.Tx,
	imageUuid string,
) (deletedFilesCount int64, err error) {
	schema := PlutoInstance.DbSchema

	cacheQuery := fmt.Sprintf(`DELETE FROM %s.pluto_cache WHERE pluto_image_uuid = $1::uuid`, schema)
	cmdTag, err := tx.Exec(ctx, cacheQuery, imageUuid)
	if err != nil {
		return 0, err
	}

	return cmdTag.RowsAffected(), nil
}

func FloatPtrEqual(a, b *float64) bool {
	if a == nil && b == nil {
		return true
	}
	if a == nil || b == nil {
		return false
	}
	return *a == *b
}
