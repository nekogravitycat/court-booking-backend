package http

import "github.com/gin-gonic/gin"

// RegisterRoutes registers file routes
func RegisterRoutes(r gin.IRouter, handler *Handler, authMiddleware gin.HandlerFunc) {
	group := r.Group("/files")
	group.Use(authMiddleware)

	group.GET("/:id", handler.ServeFile)
	group.GET("/:id/thumbnail", handler.ServeThumbnail)
}
