package errcode

import (
	"errors"
	"fmt"
	"net/http"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestFromNormalizesUnknownError(t *testing.T) {
	unknown := fmt.Errorf("底层连接被重置")

	got := From(unknown)
	require.NotNil(t, got)
	// 未知错误必须被收敛为内部错误，避免把底层细节直接抛给前端。
	assert.Equal(t, ErrInternal.Code, got.Code)
	assert.Equal(t, http.StatusInternalServerError, got.HTTP())
	// 原始原因仍要保留，便于日志追溯。
	assert.ErrorIs(t, got, unknown)
}

func TestFromKeepsBusinessError(t *testing.T) {
	got := From(ErrSMSCodeInvalid)
	assert.Equal(t, ErrSMSCodeInvalid.Code, got.Code)
	assert.Equal(t, http.StatusBadRequest, got.HTTP())
}

func TestFromNilReturnsOK(t *testing.T) {
	assert.Equal(t, OK.Code, From(nil).Code)
}

func TestWrapAndIs(t *testing.T) {
	cause := fmt.Errorf("dial tcp: i/o timeout")
	wrapped := ErrDatabase.Wrap(cause)

	require.Error(t, wrapped)
	assert.ErrorIs(t, wrapped, cause, "Wrap 后应能追溯到原始错误")
	assert.ErrorIs(t, wrapped, ErrDatabase, "同码错误应可用 errors.Is 判定")
	assert.Equal(t, http.StatusInternalServerError, wrapped.HTTP())
}

func TestWithMsgDoesNotMutateOriginal(t *testing.T) {
	original := ErrForbidden

	modified := original.WithMsg("当前角色无权访问该接口")

	assert.Equal(t, ErrForbidden.Msg, original.Msg, "原始错误定义不能被修改")
	assert.Equal(t, "当前角色无权访问该接口", modified.Msg)
	assert.Equal(t, original.Code, modified.Code)
}

func TestHTTPFallback(t *testing.T) {
	// 直接构造而不走 New：New 会往注册表里登记，拿一个假码去登记会污染全量校验。
	mystery := &Error{Code: 12345, Msg: "未知"}
	assert.Equal(t, http.StatusInternalServerError, mystery.HTTP())
}

func TestNewf(t *testing.T) {
	e := Newf(31000, http.StatusBadRequest, "手机号 %s 已被占用", "138****0000")
	assert.Equal(t, 31000, e.Code)
	assert.Equal(t, http.StatusBadRequest, e.HTTP())
	assert.Contains(t, e.Msg, "138****0000")
}

// segmentOf 按错误码反推它属于哪一段（码值 / 10000）。
func segmentOf(code int) int { return code / 10000 }

func TestAllPredefinedCodesAreUnique(t *testing.T) {
	all := All()

	// 注册表本身不能是空的——否则这个测试会在"没有任何错误码"时假装通过。
	require.Greater(t, len(all), 50, "注册到的错误码太少，New 的登记逻辑可能坏了")

	seen := make(map[int]string, len(all))
	for _, e := range all {
		require.NotNil(t, e)
		if prev, dup := seen[e.Code]; dup {
			t.Fatalf("错误码 %d 重复定义：%s 与 %s", e.Code, prev, e.Msg)
		}
		seen[e.Code] = e.Msg
		assert.NotEmpty(t, e.Msg, "错误码 %d 缺少用户可读提示", e.Code)
		assert.NotZero(t, e.HTTP(), "错误码 %d 没设 HTTP 状态码", e.Code)
	}
}

// TestCodesAreInDeclaredSegments 守住分段约定。
//
// 分段是给人看的索引：handler 与前端都靠它判断"这是哪一类问题"。
// 把订单错误码写成 4xxxx 不会让程序出错，但会让日志里 4xxxx 突然混进订单语义，
// 排查时先要花时间辨认码值属于谁——这种错只有靠断言挡。
func TestCodesAreInDeclaredSegments(t *testing.T) {
	valid := map[int]string{
		1: "通用",
		2: "认证与权限",
		3: "用户与专家",
		4: "知识库",
		5: "问题与订单",
		6: "支付与结算",
		7: "文件与存储",
		9: "内部依赖",
	}

	for _, e := range All() {
		if e.Code == OK.Code {
			continue // 0 是成功码，不属于任何错误分段
		}
		seg := segmentOf(e.Code)
		_, ok := valid[seg]
		assert.Truef(t, ok,
			"错误码 %d（%s）不在任何已声明的分段里；新增分段要同时更新本表与包注释", e.Code, e.Msg)
	}
}

// TestSegmentsAreActuallyPopulated 确认每个声明过的分段都真的有码。
//
// 空分段意味着"这层还没写"——本文件里的 5xxxx/6xxxx 曾经长期是空的，
// 而包注释与 README 都写着订单与支付已完成，两边对不上却没人发现。
func TestSegmentsAreActuallyPopulated(t *testing.T) {
	count := map[int]int{}
	for _, e := range All() {
		count[segmentOf(e.Code)]++
	}

	for _, seg := range []int{1, 2, 3, 4, 5, 6, 7, 9} {
		assert.NotZerof(t, count[seg], "%dxxxx 分段一个错误码都没有", seg)
	}
}

func TestErrorsAsFindsCause(t *testing.T) {
	wrapped := fmt.Errorf("上层包装: %w", ErrParamInvalid)

	var target *Error
	require.True(t, errors.As(wrapped, &target))
	assert.Equal(t, ErrParamInvalid.Code, target.Code)
}
