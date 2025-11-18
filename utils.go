package pluto

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"image"
	"net/http"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/disintegration/imaging"
	"github.com/gin-gonic/gin"
	"github.com/nfnt/resize"
)

type UrlParamType int

const (
	Boolean UrlParamType = iota
	Int
	Float
	Rational
	String
)

type UrlQueryParam struct {
	Name  string
	Type  UrlParamType
	Exist bool
	Err   error
	Bool  bool
	Int64 int64
	Float float64
	Value string
}

// Get the raw query parameter
func GetUrlQueryParam(gc *gin.Context, paramName string, paramType UrlParamType) UrlQueryParam {
	value, exists := gc.GetQuery(paramName)
	p := UrlQueryParam{
		Name:  paramName,
		Type:  paramType,
		Exist: exists,
		Value: value,
	}
	switch paramType {
	case Boolean:
		p.Bool = exists
	case Int:
		p.Int64, p.Err = strconv.ParseInt(p.Value, 10, 64)
	case Float:
		p.Float, p.Err = strconv.ParseFloat(p.Value, 64)
	case Rational:
		p.Float, p.Err = ParseAspectRatio(value)
	case String:
	}
	return p
}

func (p UrlQueryParam) Println() {
	if p.Exist {
		fmt.Print(p.Name, ": ")
		if p.Err != nil {
			fmt.Println("error: ", p.Err.Error())
		} else {
			switch p.Type {
			case Boolean:
				fmt.Println(p.Bool)
			case Int:
				fmt.Println(p.Int64)
			case Float:
				fmt.Println(p.Float)
			case Rational:
				fmt.Println(p.Float)
			case String:
				fmt.Println(p.Value)
			}
		}
	}
}

func (p UrlQueryParam) ValueOr(def string) string {
	if p.Exist {
		return p.Value
	}
	return def
}

func (p UrlQueryParam) Int64Or(def int64) int64 {
	if p.Exist {
		return p.Int64
	}
	return def
}

func (p UrlQueryParam) IntOr(def int) int {
	if p.Exist {
		return int(p.Int64)
	}
	return def
}

func (p UrlQueryParam) Int() int {
	if p.Exist {
		return int(p.Int64)
	}
	return 0
}

func SetInt(p *UrlQueryParam, value int) {
	if p != nil {
		p.Exist = true
		p.Type = Int
		p.Int64 = int64(value)
	}
}

func SetInt64(p *UrlQueryParam, value int64) {
	if p != nil {
		p.Exist = true
		p.Type = Int
		p.Int64 = value
	}
}

// GenerateImageFilename returns a securely generated random filename
// that preserves the original file's extension.
//
// It creates a 128-bit (16-byte) cryptographically secure random identifier,
// encodes it as a 32-character hexadecimal string, and appends the original
// file extension (e.g. ".jpg", ".png") from the input filename.
//
// This function is suitable for generating unique, unpredictable image
// filenames for uploaded files.
//
// Example:
//
//	"cat.jpg" => "f3ab7c54c8a44f01bd1182d4a57c121a.jpg"
//
// Parameters:
//
//	originalName - the original filename, from which the extension will be preserved.
//
// Returns:
//   - A string containing the generated filename (randomHex + original extension).
//   - An error if the secure random number generation fails.
func GenerateImageFilename(originalName string) (string, error) {
	bytes := make([]byte, 16) // Generate 16 random bytes (128 bits)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	randomHex := hex.EncodeToString(bytes)          // Convert to hex string
	ext := filepath.Ext(originalName)               // Extract original extension
	filename := fmt.Sprintf("%s%s", randomHex, ext) // Combine hex ID with original extension

	return filename, nil
}

func ParseAspectRatio(s string) (float64, error) {
	parts := strings.Split(s, ":")
	if len(parts) != 2 {
		return 0, fmt.Errorf("invalid ratio")
	}
	w, err1 := strconv.ParseFloat(parts[0], 64)
	h, err2 := strconv.ParseFloat(parts[1], 64)
	if err1 != nil || err2 != nil || h == 0 {
		return 0, fmt.Errorf("invalid ratio")
	}
	return w / h, nil
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

// CropToAspectAdvanced crops or resizes an image based on mode and parameters.
// - aspectRatio: target width / height (used only if one dim is missing)
// - focusX/focusY: normalized crop center [0.0–1.0]
// - mode: "cover", "contain"
// - width / height: target pixel dimensions (optional, 0 = not specified)
func CropToAspectAdvanced(
	img image.Image,
	mode string,
	aspectRatio float64,
	focusX, focusY float64,
	width, height int,
) image.Image {
	srcBounds := img.Bounds()
	srcW := srcBounds.Dx()
	srcH := srcBounds.Dy()
	srcRatio := float64(srcW) / float64(srcH)

	// Clamp focus values
	focusX = clamp01(focusX)
	focusY = clamp01(focusY)

	// Determine target dimensions
	var targetW, targetH int

	switch {
	case width > 0 && height > 0:
		targetW = width
		targetH = height
	case width > 0 && aspectRatio > 0:
		targetW = width
		targetH = int(float64(width) / aspectRatio)
	case height > 0 && aspectRatio > 0:
		targetH = height
		targetW = int(float64(height) * aspectRatio)
	case aspectRatio > 0:
		// Fallback: base on source height
		targetH = int(srcH)
		targetW = int(float64(targetH) * aspectRatio)
	default:
		// Nothing to do
		fmt.Println("nothing")
		return img
	}

	targetRatio := float64(targetW) / float64(targetH)

	switch mode {
	case "contain":
		fmt.Println("contain")
		// Resize to fit into target box, preserving aspect ratio
		var newW, newH int
		if srcRatio > targetRatio {
			// Fit by width
			newW = int(targetW)
			newH = int(float64(newW) / srcRatio)
		} else {
			// Fit by height
			newH = int(targetH)
			newW = int(float64(newH) * srcRatio)
		}

		return imaging.Resize(img, newW, newH, imaging.Lanczos)

	default: // "center" or focus-based crop
		// Crop to match target aspect ratio
		var cropW, cropH int
		if srcRatio > targetRatio {
			cropH = srcH
			cropW = int(float64(cropH) * targetRatio)
		} else {
			cropW = srcW
			cropH = int(float64(cropW) / targetRatio)
		}

		// Crop centered on focus
		x0 := int(float64(srcW-cropW) * focusX)
		y0 := int(float64(srcH-cropH) * focusY)
		x0 = clampInt(x0, 0, srcW-cropW)
		y0 = clampInt(y0, 0, srcH-cropH)

		rect := image.Rect(x0, y0, x0+cropW, y0+cropH)
		cropped := cropImage(img, rect)

		fmt.Printf("srcW: %d, srcH: %d, srcRatio: %f\n", srcW, srcH, srcRatio)
		fmt.Printf("cropW: %d, cropH: %d\n", cropW, cropH)
		fmt.Println("rect:", rect)
		fmt.Printf("width: %d, height: %d\n", width, height)

		// Final resize if needed
		switch {
		case width > 0 && height > 0:
			return imaging.Resize(cropped, width, height, imaging.Lanczos)
		case width > 0:
			return imaging.Resize(cropped, width, 0, imaging.Lanczos)
		case height > 0:
			return imaging.Resize(cropped, 0, height, imaging.Lanczos)
		default:
			return cropped
		}
	}

	return img
}

func clamp01(v float64) float64 {
	if v < 0.0 {
		return 0.0
	}
	if v > 1.0 {
		return 1.0
	}
	return v
}

func clampInt(value, min, max int) int {
	if value < min {
		return min
	}
	if value > max {
		return max
	}
	return value
}

func cropImage(img image.Image, rect image.Rectangle) image.Image {
	return img.(interface {
		SubImage(r image.Rectangle) image.Image
	}).SubImage(rect)
}

// ParamInt extracts a URL path parameter as an integer.
// If conversion fails, it writes a 400 JSON error and returns (0, false).
func ParamInt(gc *gin.Context, name string) (int, bool) {
	paramStr := gc.Param(name)
	val, err := strconv.Atoi(paramStr)
	if err != nil {
		gc.JSON(http.StatusBadRequest, gin.H{
			"error": "invalid " + name + " parameter",
		})
		return 0, false
	}
	return val, true
}
