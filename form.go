package pluto

import (
	"fmt"
	_ "image/gif"
	_ "image/jpeg"
	_ "image/png"
	"strconv"
	"strings"

	"github.com/gin-gonic/gin"
	_ "github.com/lib/pq"
)

func getPostFormPtr(gc *gin.Context, field string) *string {
	if val, ok := gc.GetPostForm(field); ok {
		return &val
	}
	return nil
}

func getPostFormFloatPtr(gc *gin.Context, key string) (*float64, error) {
	val := strings.TrimSpace(gc.PostForm(key))
	if val == "" {
		return nil, nil
	}
	// allow both "," and "." as decimal separator
	val = strings.ReplaceAll(val, ",", ".")
	f, err := strconv.ParseFloat(val, 64)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %q", key, gc.PostForm(key))
	}
	return &f, nil
}

func getPostFormIntPtr(gc *gin.Context, field string) (*int, error) {
	valStr := gc.PostForm(field)
	if valStr == "" {
		return nil, nil
	}
	val, err := strconv.Atoi(valStr)
	if err != nil {
		return nil, fmt.Errorf("invalid %s: %v", field, err)
	}
	if val == 0 {
		return nil, nil // treat 0 as "not set"
	}
	return &val, nil
}
