package httpapi

import (
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

const ContextAdminSessionID = "admin_session_id"
const ContextAppSessionID = "app_session_id"
const ContextAppUserID = "app_user_id"
const ContextRequestID = "request_id"

func RequestIDMiddleware() gin.HandlerFunc {
	return func(c *gin.Context) {
		requestID := c.GetHeader("X-Request-Id")
		if requestID == "" {
			requestID = uuid.NewString()
		}
		c.Set(ContextRequestID, requestID)
		c.Header("X-Request-Id", requestID)
		c.Next()
	}
}

func RequestID(c *gin.Context) string {
	if c == nil {
		return ""
	}
	if requestID, ok := c.Get(ContextRequestID); ok {
		if value, ok := requestID.(string); ok {
			return value
		}
	}
	return ""
}

func RequireAdmin(sessionResolver func(cookieValue string) (string, error), cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(cookieName)
		if err != nil {
			AbortUnauthorized(c)
			return
		}
		sessionID, err := sessionResolver(raw)
		if err != nil || sessionID == "" {
			AbortUnauthorized(c)
			return
		}
		c.Set(ContextAdminSessionID, sessionID)
		c.Next()
	}
}

func RequireAppUser(sessionResolver func(cookieValue string) (uint64, string, error), cookieName string) gin.HandlerFunc {
	return func(c *gin.Context) {
		raw, err := c.Cookie(cookieName)
		if err != nil {
			AbortUnauthorized(c)
			return
		}
		userID, sessionID, err := sessionResolver(raw)
		if err != nil || userID == 0 || sessionID == "" {
			AbortUnauthorized(c)
			return
		}
		c.Set(ContextAppUserID, userID)
		c.Set(ContextAppSessionID, sessionID)
		c.Next()
	}
}
