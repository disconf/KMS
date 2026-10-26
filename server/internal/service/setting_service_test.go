package service

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/repository"
)

// dtoSkill 简化测试里的技能入参构造。
func dtoSkill(name, category string, sort int) dto.AdminSkillCreateReq {
	return dto.AdminSkillCreateReq{Name: name, Category: category, Sort: sort}
}

/* ============================ 抽佣比例配置 ============================ */

func TestSetCommissionRateBPRejectsOutOfRange(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	for _, bad := range []int{-1, 5001, 100000} {
		_, err := f.svc.settings.SetCommissionRateBP(ctx, f.adminID, model.RoleAdmin, bad, AuditMeta{})
		assert.ErrorIs(t, err, errcode.ErrParamInvalid, "基点 %d 应被拒绝", bad)
	}

	// 边界值本身合法：0 表示平台不抽佣，5000 是 50% 上限。
	for _, ok := range []int{0, 5000} {
		_, err := f.svc.settings.SetCommissionRateBP(ctx, f.adminID, model.RoleAdmin, ok, AuditMeta{})
		require.NoError(t, err, "基点 %d 应被接受", ok)
	}
}

func TestSetCommissionRateBPSameValueWritesNothing(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	// 默认 1500，改成 1500 应当幂等：不写库、不留审计。
	view, err := f.svc.settings.SetCommissionRateBP(ctx, f.adminID, model.RoleAdmin, 1500, AuditMeta{})
	require.NoError(t, err)
	assert.False(t, view.IsOverride, "值没变不该产生覆盖记录")

	var count int64
	require.NoError(t, f.db.Model(&model.AuditLog{}).
		Where("action = ?", AuditActionSettingCommission).Count(&count).Error)
	assert.Zero(t, count, "值没变不该产生审计噪声")
}

func TestSetCommissionRateBPRecordsAuditWithBeforeAndAfter(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	_, err := f.svc.settings.SetCommissionRateBP(ctx, f.adminID, model.RoleAdmin, 1800, AuditMeta{
		IP: "10.0.0.8", UserAgent: "smoke-test", RequestID: "req-1",
	})
	require.NoError(t, err)

	var logs []model.AuditLog
	require.NoError(t, f.db.Where("action = ?", AuditActionSettingCommission).Find(&logs).Error)
	require.Len(t, logs, 1)
	assert.Equal(t, AuditTargetSetting, logs[0].TargetType)
	assert.EqualValues(t, f.adminID, logs[0].UserID)
	assert.EqualValues(t, 1500, logs[0].Detail["from"], "审计必须带改之前的值")
	assert.EqualValues(t, 1800, logs[0].Detail["to"])
	assert.Equal(t, "10.0.0.8", logs[0].IP)

	// 再查一次，页面上的「已覆盖」状态要成立。
	overview, err := f.svc.settings.Overview(ctx)
	require.NoError(t, err)
	assert.True(t, overview.Commission.IsOverride)
	assert.Equal(t, 1800, overview.Commission.RateBP)
	assert.Equal(t, 1500, overview.Commission.DefaultRateBP)
}

// 库里被手工改成脏值时不能拖垮下单：回落默认值并记日志。
func TestGetCommissionRateBPFallsBackOnCorruptValue(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()

	require.NoError(t, f.db.Create(&model.SystemSetting{
		Key: model.SettingKeyCommissionRateBP, Value: "15%", UpdatedBy: f.adminID,
	}).Error)

	bp, err := f.svc.settings.GetCommissionRateBP(ctx)
	require.NoError(t, err)
	assert.Equal(t, 1500, bp)
}

/* ============================ 技能字典 ============================ */

func TestSkillAdminCrud(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	svc := NewSkillAdminService(repository.NewSkillRepo(f.db))

	created, err := svc.Create(ctx, dtoSkill("etcd", "云原生", 10))
	require.NoError(t, err)
	assert.NotZero(t, created.ID)
	assert.True(t, created.Enabled, "默认启用")

	// 同名（含前后空格）必须被拒：这是唯一索引在应用层的友好提示。
	_, err = svc.Create(ctx, dtoSkill("  etcd ", "云原生", 20))
	assert.ErrorIs(t, err, errcode.ErrSkillTaken)

	// 改名 + 停用。
	newName := "etcd 集群"
	disabled := false
	updated, err := svc.Update(ctx, created.ID, dto.AdminSkillUpdateReq{
		Name: &newName, Enabled: &disabled,
	})
	require.NoError(t, err)
	assert.Equal(t, "etcd 集群", updated.Name)
	assert.False(t, updated.Enabled)

	list, err := svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, int64(0), list[0].ExpertCount)

	require.NoError(t, svc.Delete(ctx, created.ID))

	// 物理删除的验证：同名可以立刻重建。若退化成软删除，
	// 唯一索引上的旧行会让这次创建静默撞索引。
	_, err = svc.Create(ctx, dtoSkill("etcd 集群", "云原生", 30))
	require.NoError(t, err, "删除后应能用同名重建")
}

func TestSkillAdminRejectsEmptyCategory(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	svc := NewSkillAdminService(repository.NewSkillRepo(f.db))

	_, err := svc.Create(ctx, dtoSkill("Ceph", "  ", 1))
	assert.ErrorIs(t, err, errcode.ErrParamInvalid)
}

// 被专家档案引用的技能不能删：删掉后专家页与搜索里会出现悬空 ID。
// 停用是安全出口，删除不是。
func TestSkillDeleteBlockedByExpertReference(t *testing.T) {
	f := newOrderFixture(t)
	ctx := context.Background()
	skills := repository.NewSkillRepo(f.db)
	svc := NewSkillAdminService(skills)

	item, err := svc.Create(ctx, dtoSkill("Istio", "云原生", 10))
	require.NoError(t, err)

	// 复用 fixture 里已有的专家档案：experts.user_id 有唯一索引，
	// 再造一个同 user 的档案会撞约束（这本身也是对的行为）。
	expert, err := f.experts.FindByUserID(ctx, f.expertID)
	require.NoError(t, err)
	require.NoError(t, f.db.Create(&model.ExpertSkill{
		ExpertID: expert.ID, SkillID: item.ID,
		Level: model.SkillLevelSenior, Years: 5,
	}).Error)

	err = svc.Delete(ctx, item.ID)
	assert.ErrorIs(t, err, errcode.ErrSkillInUse)

	list, err := svc.List(ctx)
	require.NoError(t, err)
	require.Len(t, list, 1)
	assert.Equal(t, int64(1), list[0].ExpertCount, "列表要显示被几位专家引用")

	// 停用是可用的替代方案。
	disabled := false
	_, err = svc.Update(ctx, item.ID, dto.AdminSkillUpdateReq{Enabled: &disabled})
	require.NoError(t, err)
}
