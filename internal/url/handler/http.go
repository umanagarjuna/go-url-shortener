package handler

import (
	"net/http"
	"strconv"

	"github.com/gin-gonic/gin"
	"go.uber.org/zap"

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
		api.GET("/urls/:code", h.GetURL)
		api.DELETE("/urls/:code", h.DeleteURL)
		api.GET("/users/:userId/urls", h.GetUserURLs)
	}

	// Redirect endpoint
	router.GET("/:code", h.RedirectURL)
}

func (h *HTTPHandler) CreateURL(c *gin.Context) {
	var req domain.CreateURLRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": err.Error()})
		return
	}

	// Get user ID from context (set by auth middleware)
	if userID, exists := c.Get("user_id"); exists {
		if id, ok := userID.(int64); ok {
			req.UserID = &id
		}
	}

	resp, err := h.service.CreateURL(c.Request.Context(), &req)
	if err != nil {
		h.logger.Error("Failed to create URL",
			zap.Error(err), zap.String("url", req.URL))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusCreated, resp)
}

func (h *HTTPHandler) GetURL(c *gin.Context) {
	code := c.Param("code")

	resp, err := h.service.GetURL(c.Request.Context(), code)
	if err != nil {
		h.logger.Error("Failed to get URL",
			zap.Error(err), zap.String("code", code))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	if resp == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
		return
	}

	c.JSON(http.StatusOK, resp)
}

func (h *HTTPHandler) RedirectURL(c *gin.Context) {
	code := c.Param("code")

	// Create click event
	clickEvent := &domain.ClickEvent{
		IP:        c.ClientIP(),
		UserAgent: c.GetHeader("User-Agent"),
		Referrer:  c.GetHeader("Referer"),
	}

	originalURL, err := h.service.RedirectURL(c.Request.Context(),
		code, clickEvent)
	if err != nil {
		h.logger.Error("Failed to redirect URL",
			zap.Error(err), zap.String("code", code))
		c.JSON(http.StatusNotFound, gin.H{"error": "URL not found"})
		return
	}

	c.Redirect(http.StatusMovedPermanently, originalURL)
}

func (h *HTTPHandler) DeleteURL(c *gin.Context) {
	code := c.Param("code")

	// Get user ID from context
	var userID *int64
	if id, exists := c.Get("user_id"); exists {
		if userIDVal, ok := id.(int64); ok {
			userID = &userIDVal
		}
	}

	err := h.service.DeleteURL(c.Request.Context(), code, userID)
	if err != nil {
		h.logger.Error("Failed to delete URL",
			zap.Error(err), zap.String("code", code))
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusNoContent, nil)
}

func (h *HTTPHandler) GetUserURLs(c *gin.Context) {
	userIDStr := c.Param("userId")
	userID, err := strconv.ParseInt(userIDStr, 10, 64)
	if err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid user ID"})
		return
	}

	// Pagination
	limit, _ := strconv.Atoi(c.DefaultQuery("limit", "20"))
	offset, _ := strconv.Atoi(c.DefaultQuery("offset", "0"))

	urls, err := h.service.GetUserURLs(c.Request.Context(),
		userID, limit, offset)
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
	})
}
