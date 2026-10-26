package dto

import (
	"fmt"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/pkg/errcode"
)

const (
	// financeDefaultDays 不传日期时的默认窗口。
	financeDefaultDays = 30
	// financeMaxDays 单次查询上限。
	//
	// 92 天是一个季度的自然边界：财务按月结、按季对，超过一个季度还挤在一次查询里的，
	// 通常不是真的要看三个月，而是忘了传参数。拦下来比让报表跑十秒再超时更好。
	financeMaxDays = 92
)

// FinanceReportQuery 对账报表的日期区间。
//
// 日期用字符串而不是 time.Time：表单里传的就是 YYYY-MM-DD，
// 交给 binding 解 time.Time 会引入时区解释上的分歧（"2026-10-01" 到底是 UTC 还是本地）。
type FinanceReportQuery struct {
	From string `form:"from"`
	To   string `form:"to"`
}

// Range 把查询参数解析成左闭右闭的日期区间（按服务器本地时区）。
//
// 三种省略方式都有确定的含义，不做"猜用户想要什么"：
//   - 都不传：最近 30 天（含今天）
//   - 只传 to：以 to 结尾的 30 天
//   - 只传 from：从 from 到今天
func (q FinanceReportQuery) Range(now time.Time) (time.Time, time.Time, error) {
	today := time.Date(now.Year(), now.Month(), now.Day(), 0, 0, 0, 0, now.Location())

	parse := func(raw, label string) (time.Time, error) {
		v := strings.TrimSpace(raw)
		t, err := time.ParseInLocation("2006-01-02", v, today.Location())
		if err != nil {
			return time.Time{}, errcode.ErrParamInvalid.WithMsg(label + " 必须是 YYYY-MM-DD 格式的日期")
		}
		return t, nil
	}

	var from, to time.Time
	var err error
	switch {
	case q.From == "" && q.To == "":
		to = today
		from = today.AddDate(0, 0, -(financeDefaultDays - 1))
	case q.From == "":
		if to, err = parse(q.To, "to"); err != nil {
			return time.Time{}, time.Time{}, err
		}
		from = to.AddDate(0, 0, -(financeDefaultDays - 1))
	case q.To == "":
		if from, err = parse(q.From, "from"); err != nil {
			return time.Time{}, time.Time{}, err
		}
		to = today
	default:
		if from, err = parse(q.From, "from"); err != nil {
			return time.Time{}, time.Time{}, err
		}
		if to, err = parse(q.To, "to"); err != nil {
			return time.Time{}, time.Time{}, err
		}
	}

	if to.Before(from) {
		return time.Time{}, time.Time{}, errcode.ErrParamInvalid.WithMsg("结束日期不能早于开始日期")
	}
	if days := int(to.Sub(from).Hours()/24) + 1; days > financeMaxDays {
		return time.Time{}, time.Time{}, errcode.ErrParamInvalid.
			WithMsg(fmt.Sprintf("一次最多查询 %d 天（当前 %d 天），请缩小范围或分段导出", financeMaxDays, days))
	}
	return from, to, nil
}

// FinanceDailyRow 一天的对账行。
//
// 四组列各自独立，不做跨口径净额混算——收款、退款、结算、打款发生在四个不同时点，
// 把它们合成一个"今日利润"只会让人算错。唯一的合成列是 NetCash，
// 它的定义写在字段注释里，就是当天的现金净流入。
type FinanceDailyRow struct {
	Day string `json:"day"`

	PaidCount           int64 `json:"paidCount"`
	PaidAmountCents     int64 `json:"paidAmountCents"`
	PaidCommissionCents int64 `json:"paidCommissionCents"`
	PaidExpertCents     int64 `json:"paidExpertCents"`

	RefundCount int64 `json:"refundCount"`
	RefundCents int64 `json:"refundCents"`

	SettleCount            int64 `json:"settleCount"`
	SettledCommissionCents int64 `json:"settledCommissionCents"`
	SettledExpertCents     int64 `json:"settledExpertCents"`

	WithdrawCount       int64 `json:"withdrawCount"`
	WithdrawAmountCents int64 `json:"withdrawAmountCents"`
	WithdrawFeeCents    int64 `json:"withdrawFeeCents"`
	WithdrawActualCents int64 `json:"withdrawActualCents"`

	// NetCashCents = 收款 - 退款 - 提现实付。
	NetCashCents int64 `json:"netCashCents"`
}

// FinancePeriodSummary 区间合计。字段与日行一一对应，前端直接显示。
type FinancePeriodSummary struct {
	Days int64 `json:"days"`

	PaidCount           int64 `json:"paidCount"`
	PaidAmountCents     int64 `json:"paidAmountCents"`
	PaidCommissionCents int64 `json:"paidCommissionCents"`
	PaidExpertCents     int64 `json:"paidExpertCents"`

	RefundCount int64 `json:"refundCount"`
	RefundCents int64 `json:"refundCents"`

	SettleCount            int64 `json:"settleCount"`
	SettledCommissionCents int64 `json:"settledCommissionCents"`
	SettledExpertCents     int64 `json:"settledExpertCents"`

	WithdrawCount       int64 `json:"withdrawCount"`
	WithdrawAmountCents int64 `json:"withdrawAmountCents"`
	WithdrawFeeCents    int64 `json:"withdrawFeeCents"`
	WithdrawActualCents int64 `json:"withdrawActualCents"`

	NetCashCents int64 `json:"netCashCents"`
}

// FinanceOwed 平台当前代管与欠付（按当前时点，不按区间）。
type FinanceOwed struct {
	EscrowCents          int64 `json:"escrowCents"`
	EscrowCount          int64 `json:"escrowCount"`
	ExpertBalanceCents   int64 `json:"expertBalanceCents"`
	ExpertFrozenCents    int64 `json:"expertFrozenCents"`
	ExpertCount          int64 `json:"expertCount"`
	PendingWithdrawCents int64 `json:"pendingWithdrawCents"`
}

// FinanceTotals 全时段累计。
type FinanceTotals struct {
	PaidAmountCents     int64 `json:"paidAmountCents"`
	RefundCents         int64 `json:"refundCents"`
	WithdrawActualCents int64 `json:"withdrawActualCents"`
	NetCashCents        int64 `json:"netCashCents"`
	CommissionCents     int64 `json:"commissionCents"`
}

// FinanceCheck 一条账面校验结果。
type FinanceCheck struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Level   string   `json:"level"` // error 账不平 / warn 数据不全
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Samples []string `json:"samples"`
	Checked int64    `json:"checked"`
}

// FinanceReportResp 对账报表。
type FinanceReportResp struct {
	From    string               `json:"from"`
	To      string               `json:"to"`
	Summary FinancePeriodSummary `json:"summary"`
	Rows    []FinanceDailyRow    `json:"rows"`
	Owed    FinanceOwed          `json:"owed"`
	Totals  FinanceTotals        `json:"totals"`
	Checks  []FinanceCheck       `json:"checks"`
}

// FinanceDayOrder 当日收款或退款的订单。
type FinanceDayOrder struct {
	OrderNo         string     `json:"orderNo"`
	Title           string     `json:"title"`
	Status          string     `json:"status"`
	AmountCents     int64      `json:"amountCents"`
	CommissionCents int64      `json:"commissionCents"`
	ExpertCents     int64      `json:"expertAmountCents"`
	RefundCents     int64      `json:"refundCents"`
	PaidAt          *time.Time `json:"paidAt"`
	RefundedAt      *time.Time `json:"refundedAt"`
}

// FinanceDaySettlement 当日结算的分账记录。
type FinanceDaySettlement struct {
	SettlementNo    string     `json:"settlementNo"`
	OrderNo         string     `json:"orderNo"`
	ExpertID        int64      `json:"expertId"`
	AmountCents     int64      `json:"amountCents"`
	CommissionCents int64      `json:"commissionCents"`
	ExpertCents     int64      `json:"expertAmountCents"`
	SettledAt       *time.Time `json:"settledAt"`
}

// FinanceDayWithdrawal 当日打款的提现单。
type FinanceDayWithdrawal struct {
	WithdrawalNo string     `json:"withdrawalNo"`
	ExpertID     int64      `json:"expertId"`
	AmountCents  int64      `json:"amountCents"`
	FeeCents     int64      `json:"feeCents"`
	ActualCents  int64      `json:"actualCents"`
	AccountName  string     `json:"accountName"`
	AccountNo    string     `json:"accountNoMasked"`
	PaidAt       *time.Time `json:"paidAt"`
}

// FinanceDayDetail 单日明细。
type FinanceDayDetail struct {
	Day         string                 `json:"day"`
	Orders      []FinanceDayOrder      `json:"orders"`
	Settlements []FinanceDaySettlement `json:"settlements"`
	Withdrawals []FinanceDayWithdrawal `json:"withdrawals"`
}
