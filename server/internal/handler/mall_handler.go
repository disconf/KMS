// Package handler — 积分商城。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// MallHandler 积分商城。
type MallHandler struct{ mall *service.MallService }

// NewMallHandler 创建。
func NewMallHandler(svc *service.MallService) *MallHandler { return &MallHandler{mall: svc} }

// List 商城首页。
//
//	@Summary  商城商品列表
//	@Tags     mall
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /mall/items [get]
func (h *MallHandler) List(c *gin.Context) {
	userID := int64(0)
	if id, ok := authctx.Get(c); ok {
		userID = id.UserID
	}
	resp, err := h.mall.ListItems(c.Request.Context(), userID)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// Redeem 兑换。
//
//	@Summary  积分兑换
//	@Tags     mall
//	@Produce  json
//	@Param    id path int true "商品 ID"
//	@Success  200 {object} response.Body
//	@Router   /mall/items/{id}/redeem [post]
func (h *MallHandler) Redeem(c *gin.Context) {
	id, ok := PathInt64(c, "id")
	if !ok {
		return
	}
	uid, _ := authctx.Get(c)
	resp, err := h.mall.Redeem(c.Request.Context(), uid.UserID, id)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, resp)
}

// MyRedemptions 我的兑换记录。
//
//	@Summary  我的兑换记录
//	@Tags     mall
//	@Produce  json
//	@Success  200 {object} response.Body
//	@Router   /mall/redemptions [get]
func (h *MallHandler) MyRedemptions(c *gin.Context) {
	uid, _ := authctx.Get(c)
	limit := 50
	if v := c.Query("limit"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 && n <= 100 {
			limit = n
		}
	}
	items, err := h.mall.MyRedemptions(c.Request.Context(), uid.UserID, limit)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}
