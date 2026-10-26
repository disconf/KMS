package handler

import (
	"fmt"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/service"
)

// AdminFinanceHandler 财务对账后台接口。
//
// 这一组只读：对账报表不改任何数据。真发现不平，处理方式是去订单页 / 提现页
// 逐笔核对，而不是在报表上"改个数让它平"。
type AdminFinanceHandler struct {
	finance *service.FinanceService
}

// NewAdminFinanceHandler 创建 AdminFinanceHandler。
func NewAdminFinanceHandler(finance *service.FinanceService) *AdminFinanceHandler {
	return &AdminFinanceHandler{finance: finance}
}

// Report 区间对账报表：日表 + 区间合计 + 平台欠付 + 全时段累计 + 账面校验。
//
//	@Summary  对账报表
//	@Tags     admin-finance
//	@Security BearerAuth
//	@Produce  json
//	@Param    from query string false "开始日期 YYYY-MM-DD（默认 30 天前）"
//	@Param    to   query string false "结束日期 YYYY-MM-DD（默认今天，含当天）"
//	@Success  200 {object} response.Body
//	@Router   /admin/finance/reconciliation [get]
func (h *AdminFinanceHandler) Report(c *gin.Context) {
	var q dto.FinanceReportQuery
	if !BindQuery(c, &q) {
		return
	}
	rep, err := h.finance.Report(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, rep)
}

// Day 单日明细：当天收款的订单、结算的分账、打款的提现。
//
//	@Summary  对账单日明细
//	@Tags     admin-finance
//	@Security BearerAuth
//	@Produce  json
//	@Param    date path string true "日期 YYYY-MM-DD"
//	@Success  200 {object} response.Body
//	@Router   /admin/finance/reconciliation/day/{date} [get]
func (h *AdminFinanceHandler) Day(c *gin.Context) {
	d, err := h.finance.DayDetail(c.Request.Context(), c.Param("date"))
	if err != nil {
		response.Fail(c, err)
		return
	}
	response.OK(c, d)
}

// Export 导出区间日表（CSV）。
//
// 用 GET 直接返回文件流而不是返回一个下载链接：
// 报表是即时数据，生成一个带时效的链接需要额外的存储与签名，
// 而财务点完就存本地，没有必要在服务端留副本。
//
//	@Summary  导出对账 CSV
//	@Tags     admin-finance
//	@Security BearerAuth
//	@Produce  text/csv
//	@Param    from query string false "开始日期"
//	@Param    to   query string false "结束日期"
//	@Success  200 {file} file
//	@Router   /admin/finance/reconciliation/export [get]
func (h *AdminFinanceHandler) Export(c *gin.Context) {
	var q dto.FinanceReportQuery
	if !BindQuery(c, &q) {
		return
	}
	data, name, err := h.finance.ExportCSV(c.Request.Context(), q)
	if err != nil {
		response.Fail(c, err)
		return
	}

	// 文件名是纯 ASCII（日期区间），不带中文：Content-Disposition 里放中文
	// 需要 RFC 5987 编码，各浏览器行为不一致，这里没必要冒那个险。
	c.Header("Content-Disposition", fmt.Sprintf(`attachment; filename="%s"`, name))
	c.Data(200, "text/csv; charset=utf-8", data)
}
