package handlers

import (
	"net/http"

	"github.com/AnkitSinha0/HashVault/internal/dto"
	"github.com/AnkitSinha0/HashVault/internal/middleware"
	"github.com/AnkitSinha0/HashVault/internal/services"
	"github.com/gin-gonic/gin"
	"github.com/google/uuid"
)

type FileHandler struct {
	svc services.FileService
}

func NewFileHandler(svc services.FileService) *FileHandler {
	return &FileHandler{svc: svc}
}

// POST /api/v1/files/init-upload
func (h *FileHandler) InitUpload(c *gin.Context) {
	var req dto.InitUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	userID := middleware.MustGetUserID(c)
	resp, err := h.svc.InitUpload(c.Request.Context(), userID, req)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	respondOK(c, resp)
}

// POST /api/v1/files/confirm
func (h *FileHandler) Confirm(c *gin.Context) {
	var req dto.ConfirmUploadRequest
	if err := c.ShouldBindJSON(&req); err != nil {
		respondError(c, http.StatusBadRequest, err.Error())
		return
	}

	userID := middleware.MustGetUserID(c)
	resp, err := h.svc.ConfirmUpload(c.Request.Context(), userID, req)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	respondCreated(c, resp)
}

// GET /api/v1/files/:id/download
func (h *FileHandler) Download(c *gin.Context) {
	fileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid file id")
		return
	}

	userID := middleware.MustGetUserID(c)
	resp, err := h.svc.Download(c.Request.Context(), userID, fileID)
	if err != nil {
		respondServiceError(c, err)
		return
	}
	respondOK(c, resp)
}

// DELETE /api/v1/files/:id
func (h *FileHandler) Delete(c *gin.Context) {
	fileID, err := uuid.Parse(c.Param("id"))
	if err != nil {
		respondError(c, http.StatusBadRequest, "invalid file id")
		return
	}

	userID := middleware.MustGetUserID(c)
	if err := h.svc.Delete(c.Request.Context(), userID, fileID); err != nil {
		respondServiceError(c, err)
		return
	}
	c.Status(http.StatusNoContent)
}
