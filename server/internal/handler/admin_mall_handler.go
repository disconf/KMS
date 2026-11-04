// Package handler — 积分商城后台管理。
package handler

import (
	"strconv"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminMallHandler 商城后台。
type AdminMallHandler struct{ mall *service.MallService }

// NewAdminMallHandler 创建。
func NewAdminMallHandler(svc *service.MallService) *AdminMallHandler {
	return &AdminMallHandler{mall: svc}
}

// ListItems 商品列表（含下架）。
func (h *AdminMallHandler) ListItems(c *gin.Context) {
	items, err := h.mall.AdminListItems(c.Request.Context())
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"items": items})
}

// CreateItem 新建商品。
func (h *AdminMallHandler) CreateItem(c *gin.Context) {
	var in dto.MallAdminItemUpsert
	if !BindJSON(c, &in) {
		return
	}
	item, err := h.mall.AdminCreateItem(c.Request.Context(), in)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Created(c, item)
}

// UpdateItem 编辑商品。
func (h *AdminMallHandler) UpdateItem(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Fail(c, errcode.ErrParamInvalid.WithMsg("路径参数 id 不合法"))
		return
	}
	var in dto.MallAdminItemUpsert
	if !BindJSON(c, &in) {
		return
	}
	if err := h.mall.AdminUpdateItem(c.Request.Context(), id, in); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// DeleteItem 删除商品（软删）。
func (h *AdminMallHandler) DeleteItem(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Fail(c, errcode.ErrParamInvalid.WithMsg("路径参数 id 不合法"))
		return
	}
	if err := h.mall.AdminDeleteItem(c.Request.Context(), id); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// ListRedemptions 兑换单队列。
func (h *AdminMallHandler) ListRedemptions(c *gin.Context) {
	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	size, _ := strconv.Atoi(c.DefaultQuery("pageSize", "20"))
	items, total, err := h.mall.AdminListRedemptions(c.Request.Context(), c.Query("status"), page, size)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.Page(c, items, total, page, size)
}

// Fulfill 发放兑换单。
func (h *AdminMallHandler) Fulfill(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Fail(c, errcode.ErrParamInvalid.WithMsg("路径参数 id 不合法"))
		return
	}
	var in dto.MallAdminFulfill
	if !BindJSON(c, &in) {
		return
	}
	if err := h.mall.AdminFulfill(c.Request.Context(), id, in.Remark); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true})
}

// Cancel 取消并退分。
func (h *AdminMallHandler) Cancel(c *gin.Context) {
	id, err := strconv.ParseInt(c.Param("id"), 10, 64)
	if err != nil || id <= 0 {
		response.Fail(c, errcode.ErrParamInvalid.WithMsg("路径参数 id 不合法"))
		return
	}
	var in struct {
		Reason string `json:"reason"`
	}
	if !BindJSON(c, &in) {
		return
	}
	if err := h.mall.AdminCancel(c.Request.Context(), id, in.Reason); err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, gin.H{"ok": true})
}
