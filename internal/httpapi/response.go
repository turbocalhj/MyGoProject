// Package httpapi exposes the short link service over HTTP.
package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

const requestIDKey = "httpapi.request_id"

// Client-facing error codes.
const (
	codeInvalidRequest     = "invalid_request"
	codeInvalidPagination  = "invalid_pagination"
	codeLinkNotFound       = "link_not_found"
	codeLinkExpired        = "link_expired"
	codeLinkDisabled       = "link_disabled"
	codeAliasTaken         = "alias_taken"
	codeStorageUnavailable = "storage_unavailable"
	codeRateLimited        = "rate_limited"
	codeInternal           = "internal_error"
)

type errorBody struct {
	Code      string `json:"code"`
	Message   string `json:"message"`
	RequestID string `json:"request_id"`
}

type errorResponse struct {
	Error errorBody `json:"error"`
}

// writeError renders the unified error envelope and aborts the chain.
func writeError(c *gin.Context, status int, code, message string) {
	c.AbortWithStatusJSON(status, errorResponse{Error: errorBody{
		Code:      code,
		Message:   message,
		RequestID: requestID(c),
	}})
}

func requestID(c *gin.Context) string {
	if v, ok := c.Get(requestIDKey); ok {
		if id, ok := v.(string); ok {
			return id
		}
	}
	return ""
}

func statusOK(c *gin.Context) {
	c.JSON(http.StatusOK, gin.H{"status": "ok"})
}
