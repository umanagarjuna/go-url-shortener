package handler

import (
	"github.com/gin-gonic/gin"
	"go.uber.org/zap"
	"net/http"
	"strconv"

	"github.com/umanagarjuna/go-url-shortener/internal/url/domain"
	"github.com/umanagarjuna/go-url-shortener/internal/url/service"
)

type HTTPHandler struct {
	service *service.URLService
	logger  *zap.Logger
}

func NewHTTPHandler(service *service.URLService, logger *zap.Logger) *HTTPHandler {
	return &HTTPHandler{
		service: service,
		logger:  logger,
	}
}

func (h *HTTPHandler) RegisterRoutes(router *gin.Engine) {
	api := router.Group("/api/v1")
	{
		api.POST("/urls", h.CreateURL)
		api.GET("/urls/:shortCode", h.GetURL)
		api.DELETE("/urls/:shortCode", h.DeleteURL)
		api.GET("/users/:userId/urls", h.GetUserURLs)
	}

	// Metrics endpoint (NEW)
	router.GET("/metrics", h.GetMetrics)

	// Redirect endpoint
	router.GET("/:shortCode", h.RedirectURL)
}

func (h *HTTPHandler) CreateURL(c *gin.Context) {
	var req domain.CreateURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		h.logger.Error("Invalid request", zap.Error(err))
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Additional validation (optional)
	if req.UserID <= 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "user_id must be a positive integer"})
		return
	}

	resp, err := h.service.CreateURL(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("Failed to create URL",
			zap.Error(err),
			zap.String("url", req.URL),
			zap.Int64("user_id", req.UserID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, resp)
}

func (h *HTTPHandler) GetMetrics(c *gin.Context) {
	// This would work if you pass metrics to HTTPHandler
	// For now, return a simple response
	c.JSON(http.StatusOK, gin.H{
		"status": "metrics endpoint - implement based on your metrics collector",
	})
}

func (h *HTTPHandler) GetURL(c *gin.Context) {
	shortCode := c.Param("shortCode")

	// Debug logging
	h.logger.Info("GetURL called",
		zap.String("shortCode", shortCode),
		zap.String("path", c.Request.URL.Path))

	if shortCode == "" {
		h.logger.Error("Short code is empty")
		c.JSON(http.StatusBadRequest, gin.H{"error": "short code is required"})
		return
	}

	response, err := h.service.GetURL(c.Request.Context(), shortCode)
	if err != nil {
		h.logger.Error("Failed to get URL",
			zap.Error(err), zap.String("short_code", shortCode))
		c.JSON(http.StatusInternalServerError, gin.H{"error": "internal server error"})
		return
	}

	if response == nil {
		h.logger.Warn("URL not found", zap.String("short_code", shortCode))
		c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
		return
	}

	c.JSON(http.StatusOK, response)
}

func (h *HTTPHandler) RedirectURL(c *gin.Context) {
	shortCode := c.Param("shortCode")

	// Debug logging (you can remove this later)
	h.logger.Info("RedirectURL called",
		zap.String("path", c.Request.URL.Path),
		zap.String("shortCode", shortCode))

	if shortCode == "" {
		h.logger.Error("Short code is empty")
		c.JSON(http.StatusBadRequest, gin.H{"error": "short code is required"})
		return
	}

	// Extract analytics data
	userAgent := c.Request.UserAgent()
	referrer := c.Request.Referer()
	clientIP := c.ClientIP()

	// Get URL and increment click count
	url, err := h.service.GetURLAndIncrementClick(c.Request.Context(), shortCode, userAgent, clientIP, referrer)
	if err != nil {
		h.logger.Error("Failed to get URL for redirect",
			zap.Error(err), zap.String("short_code", shortCode))
		c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
		return
	}

	if url == nil {
		h.logger.Warn("URL not found", zap.String("short_code", shortCode))
		c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
		return
	}

	h.logger.Info("Redirecting URL",
		zap.String("short_code", shortCode),
		zap.String("original_url", url.OriginalURL),
		zap.String("client_ip", clientIP))

	// Perform redirect
	c.Redirect(http.StatusMovedPermanently, url.OriginalURL)
}

func (h *HTTPHandler) DeleteURL(c *gin.Context) {
	shortCode := c.Param("shortCode")
	if shortCode == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "short_code is required"})
		return
	}

	err := h.service.DeleteURL(c.Request.Context(), shortCode)
	if err != nil {
		h.logger.Error("Failed to delete URL",
			zap.Error(err), zap.String("short_code", shortCode))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{"message": "URL deleted successfully"})
}

func (h *HTTPHandler) GetUserURLs(c *gin.Context) {
	userIDStr := c.Param("userID")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid user_id"})
		return
	}

	// Parse pagination parameters
	limitStr := c.DefaultQuery("limit", "10")
	offsetStr := c.DefaultQuery("offset", "0")

	limit, err := strconv.Atoi(limitStr)
	if err != nil || limit <= 0 {
		limit = 10
	}

	offset, err := strconv.Atoi(offsetStr)
	if err != nil || offset < 0 {
		offset = 0
	}

	urls, err := h.service.GetUserURLs(c.Request.Context(), userID, limit, offset)
	if err != nil {
		h.logger.Error("Failed to get user URLs",
			zap.Error(err), zap.Int64("user_id", userID))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"urls":   urls,
		"limit":  limit,
		"offset": offset,
		"count":  len(urls),
	})
}
