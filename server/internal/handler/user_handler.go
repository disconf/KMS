package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// UserHandler 个人资料相关接口。
type UserHandler struct {
	auth *service.AuthService
}

// NewUserHandler 创建 UserHandler。
func NewUserHandler(auth *service.AuthService) *UserHandler {
	return &UserHandler{auth: auth}
}

// Me 获取当前登录用户资料。
//
//	@Summary  获取当前用户资料
//	@Tags     user
//	@Security BearerAuth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /me [get]
func (h *UserHandler) Me(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	profile, err := h.auth.Profile(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, profile)
}

// UpdateMe 更新当前用户资料，仅更新传入的字段。
//
//	@Summary  更新当前用户资料
//	@Tags     user
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.UpdateProfileReq true "待更新字段"
//	@Success  200 {object} response.Body
//	@Router   /me [put]
func (h *UserHandler) UpdateMe(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.UpdateProfileReq
	if !BindJSON(c, &req) {
		return
	}
	profile, err := h.auth.UpdateProfile(c.Request.Context(), userID, req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, profile)
}

// ChangePassword 修改当前账号的登录口令。
//
//	@Summary  修改登录密码
//	@Tags     user
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.ChangePasswordReq true "原密码与新密码"
//	@Success  200 {object} response.Body
//	@Router   /me/password [post]
func (h *UserHandler) ChangePassword(c *gin.Context) {
	userID, err := authctx.RequireUserID(c)
	if err != nil {
		response.Fail(c, err)
		return
	}
	var req dto.ChangePasswordReq
	if !BindJSON(c, &req) {
		return
	}
	if err := h.auth.ChangePassword(c.Request.Context(), userID, req); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"changed": true})
}
