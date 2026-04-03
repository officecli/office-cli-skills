package httpapi

import (
	"net/http"

	"github.com/gin-gonic/gin"
)

type Envelope struct {
	Data      any    `json:"data,omitempty"`
	Error     string `json:"error,omitempty"`
	RequestID string `json:"request_id,omitempty"`
}

func JSON(c *gin.Context, status int, data any) {
	c.JSON(status, Envelope{
		Data:      data,
		RequestID: RequestID(c),
	})
}

func Error(c *gin.Context, status int, message string) {
	c.JSON(status, Envelope{
		Error:     message,
		RequestID: RequestID(c),
	})
}

func AbortUnauthorized(c *gin.Context) {
	Error(c, http.StatusUnauthorized, "unauthorized")
	c.Abort()
}
