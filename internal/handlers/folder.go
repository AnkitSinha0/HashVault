package handlers

import (
	"net/http"

	"github.com/AnkitSinha0/HashVault/internal/dto"
	"github.com/AnkitSinha0/HashVault/internal/middleware"
	"github.com/AnkitSinha0/HashVault/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type FolderHandler struct {
	svc services.FolderService
}

func NewFolderHandler(svc services.FolderService) *FolderHandler {
	return &FolderHandler{svc: svc}
}

// POST /api/v1/folders
func (h *FolderHandler) Create(c *gin.Context) {
	var req dto.CreateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	userID := middleware.MustGetUserID(c)
	resp, err := h.svc.Create(c.Request.Context(), userID, req)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	respondCreated(c, resp)
}

// GET /api/v1/folders
func (h *FolderHandler) ListRoot(c *gin.Context) {
	userID := middleware.MustGetUserID(c)
	folders, err := h.svc.ListRoot(c.Request.Context(), userID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	respondOK(c, folders)
}

// GET /api/v1/folders/:id
func (h *FolderHandler) ListChildren(c *gin.Context) {
	folderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid folder id")
		return
	}

	userID := middleware.MustGetUserID(c)
	folders, err := h.svc.ListChildren(c.Request.Context(), userID, folderID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	respondOK(c, folders)
}

// PATCH /api/v1/folders/:id
func (h *FolderHandler) Rename(c *gin.Context) {
	folderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid folder id")
		return
	}

	var req dto.UpdateFolderRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	userID := middleware.MustGetUserID(c)
	resp, err := h.svc.Rename(c.Request.Context(), userID, folderID, req)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	respondOK(c, resp)
}

// DELETE /api/v1/folders/:id
func (h *FolderHandler) Delete(c *gin.Context) {
	folderID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid folder id")
		return
	}

	userID := middleware.MustGetUserID(c)
	if err := h.svc.Delete(c.Request.Context(), userID, folderID); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
