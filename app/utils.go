package app

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
)

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
	// Generate 16 random bytes (128 bits)
	bytes := make([]byte, 16)
	if _, err := rand.Read(bytes); err != nil {
		return "", err
	}

	// Convert to hex string
	randomHex := hex.EncodeToString(bytes)

	// Extract original extension (e.g. ".jpg")
	ext := filepath.Ext(originalName)

	// Combine hex ID with original extension
	filename := fmt.Sprintf("%s%s", randomHex, ext)

	return filename, nil
}

func ParseAspectRatio(s string) (float64, error) {
	parts := strings.Split(s, "by")
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
