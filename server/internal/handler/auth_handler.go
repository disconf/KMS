package handler

import (
	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AuthHandler 认证与账号相关接口。
type AuthHandler struct {
	auth *service.AuthService
}

// NewAuthHandler 创建 AuthHandler。
func NewAuthHandler(auth *service.AuthService) *AuthHandler {
	return &AuthHandler{auth: auth}
}

// SendCode 发送短信验证码。
//
//	@Summary  发送短信验证码
//	@Tags     auth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.SendSMSCodeReq true "手机号与场景"
//	@Success  200 {object} response.Body
//	@Router   /auth/sms/send [post]
func (h *AuthHandler) SendCode(c *gin.Context) {
	var req dto.SendSMSCodeReq
	if !BindJSON(c, &req) {
		return
	}
	resp, err := h.auth.SendCode(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// LoginBySMS 手机号验证码登录，未注册则自动创建账号。
//
//	@Summary  手机号验证码登录
//	@Tags     auth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.LoginSMSReq true "手机号与验证码"
//	@Success  200 {object} response.Body
//	@Router   /auth/login/sms [post]
func (h *AuthHandler) LoginBySMS(c *gin.Context) {
	var req dto.LoginSMSReq
	if !BindJSON(c, &req) {
		return
	}
	resp, err := h.auth.LoginBySMS(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// LoginByPassword 手机号 + 密码登录，仅对已设置密码的账号生效。
//
//	@Summary  手机号密码登录
//	@Tags     auth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.LoginPasswordReq true "手机号与密码"
//	@Success  200 {object} response.Body
//	@Router   /auth/login/password [post]
func (h *AuthHandler) LoginByPassword(c *gin.Context) {
	var req dto.LoginPasswordReq
	if !BindJSON(c, &req) {
		return
	}
	resp, err := h.auth.LoginByPassword(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// SendEmailCode 发送邮箱验证码。
//
//	@Summary  发送邮箱验证码
//	@Tags     auth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.SendEmailCodeReq true "邮箱与场景"
//	@Success  200 {object} response.Body
//	@Router   /auth/email/send [post]
func (h *AuthHandler) SendEmailCode(c *gin.Context) {
	var req dto.SendEmailCodeReq
	if !BindJSON(c, &req) {
		return
	}
	resp, err := h.auth.SendEmailCode(c.Request.Context(), req)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// RegisterByEmail 邮箱注册，注册出来的账号默认为只读。
//
//	@Summary  邮箱注册
//	@Tags     auth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.RegisterEmailReq true "邮箱、验证码与密码"
//	@Success  200 {object} response.Body
//	@Router   /auth/register [post]
func (h *AuthHandler) RegisterByEmail(c *gin.Context) {
	var req dto.RegisterEmailReq
	if !BindJSON(c, &req) {
		return
	}
	resp, err := h.auth.RegisterByEmail(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// LoginByEmail 邮箱 + 密码登录。
//
//	@Summary  邮箱密码登录
//	@Tags     auth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.LoginEmailReq true "邮箱与密码"
//	@Success  200 {object} response.Body
//	@Router   /auth/login/email [post]
func (h *AuthHandler) LoginByEmail(c *gin.Context) {
	var req dto.LoginEmailReq
	if !BindJSON(c, &req) {
		return
	}
	resp, err := h.auth.LoginByEmail(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// LoginByEmailCode 邮箱 + 验证码登录。
//
//	@Summary  邮箱验证码登录
//	@Tags     auth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.LoginEmailCodeReq true "邮箱与验证码"
//	@Success  200 {object} response.Body
//	@Router   /auth/login/email-code [post]
func (h *AuthHandler) LoginByEmailCode(c *gin.Context) {
	var req dto.LoginEmailCodeReq
	if !BindJSON(c, &req) {
		return
	}
	resp, err := h.auth.LoginByEmailCode(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// WechatAuthURL 获取微信授权跳转地址。//
//
//	@Summary  获取微信授权地址
//	@Tags     auth
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /auth/wechat/url [get]
func (h *AuthHandler) WechatAuthURL(c *gin.Context) {
	resp, err := h.auth.WechatAuthURL(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// LoginByWechat 微信授权登录。
//
//	@Summary  微信授权登录
//	@Tags     auth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.LoginWechatReq true "授权 code"
//	@Success  200 {object} response.Body
//	@Router   /auth/login/wechat [post]
func (h *AuthHandler) LoginByWechat(c *gin.Context) {
	var req dto.LoginWechatReq
	if !BindJSON(c, &req) {
		return
	}
	resp, err := h.auth.LoginByWechat(c.Request.Context(), req, c.ClientIP())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// Refresh 刷新令牌。
//
//	@Summary  刷新访问令牌
//	@Tags     auth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.RefreshReq true "刷新令牌"
//	@Success  200 {object} response.Body
//	@Router   /auth/refresh [post]
func (h *AuthHandler) Refresh(c *gin.Context) {
	var req dto.RefreshReq
	if !BindJSON(c, &req) {
		return
	}
	resp, err := h.auth.Refresh(c.Request.Context(), req.RefreshToken)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// Logout 登出，吊销当前访问令牌与（可选的）刷新令牌。
//
//	@Summary  登出
//	@Tags     auth
//	@Security BearerAuth
//	@Accept   json
//	@Produce  json
//	@Param    body body dto.LogoutReq false "刷新令牌（可选）"
//	@Success  200 {object} response.Body
//	@Router   /auth/logout [post]
func (h *AuthHandler) Logout(c *gin.Context) {
	var req dto.LogoutReq
	// 登出接口对空 body 宽容处理：没有 body 也要能登出。
	if c.Request.ContentLength > 0 {
		if !BindJSON(c, &req) {
			return
		}
	}

	identity, ok := authctx.Get(c)
	if !ok {
		response.Fail(c, errcode.ErrUnauthorized)
		return
	}
	if err := h.auth.Logout(c.Request.Context(), identity, req.RefreshToken); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"loggedOut": true})
}
