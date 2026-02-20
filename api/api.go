package api

import (
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
)

type ApiResponse[T any] struct {
	Service      string                 `json:"service"`
	Version      string                 `json:"version"`
	ResponseType string                 `json:"type"`
	Status       string                 `json:"status"`
	Timestamp    string                 `json:"timestamp"`
	Meta         map[string]interface{} `json:"meta,omitempty"`
	Data         T                      `json:"data,omitempty"`
	Message      string                 `json:"error,omitempty"`
}

func JSONSuccess[T any](gc *gin.Context, responseType string, data T, meta map[string]interface{}) {
	resp := ApiResponse[T]{
		Service:      "Pluto API",
		Version:      "1.0",
		ResponseType: responseType,
		Status:       "ok",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Meta:         meta,
		Data:         data,
	}
	gc.JSON(200, resp)
}

func JSONSuccessNoData(gc *gin.Context, responseType string) {
	resp := ApiResponse[any]{ // 'any' because there is no Data
		Service:      "Pluto API",
		Version:      "1.0",
		ResponseType: responseType,
		Status:       "ok",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
	}
	gc.JSON(200, resp)
}

func JSONSuccessMessage(gc *gin.Context, responseType string, statusCode int, message string) {
	resp := ApiResponse[any]{ // 'any' because there is no Data
		Service:      "Pluto API",
		Version:      "1.0",
		ResponseType: responseType,
		Status:       "error",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Message:      message,
	}
	gc.JSON(statusCode, resp)
}

func JSONError(gc *gin.Context, responseType string, statusCode int, errorMessage string) {
	resp := ApiResponse[any]{ // 'any' because there is no Data
		Service:      "Pluto API",
		Version:      "1.0",
		ResponseType: responseType,
		Status:       "error",
		Timestamp:    time.Now().UTC().Format(time.RFC3339),
		Message:      errorMessage,
	}
	gc.JSON(statusCode, resp)
}

func JSONDatabaseError(gc *gin.Context, responseType string) {
	JSONError(gc, responseType, http.StatusInternalServerError, "database error")
}

func JSONPayloadError(gc *gin.Context, responseType string) {
	JSONError(gc, responseType, http.StatusInternalServerError, "payload error")
}
