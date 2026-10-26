package service

import (
	"bytes"
	"context"
	"encoding/csv"
	"fmt"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/repository"
)

// FinanceService 财务对账。
//
// 这一层的职责很窄：把仓储返回的四组聚合按日期拼成连续的表、算区间合计、
// 顺手把账面校验包成 DTO。**不做任何金额运算上的"补偿"**——
// 校验不平就是不平，把差额悄悄算进某一列，等于替真问题打掩护。
type FinanceService struct {
	reports *repository.FinanceReportRepo
	// now 可替换，便于测试固定"今天"。
	now func() time.Time
}

// NewFinanceService 创建 FinanceService。
func NewFinanceService(reports *repository.FinanceReportRepo) *FinanceService {
	return &FinanceService{reports: reports, now: time.Now}
}

// Report 生成区间对账报表。
func (s *FinanceService) Report(ctx context.Context, q dto.FinanceReportQuery) (*dto.FinanceReportResp, error) {
	from, to, err := q.Range(s.now())
	if err != nil {
		return nil, err
	}

	daily, err := s.reports.DailyReport(ctx, from, to)
	if err != nil {
		return nil, err
	}
	owed, err := s.reports.Owed(ctx)
	if err != nil {
		return nil, err
	}
	totals, err := s.reports.Totals(ctx)
	if err != nil {
		return nil, err
	}
	checks, err := s.reports.Checks(ctx)
	if err != nil {
		return nil, err
	}

	// 补齐没有流水的日期：财务看到"3 号缺失"会以为是漏了，其实是那天没单。
	// 连续日期是这个表的基本形态，补空行的成本是零（最多 92 行）。
	index := make(map[string]repository.FinanceDailyRow, len(daily))
	for _, r := range daily {
		index[r.Day] = r
	}

	resp := &dto.FinanceReportResp{
		From:   from.Format("2006-01-02"),
		To:     to.Format("2006-01-02"),
		Rows:   make([]dto.FinanceDailyRow, 0, len(index)),
		Owed:   toOwedDTO(owed),
		Totals: toTotalsDTO(totals),
		Checks: make([]dto.FinanceCheck, 0, len(checks)),
	}

	var sum dto.FinancePeriodSummary
	for d := from; !d.After(to); d = d.AddDate(0, 0, 1) {
		key := d.Format("2006-01-02")
		row := dto.FinanceDailyRow{Day: key}
		if r, ok := index[key]; ok {
			row = toDailyDTO(r)
		}
		resp.Rows = append(resp.Rows, row)

		sum.Days++
		sum.PaidCount += row.PaidCount
		sum.PaidAmountCents += row.PaidAmountCents
		sum.PaidCommissionCents += row.PaidCommissionCents
		sum.PaidExpertCents += row.PaidExpertCents
		sum.RefundCount += row.RefundCount
		sum.RefundCents += row.RefundCents
		sum.SettleCount += row.SettleCount
		sum.SettledCommissionCents += row.SettledCommissionCents
		sum.SettledExpertCents += row.SettledExpertCents
		sum.WithdrawCount += row.WithdrawCount
		sum.WithdrawAmountCents += row.WithdrawAmountCents
		sum.WithdrawFeeCents += row.WithdrawFeeCents
		sum.WithdrawActualCents += row.WithdrawActualCents
		sum.NetCashCents += row.NetCashCents
	}
	resp.Summary = sum

	for _, c := range checks {
		resp.Checks = append(resp.Checks, toCheckDTO(c))
	}
	return resp, nil
}

// DayDetail 取某一天的明细。
func (s *FinanceService) DayDetail(ctx context.Context, rawDate string) (*dto.FinanceDayDetail, error) {
	date := strings.TrimSpace(rawDate)
	day, err := time.ParseInLocation("2006-01-02", date, s.now().Location())
	if err != nil {
		return nil, errcode.ErrParamInvalid.WithMsg("date 必须是 YYYY-MM-DD 格式的日期")
	}

	d, err := s.reports.DayDetail(ctx, day)
	if err != nil {
		return nil, err
	}

	out := &dto.FinanceDayDetail{
		Day:         d.Day,
		Orders:      make([]dto.FinanceDayOrder, 0, len(d.Orders)),
		Settlements: make([]dto.FinanceDaySettlement, 0, len(d.Settlements)),
		Withdrawals: make([]dto.FinanceDayWithdrawal, 0, len(d.Withdrawals)),
	}
	for _, o := range d.Orders {
		out.Orders = append(out.Orders, dto.FinanceDayOrder{
			OrderNo:         o.OrderNo,
			Title:           o.Title,
			Status:          o.Status,
			AmountCents:     o.AmountCents,
			CommissionCents: o.CommissionCents,
			ExpertCents:     o.ExpertCents,
			RefundCents:     o.RefundCents,
			PaidAt:          o.PaidAt,
			RefundedAt:      o.RefundedAt,
		})
	}
	for _, x := range d.Settlements {
		out.Settlements = append(out.Settlements, dto.FinanceDaySettlement{
			SettlementNo:    x.SettlementNo,
			OrderNo:         x.OrderNo,
			ExpertID:        x.ExpertID,
			AmountCents:     x.AmountCents,
			CommissionCents: x.CommissionCents,
			ExpertCents:     x.ExpertCents,
			SettledAt:       x.SettledAt,
		})
	}
	for _, w := range d.Withdrawals {
		out.Withdrawals = append(out.Withdrawals, dto.FinanceDayWithdrawal{
			WithdrawalNo: w.WithdrawalNo,
			ExpertID:     w.ExpertID,
			AmountCents:  w.AmountCents,
			FeeCents:     w.FeeCents,
			ActualCents:  w.ActualCents,
			AccountName:  w.AccountName,
			AccountNo:    w.AccountNo,
			PaidAt:       w.PaidAt,
		})
	}
	return out, nil
}

// ExportCSV 导出一个区间的日表，返回文件内容与建议文件名。
//
// 直接复用 Report：报表页看到什么，导出的就是什么。
// 另开一条查询很容易出现"页面和 Excel 差 3 分钱"，那种问题查一次要半天。
func (s *FinanceService) ExportCSV(ctx context.Context, q dto.FinanceReportQuery) ([]byte, string, error) {
	rep, err := s.Report(ctx, q)
	if err != nil {
		return nil, "", err
	}

	var buf bytes.Buffer
	// BOM：没有它，Excel 打开中文表头就是乱码。CSV 本身不带编码信息，
	// 只能靠这三个字节告诉 Excel"这是 UTF-8"。
	buf.WriteString("\xef\xbb\xbf")

	w := csv.NewWriter(&buf)
	header := []string{
		"日期", "收款笔数", "收款金额", "订单佣金", "专家应得",
		"退款笔数", "退款金额",
		"结算笔数", "已结算佣金", "已结算专家实收",
		"提现笔数", "提现金额", "提现手续费", "提现实付",
		"当日净现金",
	}
	if err := w.Write(header); err != nil {
		return nil, "", errcode.ErrDatabase.Wrap(fmt.Errorf("生成导出文件失败: %w", err))
	}

	for _, r := range rep.Rows {
		rec := []string{
			r.Day,
			itoa(r.PaidCount), yuanPlain(r.PaidAmountCents), yuanPlain(r.PaidCommissionCents), yuanPlain(r.PaidExpertCents),
			itoa(r.RefundCount), yuanPlain(r.RefundCents),
			itoa(r.SettleCount), yuanPlain(r.SettledCommissionCents), yuanPlain(r.SettledExpertCents),
			itoa(r.WithdrawCount), yuanPlain(r.WithdrawAmountCents), yuanPlain(r.WithdrawFeeCents), yuanPlain(r.WithdrawActualCents),
			yuanPlain(r.NetCashCents),
		}
		if err := w.Write(rec); err != nil {
			return nil, "", errcode.ErrDatabase.Wrap(fmt.Errorf("生成导出文件失败: %w", err))
		}
	}

	sum := rep.Summary
	footer := []string{
		"合计",
		itoa(sum.PaidCount), yuanPlain(sum.PaidAmountCents), yuanPlain(sum.PaidCommissionCents), yuanPlain(sum.PaidExpertCents),
		itoa(sum.RefundCount), yuanPlain(sum.RefundCents),
		itoa(sum.SettleCount), yuanPlain(sum.SettledCommissionCents), yuanPlain(sum.SettledExpertCents),
		itoa(sum.WithdrawCount), yuanPlain(sum.WithdrawAmountCents), yuanPlain(sum.WithdrawFeeCents), yuanPlain(sum.WithdrawActualCents),
		yuanPlain(sum.NetCashCents),
	}
	if err := w.Write(footer); err != nil {
		return nil, "", errcode.ErrDatabase.Wrap(fmt.Errorf("生成导出文件失败: %w", err))
	}

	// 校验结论不写进 CSV：这张表是给 Excel 算数的，
	// 表格下面多出一段自由文本会让行数不确定、SUM 范围跟着错位。
	// 不平的结论在页面上和导出前就提示，文件本身只放可计算的表。

	w.Flush()
	if err := w.Error(); err != nil {
		return nil, "", errcode.ErrDatabase.Wrap(fmt.Errorf("生成导出文件失败: %w", err))
	}

	name := fmt.Sprintf("finance-reconciliation-%s_%s.csv",
		strings.ReplaceAll(rep.From, "-", ""), strings.ReplaceAll(rep.To, "-", ""))
	return buf.Bytes(), name, nil
}

/* ============================ 转换 ============================ */

func toDailyDTO(r repository.FinanceDailyRow) dto.FinanceDailyRow {
	return dto.FinanceDailyRow{
		Day:                    r.Day,
		PaidCount:              r.PaidCount,
		PaidAmountCents:        r.PaidAmountCents,
		PaidCommissionCents:    r.PaidCommissionCents,
		PaidExpertCents:        r.PaidExpertCents,
		RefundCount:            r.RefundCount,
		RefundCents:            r.RefundCents,
		SettleCount:            r.SettleCount,
		SettledCommissionCents: r.SettledCommissionCents,
		SettledExpertCents:     r.SettledExpertCents,
		WithdrawCount:          r.WithdrawCount,
		WithdrawAmountCents:    r.WithdrawAmountCents,
		WithdrawFeeCents:       r.WithdrawFeeCents,
		WithdrawActualCents:    r.WithdrawActualCents,
		NetCashCents:           r.NetCashCents,
	}
}

func toOwedDTO(o *repository.FinanceOwed) dto.FinanceOwed {
	if o == nil {
		return dto.FinanceOwed{}
	}
	return dto.FinanceOwed{
		EscrowCents:          o.EscrowCents,
		EscrowCount:          o.EscrowCount,
		ExpertBalanceCents:   o.ExpertBalanceCents,
		ExpertFrozenCents:    o.ExpertFrozenCents,
		ExpertCount:          o.ExpertCount,
		PendingWithdrawCents: o.PendingWithdrawCents,
	}
}

func toTotalsDTO(t *repository.FinanceTotals) dto.FinanceTotals {
	if t == nil {
		return dto.FinanceTotals{}
	}
	return dto.FinanceTotals{
		PaidAmountCents:     t.PaidAmountCents,
		RefundCents:         t.RefundCents,
		WithdrawActualCents: t.WithdrawActualCents,
		NetCashCents:        t.NetCashCents,
		CommissionCents:     t.CommissionCents,
	}
}

func toCheckDTO(c repository.FinanceCheck) dto.FinanceCheck {
	samples := c.Samples
	if samples == nil {
		samples = []string{}
	}
	return dto.FinanceCheck{
		Key:     c.Key,
		Name:    c.Name,
		Level:   c.Level,
		OK:      c.OK,
		Message: c.Message,
		Samples: samples,
		Checked: c.Checked,
	}
}

// yuanPlain 把「分」写成 CSV 里的元，保留两位小数。
//
// 这里刻意用整数除法而不是 float64(cents)/100：
// 金额全程以分存储就是为了不碰浮点，导出这一环也不该破例——
// 0.1+0.2 那类误差在财务报表里会直接变成"对不上"。
func yuanPlain(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s%d.%02d", sign, cents/100, cents%100)
}

// itoa 小整数转字符串，CSV 里笔数列用。
func itoa(v int64) string {
	if v == 0 {
		return "0"
	}
	var buf [20]byte
	i := len(buf)
	neg := v < 0
	if neg {
		v = -v
	}
	for v > 0 {
		i--
		buf[i] = byte('0' + v%10)
		v /= 10
	}
	if neg {
		i--
		buf[i] = '-'
	}
	return string(buf[i:])
}
