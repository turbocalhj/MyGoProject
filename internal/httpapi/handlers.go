package httpapi

import (
	"context"
	"errors"
	"log/slog"
	"net/http"
	"strconv"
	"time"

	"github.com/gin-gonic/gin"

	"github.com/turbocalhj/MyGoProject/internal/shortener"
	"github.com/turbocalhj/MyGoProject/internal/store"
)

type handlers struct {
	service *shortener.Service
	repo    store.Repository
	logger  *slog.Logger
}

func (h *handlers) health(c *gin.Context) { statusOK(c) }

func (h *handlers) ready(c *gin.Context) {
	ctx, cancel := context.WithTimeout(c.Request.Context(), 2*time.Second)
	defer cancel()
	if err := h.repo.Ping(ctx); err != nil {
		c.JSON(http.StatusServiceUnavailable, gin.H{"status": "unavailable"})
		return
	}
	statusOK(c)
}

func (h *handlers) createLink(c *gin.Context) {
	var req createLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, codeInvalidRequest, "request body must be a JSON object with a url field")
		return
	}

	link, err := h.service.Create(c.Request.Context(), shortener.CreateInput{
		TargetURL: req.URL,
		Alias:     req.Alias,
		ExpiresAt: req.ExpiresAt,
	})
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusCreated, newLinkResponse(h.service, link))
}

func (h *handlers) listLinks(c *gin.Context) {
	page, err := queryInt(c, "page", 1)
	if err != nil {
		writeError(c, http.StatusBadRequest, codeInvalidPagination, "page must be an integer")
		return
	}
	pageSize, err := queryInt(c, "page_size", 20)
	if err != nil {
		writeError(c, http.StatusBadRequest, codeInvalidPagination, "page_size must be an integer")
		return
	}

	result, err := h.service.List(c.Request.Context(), page, pageSize, c.Query("q"))
	if err != nil {
		h.fail(c, err)
		return
	}

	items := make([]linkResponse, 0, len(result.Items))
	for _, link := range result.Items {
		items = append(items, newLinkResponse(h.service, link))
	}
	c.JSON(http.StatusOK, listResponse{Items: items, Page: page, PageSize: pageSize, Total: result.Total})
}

func (h *handlers) getLink(c *gin.Context) {
	link, err := h.service.Get(c.Request.Context(), c.Param("code"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, newLinkResponse(h.service, link))
}

func (h *handlers) updateLink(c *gin.Context) {
	var req updateLinkRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		writeError(c, http.StatusBadRequest, codeInvalidRequest, "request body must be a JSON object")
		return
	}
	in, err := req.toInput()
	if err != nil {
		writeError(c, http.StatusBadRequest, shortener.CodeInvalidExpires, err.Error())
		return
	}

	link, err := h.service.Update(c.Request.Context(), c.Param("code"), in)
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, newLinkResponse(h.service, link))
}

func (h *handlers) deleteLink(c *gin.Context) {
	if err := h.service.Delete(c.Request.Context(), c.Param("code")); err != nil {
		h.fail(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}

func (h *handlers) linkStats(c *gin.Context) {
	link, err := h.service.Get(c.Request.Context(), c.Param("code"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.JSON(http.StatusOK, statsResponse{
		Code:         link.Code,
		Hits:         link.Hits,
		LastAccessAt: link.LastAccessAt,
		CreatedAt:    link.CreatedAt,
		ExpiresAt:    link.ExpiresAt,
	})
}

func (h *handlers) redirect(c *gin.Context) {
	link, err := h.service.Resolve(c.Request.Context(), c.Param("code"))
	if err != nil {
		h.fail(c, err)
		return
	}
	c.Header("Cache-Control", "no-store")
	c.Header("Location", link.TargetURL)
	c.Status(http.StatusFound)
}

// fail maps domain errors onto the unified error envelope.
func (h *handlers) fail(c *gin.Context, err error) {
	var vErr *shortener.ValidationError
	switch {
	case errors.As(err, &vErr):
		writeError(c, http.StatusBadRequest, vErr.Code, vErr.Message)
	case errors.Is(err, shortener.ErrNotFound):
		writeError(c, http.StatusNotFound, codeLinkNotFound, "link not found")
	case errors.Is(err, shortener.ErrExpired):
		writeError(c, http.StatusGone, codeLinkExpired, "link has expired")
	case errors.Is(err, shortener.ErrDisabled):
		writeError(c, http.StatusForbidden, codeLinkDisabled, "link is disabled")
	case errors.Is(err, shortener.ErrConflict):
		writeError(c, http.StatusConflict, codeAliasTaken, "alias is already taken")
	case errors.Is(err, shortener.ErrStorage):
		h.logger.Error("storage failure", "error", err, "request_id", requestID(c))
		writeError(c, http.StatusServiceUnavailable, codeStorageUnavailable, "storage is unavailable")
	default:
		h.logger.Error("unexpected failure", "error", err, "request_id", requestID(c))
		writeError(c, http.StatusInternalServerError, codeInternal, "internal server error")
	}
}

func queryInt(c *gin.Context, key string, fallback int) (int, error) {
	raw := c.Query(key)
	if raw == "" {
		return fallback, nil
	}
	return strconv.Atoi(raw)
}
