// Package handler — 开放 API 密钥管理（用户侧）。
package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// APIKeyHandler 密钥管理。
type APIKeyHandler struct{ keys *service.ApiKeyService }

// NewAPIKeyHandler 创建。
func NewAPIKeyHandler(svc *service.ApiKeyService) *APIKeyHandler { return &APIKeyHandler{keys: svc} }

// List 我的密钥。
//
//	@Summary  我的 API 密钥列表
//	@Tags     apikeys
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /me/apikeys [get]
func (h *APIKeyHandler) List(c *gin.Context) {
	uid, _ := authctx.Get(c)
	keys, err := h.keys.List(c.Request.Context(), uid.UserID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{
		"keys":     keys,
		"maxKeys":  10,
		"endpoint": "/open/v1",
	})
}

// Create 新建密钥（明文只在本响应出现一次）。
//
//	@Summary  创建 API 密钥
//	@Tags     apikeys
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /me/apikeys [post]
func (h *APIKeyHandler) Create(c *gin.Context) {
	var in struct {
		Name string `json:"name"`
	}
	if !BindJSON(c, &in) {
		return
	}
	uid, _ := authctx.Get(c)
	created, err := h.keys.Create(c.Request.Context(), uid.UserID, in.Name)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, created)
}

// Revoke 吊销。
//
//	@Summary  吊销 API 密钥
//	@Tags     apikeys
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /me/apikeys/{id}/revoke [post]
func (h *APIKeyHandler) Revoke(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	uid, _ := authctx.Get(c)
	if err := h.keys.Revoke(c.Request.Context(), uid.UserID, id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true})
}
