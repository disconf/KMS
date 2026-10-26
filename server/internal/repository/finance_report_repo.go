package repository

import (
	"context"
	"fmt"
	"strings"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// FinanceReportRepo 财务对账报表数据访问。
//
// 对账不能只用一条大 SQL 把四件事加在一起，因为它们的**时间口径各不相同**：
//
//	收款   按 orders.paid_at       —— 钱进来的那天
//	退款   按 orders.refunded_at   —— 钱出去的那天（不是下单日）
//	结算   按 settlements.settled_at —— 专家余额增加的那天（不是收款日）
//	打款   按 withdrawals.paid_at  —— 钱真正离开平台的那天（不是申请日）
//
// 一条订单可能 1 号收款、5 号退款；把退款算在 1 号，1 号的净额就虚高，
// 而 5 号看不出少了钱。所以四段独立聚合，在 Go 里按日期拼成一行的四组列。
type FinanceReportRepo struct {
	db *gorm.DB
}

// NewFinanceReportRepo 创建 FinanceReportRepo。
func NewFinanceReportRepo(db *gorm.DB) *FinanceReportRepo {
	return &FinanceReportRepo{db: db}
}

// dayExpr 给出"把时间列截成日期"的表达式。
//
// 这里不能用 DATE() 统一处理：SQLite 的 date() 会把值**先转成 UTC 再取日期**，
// 而库里存的是带偏移的本地时间（如 2026-10-15 00:30:00+08:00）。
// 实测 date('2026-10-15 00:30:00+08:00') = '2026-10-14'——
// 也就是说早 8 点前的每一笔钱都会被算到前一天，日报表和日结单永远对不上。
// SQLite 侧改为直接截前 10 个字符，取到的正是写入时的本地日期。
// MySQL 的 datetime 不带时区，DATE() 就是取日期部分，可以放心用。
func (r *FinanceReportRepo) dayExpr(col string) string {
	if r.db.Dialector.Name() == "mysql" {
		return "DATE(" + col + ")"
	}
	return "substr(" + col + ", 1, 10)"
}

// FinanceDailyRow 一天的对账行。
//
// 列按口径分组，不预先做跨口径净额运算（除了 NetCash，它的含义是"当天现金净流入"，
// 定义写在注释里）。营销口径要改的时候，改的是展示层，不是这里的账。
type FinanceDailyRow struct {
	Day string `json:"day"` // YYYY-MM-DD

	// 收款：按 paid_at 归日。金额是用户实际付的，佣金与专家实收是订单上的快照。
	PaidCount           int64 `json:"paidCount"`
	PaidAmountCents     int64 `json:"paidAmountCents"`
	PaidCommissionCents int64 `json:"paidCommissionCents"`
	PaidExpertCents     int64 `json:"paidExpertCents"`

	// 退款：按 refunded_at 归日，从平台已收款里退出去的部分。
	RefundCount int64 `json:"refundCount"`
	RefundCents int64 `json:"refundCents"`

	// 结算：按 settled_at 归日。这一天专家余额增加，平台确认佣金收入。
	// 注意它与"收款"不是同一天，两者之差就是平台账上代管中的钱。
	SettleCount            int64 `json:"settleCount"`
	SettledCommissionCents int64 `json:"settledCommissionCents"`
	SettledExpertCents     int64 `json:"settledExpertCents"`

	// 打款：按 paid_at 归日。actual 是真正打出去的钱，fee 留在平台。
	WithdrawCount       int64 `json:"withdrawCount"`
	WithdrawAmountCents int64 `json:"withdrawAmountCents"`
	WithdrawFeeCents    int64 `json:"withdrawFeeCents"`
	WithdrawActualCents int64 `json:"withdrawActualCents"`

	// NetCash 当天现金净流入 = 收款 - 退款 - 提现实付。
	// 这是"平台账户余额今天涨了多少"，与佣金收入不是一回事：
	// 收款里含代管中的专家款，结算只是把它从代管挪到专家余额，不动现金。
	NetCashCents int64 `json:"netCashCents"`
}

// FinanceOwed 平台当前欠出去的钱（按当前时点，不按日期）。
//
// 这三项是财务最关心的"平台手上有多少是别人的钱"：
// 代管中的订单款、专家可提余额、提现冻结。三者加上已确认佣金，应当能被平台账户覆盖。
type FinanceOwed struct {
	EscrowCents          int64 `json:"escrowCents"`          // 已收款但还没结算的订单金额（扣掉已退款）
	EscrowCount          int64 `json:"escrowCount"`          //
	ExpertBalanceCents   int64 `json:"expertBalanceCents"`   // 专家可提现余额合计
	ExpertFrozenCents    int64 `json:"expertFrozenCents"`    // 提现申请中冻结的合计
	ExpertCount          int64 `json:"expertCount"`          // 有余额或有冻结的专家数
	PendingWithdrawCents int64 `json:"pendingWithdrawCents"` // 处理中的提现申请（待审 + 待打款）
}

// FinanceTotals 全时段累计，用于判断账面粉规模，不受日期范围影响。
type FinanceTotals struct {
	PaidAmountCents     int64 `json:"paidAmountCents"`
	RefundCents         int64 `json:"refundCents"`
	WithdrawActualCents int64 `json:"withdrawActualCents"`
	NetCashCents        int64 `json:"netCashCents"`
	CommissionCents     int64 `json:"commissionCents"` // 已结算佣金累计（平台确认的收入）
}

// boundPair 构造"左闭右开"的时间范围：报表按天切，to 当天必须包含在内，
// 所以上界取 to+1 天的零点。用 <= 'to 23:59:59' 会漏掉最后一秒里的记录。
func boundPair(from, to time.Time) (time.Time, time.Time) {
	return from, to.AddDate(0, 0, 1)
}

// DailyReport 聚合 [from, to] 区间内的四类流水，按日返回。
//
// 空日期（当天没有任何流水）不返回，由 service 补齐——补空行是展示需要，
// 不是账务事实，让它留在数据层之外。
func (r *FinanceReportRepo) DailyReport(ctx context.Context, from, to time.Time) ([]FinanceDailyRow, error) {
	lo, hi := boundPair(from, to)
	byDay := make(map[string]*FinanceDailyRow)

	get := func(day string) *FinanceDailyRow {
		if row, ok := byDay[day]; ok {
			return row
		}
		row := &FinanceDailyRow{Day: day}
		byDay[day] = row
		return row
	}

	// ① 收款
	{
		var rows []struct {
			Day             string
			Cnt             int64
			AmountCents     int64
			CommissionCents int64
			ExpertCents     int64
		}
		err := r.db.WithContext(ctx).
			Table("orders").
			Select(fmt.Sprintf(`%s AS day, COUNT(*) AS cnt,
				COALESCE(SUM(amount_cents), 0) AS amount_cents,
				COALESCE(SUM(commission_cents), 0) AS commission_cents,
				COALESCE(SUM(expert_amount_cents), 0) AS expert_cents`, r.dayExpr("paid_at"))).
			Where("deleted_at IS NULL AND paid_at IS NOT NULL AND paid_at >= ? AND paid_at < ?", lo, hi).
			Group("day").
			Scan(&rows).Error
		if err != nil {
			return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计收款失败: %w", err))
		}
		for _, x := range rows {
			day, err := normalizeDay(x.Day)
			if err != nil {
				return nil, err
			}
			row := get(day)
			row.PaidCount += x.Cnt
			row.PaidAmountCents += x.AmountCents
			row.PaidCommissionCents += x.CommissionCents
			row.PaidExpertCents += x.ExpertCents
		}
	}

	// ② 退款
	{
		var rows []struct {
			Day         string
			Cnt         int64
			RefundCents int64
		}
		err := r.db.WithContext(ctx).
			Table("orders").
			Select(fmt.Sprintf(`%s AS day, COUNT(*) AS cnt,
				COALESCE(SUM(refund_cents), 0) AS refund_cents`, r.dayExpr("refunded_at"))).
			Where("deleted_at IS NULL AND refunded_at IS NOT NULL AND refunded_at >= ? AND refunded_at < ?", lo, hi).
			Group("day").
			Scan(&rows).Error
		if err != nil {
			return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计退款失败: %w", err))
		}
		for _, x := range rows {
			day, err := normalizeDay(x.Day)
			if err != nil {
				return nil, err
			}
			row := get(day)
			row.RefundCount += x.Cnt
			row.RefundCents += x.RefundCents
		}
	}

	// ③ 结算
	{
		var rows []struct {
			Day             string
			Cnt             int64
			CommissionCents int64
			ExpertCents     int64
		}
		err := r.db.WithContext(ctx).
			Table("settlements").
			Select(fmt.Sprintf(`%s AS day, COUNT(*) AS cnt,
				COALESCE(SUM(commission_cents), 0) AS commission_cents,
				COALESCE(SUM(expert_amount_cents), 0) AS expert_cents`, r.dayExpr("settled_at"))).
			Where("deleted_at IS NULL AND status = ? AND settled_at IS NOT NULL AND settled_at >= ? AND settled_at < ?",
				model.SettlementStatusSettled, lo, hi).
			Group("day").
			Scan(&rows).Error
		if err != nil {
			return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计结算失败: %w", err))
		}
		for _, x := range rows {
			day, err := normalizeDay(x.Day)
			if err != nil {
				return nil, err
			}
			row := get(day)
			row.SettleCount += x.Cnt
			row.SettledCommissionCents += x.CommissionCents
			row.SettledExpertCents += x.ExpertCents
		}
	}

	// ④ 打款。只算 paid：rejected / failed 的钱退回了专家余额，从没离开平台。
	{
		var rows []struct {
			Day         string
			Cnt         int64
			AmountCents int64
			FeeCents    int64
			ActualCents int64
		}
		err := r.db.WithContext(ctx).
			Table("withdrawals").
			Select(fmt.Sprintf(`%s AS day, COUNT(*) AS cnt,
				COALESCE(SUM(amount_cents), 0) AS amount_cents,
				COALESCE(SUM(fee_cents), 0) AS fee_cents,
				COALESCE(SUM(actual_cents), 0) AS actual_cents`, r.dayExpr("paid_at"))).
			Where("deleted_at IS NULL AND status = ? AND paid_at IS NOT NULL AND paid_at >= ? AND paid_at < ?",
				model.WithdrawalStatusPaid, lo, hi).
			Group("day").
			Scan(&rows).Error
		if err != nil {
			return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计提现打款失败: %w", err))
		}
		for _, x := range rows {
			day, err := normalizeDay(x.Day)
			if err != nil {
				return nil, err
			}
			row := get(day)
			row.WithdrawCount += x.Cnt
			row.WithdrawAmountCents += x.AmountCents
			row.WithdrawFeeCents += x.FeeCents
			row.WithdrawActualCents += x.ActualCents
		}
	}

	out := make([]FinanceDailyRow, 0, len(byDay))
	for _, row := range byDay {
		row.NetCashCents = row.PaidAmountCents - row.RefundCents - row.WithdrawActualCents
		out = append(out, *row)
	}
	return out, nil
}

// Owed 平台当前代管与欠付金额。
func (r *FinanceReportRepo) Owed(ctx context.Context) (*FinanceOwed, error) {
	out := &FinanceOwed{}

	// 代管中：已付钱但还没结算的订单。退款单已经出过账，要扣掉。
	var escrow struct {
		Cnt         int64
		AmountCents int64
	}
	err := r.db.WithContext(ctx).
		Table("orders").
		Select("COUNT(*) AS cnt, COALESCE(SUM(amount_cents - refund_cents), 0) AS amount_cents").
		Where("deleted_at IS NULL AND paid_at IS NOT NULL AND status IN ?",
			[]string{model.OrderStatusInService, model.OrderStatusPendingConfirm, model.OrderStatusCompleted, model.OrderStatusDisputed}).
		Scan(&escrow).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计代管金额失败: %w", err))
	}
	out.EscrowCount = escrow.Cnt
	out.EscrowCents = escrow.AmountCents

	var ex struct {
		Cnt     int64
		Balance int64
		Frozen  int64
	}
	err = r.db.WithContext(ctx).
		Table("experts").
		Select("COUNT(*) AS cnt, COALESCE(SUM(balance_cents), 0) AS balance, COALESCE(SUM(frozen_cents), 0) AS frozen").
		Where("deleted_at IS NULL").
		Scan(&ex).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计专家余额失败: %w", err))
	}
	out.ExpertCount = ex.Cnt
	out.ExpertBalanceCents = ex.Balance
	out.ExpertFrozenCents = ex.Frozen

	var pending *int64
	err = r.db.WithContext(ctx).
		Table("withdrawals").
		Select("COALESCE(SUM(amount_cents), 0)").
		Where("deleted_at IS NULL AND status IN ?",
			[]string{model.WithdrawalStatusPending, model.WithdrawalStatusApproved}).
		Scan(&pending).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计处理中提现失败: %w", err))
	}
	if pending != nil {
		out.PendingWithdrawCents = *pending
	}
	return out, nil
}

// Totals 全时段累计。不限日期，用来判断账面粉规模。
func (r *FinanceReportRepo) Totals(ctx context.Context) (*FinanceTotals, error) {
	out := &FinanceTotals{}

	var paid *int64
	if err := r.db.WithContext(ctx).Table("orders").
		Select("COALESCE(SUM(amount_cents), 0)").
		Where("deleted_at IS NULL AND paid_at IS NOT NULL").
		Scan(&paid).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计累计收款失败: %w", err))
	}
	var refund *int64
	if err := r.db.WithContext(ctx).Table("orders").
		Select("COALESCE(SUM(refund_cents), 0)").
		// 累计口径用金额本身判断，不用 refunded_at：这个字段是后加的，
		// 之前退掉的单子 refunded_at 为 NULL，按时间过滤会把它们漏掉，
		// 累计净现金就会虚高。按日统计没法补历史（归不到哪一天），
		// 但"一共退过多少钱"必须准，所以这里换条件。
		Where("deleted_at IS NULL AND refund_cents > 0").
		Scan(&refund).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计累计退款失败: %w", err))
	}
	var paidOut *int64
	if err := r.db.WithContext(ctx).Table("withdrawals").
		Select("COALESCE(SUM(actual_cents), 0)").
		Where("deleted_at IS NULL AND status = ?", model.WithdrawalStatusPaid).
		Scan(&paidOut).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计累计打款失败: %w", err))
	}
	var commission *int64
	if err := r.db.WithContext(ctx).Table("settlements").
		Select("COALESCE(SUM(commission_cents), 0)").
		Where("deleted_at IS NULL AND status = ?", model.SettlementStatusSettled).
		Scan(&commission).Error; err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("统计累计佣金失败: %w", err))
	}

	if paid != nil {
		out.PaidAmountCents = *paid
	}
	if refund != nil {
		out.RefundCents = *refund
	}
	if paidOut != nil {
		out.WithdrawActualCents = *paidOut
	}
	if commission != nil {
		out.CommissionCents = *commission
	}
	out.NetCashCents = out.PaidAmountCents - out.RefundCents - out.WithdrawActualCents
	return out, nil
}

/* ============================ 账面校验 ============================ */

// FinanceCheck 一条账面校验的结果。
//
// 这些检查不是"提醒运营注意一下"，而是**账务恒等式**：一旦不平，说明某条写路径
// 漏了字段或算错了钱，必须人工介入。所以 level=error 的一律给红，不做成灰色提示。
type FinanceCheck struct {
	Key     string   `json:"key"`
	Name    string   `json:"name"`
	Level   string   `json:"level"` // error 账不平 / warn 数据不全但不影响恒等式
	OK      bool     `json:"ok"`
	Message string   `json:"message"`
	Samples []string `json:"samples"` // 最多 10 条，够定位即可
	Checked int64    `json:"checked"` // 参与检查的记录数
}

// 校验等级。
const (
	CheckLevelError = "error"
	CheckLevelWarn  = "warn"
)

// checkSampleLimit 每条校验最多带出多少条样本。
// 全量列出来对定位没有额外帮助，反而会把页面刷成一片红。
const checkSampleLimit = 10

// Checks 跑全部账面校验。
func (r *FinanceReportRepo) Checks(ctx context.Context) ([]FinanceCheck, error) {
	checks := make([]FinanceCheck, 0, 7)
	add := func(c FinanceCheck, err error) error {
		if err != nil {
			return err
		}
		// 默认按"账不平"处理：只有明确标了 warn 的检查才降级。
		// 反过来写（默认 warn、逐条标 error）迟早会漏标一条，
		// 而漏标的表现就是用户看不见这条不平。
		if c.Level == "" {
			c.Level = CheckLevelError
		}
		checks = append(checks, c)
		return nil
	}

	if err := add(r.checkOrderIdentity(ctx)); err != nil {
		return nil, err
	}
	if err := add(r.checkSettlementIdentity(ctx)); err != nil {
		return nil, err
	}
	if err := add(r.checkWithdrawIdentity(ctx)); err != nil {
		return nil, err
	}
	if err := add(r.checkExpertLedger(ctx)); err != nil {
		return nil, err
	}
	if err := add(r.checkSettledHasSettlement(ctx)); err != nil {
		return nil, err
	}
	if err := add(r.checkPaidBeforeService(ctx)); err != nil {
		return nil, err
	}
	if err := add(r.checkRefundTimeMissing(ctx)); err != nil {
		return nil, err
	}
	return checks, nil
}

// checkOrderIdentity 订单金额恒等式：佣金 + 专家实收 == 订单总额。
//
// 这是抽佣拆分唯一的硬约束。不平意味着一笔钱凭空出现或消失，
// 结算时会直接把错的专家实收打进余额。
func (r *FinanceReportRepo) checkOrderIdentity(ctx context.Context) (FinanceCheck, error) {
	const key = "order_amount_identity"
	c := FinanceCheck{
		Key:  key,
		Name: "订单金额恒等式（佣金 + 专家实收 = 总额）",
	}
	var total int64
	if err := r.db.WithContext(ctx).Table("orders").
		Where("deleted_at IS NULL AND paid_at IS NOT NULL").
		Count(&total).Error; err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("统计已支付订单失败: %w", err))
	}
	c.Checked = total

	var rows []struct {
		OrderNo     string
		AmountCents int64
		Diff        int64
	}
	err := r.db.WithContext(ctx).Table("orders").
		Select("order_no, amount_cents, (commission_cents + expert_amount_cents - amount_cents) AS diff").
		Where("deleted_at IS NULL AND paid_at IS NOT NULL AND commission_cents + expert_amount_cents <> amount_cents").
		Order("id DESC").Limit(checkSampleLimit).
		Scan(&rows).Error
	if err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("校验订单金额失败: %w", err))
	}
	c.OK = len(rows) == 0
	if c.OK {
		c.Message = fmt.Sprintf("%d 笔已支付订单全部平账", total)
	} else {
		for _, x := range rows {
			c.Samples = append(c.Samples, fmt.Sprintf("%s 总额 %s，差额 %s",
				x.OrderNo, yuanText(x.AmountCents), signedYuan(x.Diff)))
		}
		c.Message = fmt.Sprintf("发现 %d 笔订单的佣金与专家实收之和不等于总额", len(rows))
	}
	return c, nil
}

// checkSettlementIdentity 分账恒等式，与订单侧同一条规则。
func (r *FinanceReportRepo) checkSettlementIdentity(ctx context.Context) (FinanceCheck, error) {
	c := FinanceCheck{
		Key:  "settlement_amount_identity",
		Name: "分账金额恒等式（佣金 + 专家实收 = 总额）",
	}
	var total int64
	if err := r.db.WithContext(ctx).Table("settlements").
		Where("deleted_at IS NULL").Count(&total).Error; err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("统计分账记录失败: %w", err))
	}
	c.Checked = total

	var rows []struct {
		SettlementNo string
		OrderNo      string
		Diff         int64
	}
	err := r.db.WithContext(ctx).Table("settlements").
		Select("settlement_no, order_no, (commission_cents + expert_amount_cents - amount_cents) AS diff").
		Where("deleted_at IS NULL AND commission_cents + expert_amount_cents <> amount_cents").
		Order("id DESC").Limit(checkSampleLimit).
		Scan(&rows).Error
	if err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("校验分账金额失败: %w", err))
	}
	c.OK = len(rows) == 0
	if c.OK {
		c.Message = fmt.Sprintf("%d 条分账记录全部平账", total)
	} else {
		for _, x := range rows {
			c.Samples = append(c.Samples, fmt.Sprintf("%s（订单 %s）差额 %s", x.SettlementNo, x.OrderNo, signedYuan(x.Diff)))
		}
		c.Message = fmt.Sprintf("发现 %d 条分账记录不平", len(rows))
	}
	return c, nil
}

// checkWithdrawIdentity 提现守恒：实际到账 == 申请金额 - 手续费。
//
// 不平意味着给专家多打或少打了钱，而且打款流水与提现单对不上，
// 后面查账只能靠银行回单逐笔猜。
func (r *FinanceReportRepo) checkWithdrawIdentity(ctx context.Context) (FinanceCheck, error) {
	c := FinanceCheck{
		Key:  "withdrawal_identity",
		Name: "提现守恒（实际到账 = 申请金额 − 手续费）",
	}
	var total int64
	if err := r.db.WithContext(ctx).Table("withdrawals").
		Where("deleted_at IS NULL").Count(&total).Error; err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("统计提现记录失败: %w", err))
	}
	c.Checked = total

	var rows []struct {
		WithdrawalNo string
		AmountCents  int64
		ActualCents  int64
	}
	err := r.db.WithContext(ctx).Table("withdrawals").
		Select("withdrawal_no, amount_cents, actual_cents").
		Where("deleted_at IS NULL AND actual_cents <> amount_cents - fee_cents").
		Order("id DESC").Limit(checkSampleLimit).
		Scan(&rows).Error
	if err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("校验提现金额失败: %w", err))
	}
	c.OK = len(rows) == 0
	if c.OK {
		c.Message = fmt.Sprintf("%d 笔提现记录金额自洽", total)
	} else {
		for _, x := range rows {
			c.Samples = append(c.Samples, fmt.Sprintf("%s 申请 %s，到账 %s",
				x.WithdrawalNo, yuanText(x.AmountCents), yuanText(x.ActualCents)))
		}
		c.Message = fmt.Sprintf("发现 %d 笔提现的到账金额与申请金额对不上", len(rows))
	}
	return c, nil
}

// checkExpertLedger 专家台账：累计收入 == 可提余额 + 冻结 + 已打款。
//
// 这是整个资金链的总校验。四个动作各自改两个字段：
//
//	结算      total_income += x, balance += x
//	申请提现  balance -= x, frozen += x
//	已打款    frozen -= x
//	驳回/失败 frozen -= x, balance += x
//
// 所以任何时刻都该有 total_income == balance + frozen + 已打款。
// 不平就说明上面某一步只改了一半——这是最难靠人工发现、也最伤钱的一类错误。
func (r *FinanceReportRepo) checkExpertLedger(ctx context.Context) (FinanceCheck, error) {
	c := FinanceCheck{
		Key:  "expert_ledger",
		Name: "专家台账（累计收入 = 可提余额 + 冻结 + 已打款）",
	}

	var rows []struct {
		UserID      int64
		RealName    string
		TotalIncome int64
		Balance     int64
		Frozen      int64
		PaidOut     int64
		PendingOut  int64
	}
	err := r.db.WithContext(ctx).
		Table("experts AS e").
		Select(`e.user_id, e.real_name, e.total_income_cents AS total_income,
			e.balance_cents AS balance, e.frozen_cents AS frozen,
			COALESCE((SELECT SUM(w.amount_cents) FROM withdrawals w
				WHERE w.expert_id = e.user_id AND w.status = ? AND w.deleted_at IS NULL), 0) AS paid_out,
			COALESCE((SELECT SUM(w.amount_cents) FROM withdrawals w
				WHERE w.expert_id = e.user_id AND w.status IN ? AND w.deleted_at IS NULL), 0) AS pending_out`,
			model.WithdrawalStatusPaid,
			[]string{model.WithdrawalStatusPending, model.WithdrawalStatusApproved}).
		Where("e.deleted_at IS NULL").
		Limit(5000).
		Scan(&rows).Error
	if err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("校验专家台账失败: %w", err))
	}
	c.Checked = int64(len(rows))

	for _, x := range rows {
		want := x.Balance + x.Frozen + x.PaidOut
		if x.TotalIncome == want {
			continue
		}
		// 只打印差异，不打全量数字：运维看到"差 -30.00"比看四个数字相加更快。
		c.Samples = append(c.Samples, fmt.Sprintf("专家 #%d %s：累计收入 %s，应为 %s（差额 %s）",
			x.UserID, x.RealName, yuanText(x.TotalIncome), yuanText(want), signedYuan(x.TotalIncome-want)))
		if len(c.Samples) >= checkSampleLimit {
			break
		}
	}
	c.OK = len(c.Samples) == 0
	if c.OK {
		c.Message = fmt.Sprintf("%d 位专家的台账全部平账", len(rows))
	} else {
		c.Message = fmt.Sprintf("发现 %d 位专家的台账不平", len(c.Samples))
	}
	return c, nil
}

// checkSettledHasSettlement 已结算订单必须有分账记录。
//
// 状态到 settled 但没有 settlements 行，说明专家一分钱没收到而订单已经关掉，
// 事后只能靠这条检查发现——用户和专家都不会来报这个错。
func (r *FinanceReportRepo) checkSettledHasSettlement(ctx context.Context) (FinanceCheck, error) {
	c := FinanceCheck{
		Key:  "settled_has_settlement",
		Name: "已结算订单都有分账记录",
	}
	var total int64
	if err := r.db.WithContext(ctx).Table("orders").
		Where("deleted_at IS NULL AND status = ?", model.OrderStatusSettled).
		Count(&total).Error; err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("统计已结算订单失败: %w", err))
	}
	c.Checked = total

	var rows []struct {
		OrderNo string
	}
	err := r.db.WithContext(ctx).
		Table("orders AS o").
		Select("o.order_no").
		Joins("LEFT JOIN settlements AS s ON s.order_id = o.id AND s.deleted_at IS NULL").
		Where("o.deleted_at IS NULL AND o.status = ? AND s.id IS NULL", model.OrderStatusSettled).
		Order("o.id DESC").Limit(checkSampleLimit).
		Scan(&rows).Error
	if err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("校验结算记录失败: %w", err))
	}
	c.OK = len(rows) == 0
	if c.OK {
		c.Message = fmt.Sprintf("%d 笔已结算订单都有分账记录", total)
	} else {
		for _, x := range rows {
			c.Samples = append(c.Samples, x.OrderNo)
		}
		c.Message = fmt.Sprintf("发现 %d 笔订单已置为已结算但没有分账记录（专家没收到钱）", len(rows))
	}
	return c, nil
}

// checkPaidBeforeService 进入服务前必须真的收到钱。
//
// in_service 及之后的状态意味着平台已经承诺交付。paid_at 为空就说明
// 有人绕过了支付直接推状态（手工改库、或某条写路径漏写 paid_at），
// 这种单子结算时会凭空给专家加余额——钱从空气里来。
func (r *FinanceReportRepo) checkPaidBeforeService(ctx context.Context) (FinanceCheck, error) {
	c := FinanceCheck{
		Key:  "paid_before_service",
		Name: "进入服务的订单都已收款",
	}
	stages := []string{
		model.OrderStatusInService, model.OrderStatusPendingConfirm,
		model.OrderStatusCompleted, model.OrderStatusSettled,
	}
	var total int64
	if err := r.db.WithContext(ctx).Table("orders").
		Where("deleted_at IS NULL AND status IN ?", stages).
		Count(&total).Error; err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("统计服务中订单失败: %w", err))
	}
	c.Checked = total

	var rows []struct {
		OrderNo string
		Status  string
	}
	err := r.db.WithContext(ctx).Table("orders").
		Select("order_no, status").
		Where("deleted_at IS NULL AND paid_at IS NULL AND status IN ?", stages).
		Order("id DESC").Limit(checkSampleLimit).
		Scan(&rows).Error
	if err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("校验收款状态失败: %w", err))
	}
	c.OK = len(rows) == 0
	if c.OK {
		c.Message = fmt.Sprintf("%d 笔进入服务的订单都有支付时间", total)
	} else {
		for _, x := range rows {
			c.Samples = append(c.Samples, fmt.Sprintf("%s（%s）没有支付时间", x.OrderNo, x.Status))
		}
		c.Message = fmt.Sprintf("发现 %d 笔订单已进入服务却没有支付时间", len(rows))
	}
	return c, nil
}

// checkRefundTimeMissing 有退款金额却没有退款时间的历史单。
//
// `refunded_at` 是后加的字段，之前退掉的单子补不出真实时间。
// 这笔钱进得了累计口径（按金额统计），却进不了按日统计——
// 不说清楚的话，财务会以为某个月的退款少记了，然后去找一笔根本不存在的差错。
// 定位后可以手工回填一条 UPDATE，SQL 写在 README 的运维小节里。
func (r *FinanceReportRepo) checkRefundTimeMissing(ctx context.Context) (FinanceCheck, error) {
	c := FinanceCheck{
		Key:   "refund_time_missing",
		Level: CheckLevelWarn,
		Name:  "历史退款可归入日报表（退款时间齐全）",
	}
	var total int64
	if err := r.db.WithContext(ctx).Table("orders").
		Where("deleted_at IS NULL AND refund_cents > 0").
		Count(&total).Error; err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("统计退款订单失败: %w", err))
	}
	c.Checked = total

	var missing int64
	if err := r.db.WithContext(ctx).Table("orders").
		Where("deleted_at IS NULL AND refund_cents > 0 AND refunded_at IS NULL").
		Count(&missing).Error; err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("统计缺失退款时间的订单失败: %w", err))
	}
	c.OK = missing == 0
	if c.OK {
		c.Message = fmt.Sprintf("%d 笔退款订单都有退款时间", total)
		return c, nil
	}

	var rows []struct {
		OrderNo     string
		RefundCents int64
	}
	if err := r.db.WithContext(ctx).Table("orders").
		Select("order_no, refund_cents").
		Where("deleted_at IS NULL AND refund_cents > 0 AND refunded_at IS NULL").
		Order("id DESC").Limit(checkSampleLimit).
		Scan(&rows).Error; err != nil {
		return c, errcode.ErrDatabase.Wrap(fmt.Errorf("查询缺失退款时间的订单失败: %w", err))
	}
	for _, x := range rows {
		c.Samples = append(c.Samples, fmt.Sprintf("%s 退款 %s", x.OrderNo, yuanText(x.RefundCents)))
	}
	c.Message = fmt.Sprintf("有 %d 笔历史退款没有退款时间：已计入累计，但不进按日统计", missing)
	return c, nil
}

/* ============================ 当日明细 ============================ */
// FinanceDayDetail 某一天的明细清单，供页面下钻。
//
// 三组分开返回而不是拼成一张统一列表：它们的字段完全不同
// （订单看双方、结算看抽佣拆分、提现看收款账户），硬拼会得到一张谁都不好读的表。
type FinanceDayDetail struct {
	Day         string                 `json:"day"`
	Orders      []FinanceDayOrder      `json:"orders"`
	Settlements []FinanceDaySettlement `json:"settlements"`
	Withdrawals []FinanceDayWithdrawal `json:"withdrawals"`
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

// DayDetail 取某一天的三组明细。
//
// 单日上限 500 条：一天超过这个量说明不是日常经营数据，
// 先分页或导 CSV 更合适，一次性拉给浏览器没有意义。
func (r *FinanceReportRepo) DayDetail(ctx context.Context, day time.Time) (*FinanceDayDetail, error) {
	lo, hi := boundPair(day, day)
	out := &FinanceDayDetail{
		Day:         day.Format("2006-01-02"),
		Orders:      make([]FinanceDayOrder, 0),
		Settlements: make([]FinanceDaySettlement, 0),
		Withdrawals: make([]FinanceDayWithdrawal, 0),
	}

	// 收款与退款可能落在同一天（当天下单当天退），所以用 OR 取并集。
	err := r.db.WithContext(ctx).Table("orders").
		Select("order_no, title, status, amount_cents, commission_cents, expert_amount_cents, refund_cents, paid_at, refunded_at").
		Where(`deleted_at IS NULL AND (
			(paid_at IS NOT NULL AND paid_at >= ? AND paid_at < ?) OR
			(refunded_at IS NOT NULL AND refunded_at >= ? AND refunded_at < ?))`, lo, hi, lo, hi).
		Order("id DESC").Limit(500).
		Scan(&out.Orders).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询当日订单失败: %w", err))
	}

	err = r.db.WithContext(ctx).Table("settlements").
		Select("settlement_no, order_no, expert_id, amount_cents, commission_cents, expert_amount_cents, settled_at").
		Where("deleted_at IS NULL AND status = ? AND settled_at >= ? AND settled_at < ?",
			model.SettlementStatusSettled, lo, hi).
		Order("id DESC").Limit(500).
		Scan(&out.Settlements).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询当日结算失败: %w", err))
	}

	err = r.db.WithContext(ctx).Table("withdrawals").
		Select("withdrawal_no, expert_id, amount_cents, fee_cents, actual_cents, account_name, account_no_masked, paid_at").
		Where("deleted_at IS NULL AND status = ? AND paid_at >= ? AND paid_at < ?",
			model.WithdrawalStatusPaid, lo, hi).
		Order("id DESC").Limit(500).
		Scan(&out.Withdrawals).Error
	if err != nil {
		return nil, errcode.ErrDatabase.Wrap(fmt.Errorf("查询当日打款失败: %w", err))
	}
	return out, nil
}

/* ============================ 小工具 ============================ */

// normalizeDay 校验 SQL 切出来的日期。
//
// 这里宁可报错也不静默丢行：日期解析不出来说明时间列的格式变了
// （换库、换驱动、有人手工写过脏数据），丢一行就是丢一天的钱，
// 而报表照样"看起来正常"。报错让人立刻知道账算不出来。
func normalizeDay(raw string) (string, error) {
	s := strings.TrimSpace(raw)
	if len(s) >= 10 {
		s = s[:10]
	}
	if _, err := time.Parse("2006-01-02", s); err != nil {
		return "", errcode.ErrDatabase.Wrap(fmt.Errorf("时间列格式无法识别（%q），对账报表无法按日聚合", raw))
	}
	return s, nil
}

// yuanText 把分转成"¥123.45"，只用于校验样本的可读性。
func yuanText(cents int64) string {
	sign := ""
	if cents < 0 {
		sign = "-"
		cents = -cents
	}
	return fmt.Sprintf("%s¥%d.%02d", sign, cents/100, cents%100)
}

// signedYuan 带符号的差额，正数补 +。
func signedYuan(cents int64) string {
	if cents > 0 {
		return "+" + yuanText(cents)
	}
	return yuanText(cents)
}
