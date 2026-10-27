// Package errcode 定义平台统一的业务错误码。
//
// 约定：
//   - 0        成功
//   - 1xxxx    通用错误
//   - 2xxxx    认证与权限
//   - 3xxxx    用户与专家
//   - 4xxxx    知识库
//   - 5xxxx    问题与订单
//   - 6xxxx    支付与结算
//   - 9xxxx    内部依赖（数据库、缓存、第三方）
//
// 每个错误同时携带建议的 HTTP 状态码，handler 层直接使用，不需要再判断。
package errcode

import (
	"errors"
	"fmt"
	"net/http"
)

// Error 是平台的业务错误类型，实现 error 接口并支持 errors.Is / errors.As。
type Error struct {
	Code    int    // 业务码
	Msg     string // 面向用户的提示（可直接展示）
	Status  int    // 建议 HTTP 状态码
	Message string // 面向开发的详细说明，不返回给前端
	cause   error  // 底层原因
}

func (e *Error) Error() string {
	if e.cause != nil {
		return fmt.Sprintf("[%d] %s: %v", e.Code, e.Msg, e.cause)
	}
	return fmt.Sprintf("[%d] %s", e.Code, e.Msg)
}

// Unwrap 支持 errors.Is / errors.As 向上追溯。
func (e *Error) Unwrap() error { return e.cause }

// Is 让同 Code 的错误可被 errors.Is 判定相等。
func (e *Error) Is(target error) bool {
	var t *Error
	if errors.As(target, &t) {
		return t.Code == e.Code
	}
	return false
}

// HTTP 返回建议的 HTTP 状态码，未设置时兜底 500。
func (e *Error) HTTP() int {
	if e.Status == 0 {
		return http.StatusInternalServerError
	}
	return e.Status
}

// New 创建一个业务错误。
//
// New 只用于在包级变量里定义错误码常量，每次调用都会登记到注册表，
// 供 errcode_test.go 做"码值唯一 + 分段正确"的全量校验。
// 需要包装运行时错误时用 Wrap / WithCause，不要用 New——那会往注册表里灌垃圾。
func New(code int, msg string, status int) *Error {
	e := &Error{Code: code, Msg: msg, Status: status}
	registry = append(registry, e)
	return e
}

// registry 记录本包定义过的全部错误码。
//
// 之所以要自动登记而不是在测试里维护一份清单：清单一定会漂移。
// 本文件此前有 25 个错误码从没进过测试清单（题库、资源、文件、专家详情那几段），
// 唯一性校验看着是绿的，实际只覆盖了一半——手工维护的校验清单本身就是个坑。
var registry []*Error

// All 返回本包定义过的全部错误码（不含 Newf 临时构造的）。
// 供测试与文档生成使用。
func All() []*Error {
	out := make([]*Error, len(registry))
	copy(out, registry)
	return out
}

// Newf 创建一个带格式化提示的业务错误。
func Newf(code int, status int, format string, args ...any) *Error {
	return &Error{Code: code, Msg: fmt.Sprintf(format, args...), Status: status}
}

// Wrap 复制错误并附加底层原因（不会污染原始定义）。
func (e *Error) Wrap(cause error) *Error {
	c := *e
	c.cause = cause
	return &c
}

// WithCause 是 Wrap 的链式别名，便于在一行内书写。
func (e *Error) WithCause(cause error) *Error { return e.Wrap(cause) }

// WithMsg 复制错误并替换面向用户的提示。
func (e *Error) WithMsg(msg string) *Error {
	c := *e
	c.Msg = msg
	return &c
}

// ============================ 通用 1xxxx ============================

var (
	OK                = New(0, "ok", http.StatusOK)
	ErrParamInvalid   = New(10001, "请求参数不合法", http.StatusBadRequest)
	ErrParamMissing   = New(10002, "缺少必要参数", http.StatusBadRequest)
	ErrNotFound       = New(10003, "资源不存在", http.StatusNotFound)
	ErrMethodInvalid  = New(10004, "请求方法不被支持", http.StatusMethodNotAllowed)
	ErrTooManyReqs    = New(10005, "操作过于频繁，请稍后再试", http.StatusTooManyRequests)
	ErrInternal       = New(10006, "服务内部错误，请稍后重试", http.StatusInternalServerError)
	ErrNotImplemented = New(10007, "该功能尚未实现", http.StatusNotImplemented)
)

// ============================ 认证与权限 2xxxx ============================

var (
	ErrUnauthorized     = New(20001, "请先登录", http.StatusUnauthorized)
	ErrTokenExpired     = New(20002, "登录状态已过期，请重新登录", http.StatusUnauthorized)
	ErrTokenInvalid     = New(20003, "登录凭证无效", http.StatusUnauthorized)
	ErrForbidden        = New(20004, "没有操作权限", http.StatusForbidden)
	ErrSMSCodeInvalid   = New(20005, "验证码不正确", http.StatusBadRequest)
	ErrSMSCodeExpired   = New(20006, "验证码已过期，请重新获取", http.StatusBadRequest)
	ErrSMSSendTooFast   = New(20007, "验证码发送过于频繁，请稍后再试", http.StatusTooManyRequests)
	ErrSMSDailyLimit    = New(20008, "今日验证码发送次数已达上限", http.StatusTooManyRequests)
	ErrUserDisabled     = New(20009, "账号已被禁用，请联系客服", http.StatusForbidden)
	ErrPhoneInvalid     = New(20010, "手机号格式不正确", http.StatusBadRequest)
	ErrWechatAuthFailed = New(20011, "微信授权失败，请重试", http.StatusBadRequest)
	ErrRefreshInvalid   = New(20012, "登录凭证已失效，请重新登录", http.StatusUnauthorized)
	ErrAccountNotFound  = New(20013, "账号不存在", http.StatusNotFound)

	// ErrPasswordInvalid 手机号不存在与口令错误共用同一条文案。
	// 分开提示会把"这个手机号有后台账号"直接告诉扫描器。
	ErrPasswordInvalid = New(20014, "手机号或密码不正确", http.StatusUnauthorized)
	ErrPasswordTooWeak = New(20015, "密码强度不足：至少 8 位且同时包含字母和数字", http.StatusBadRequest)
	ErrPasswordNotSet  = New(20016, "该账号未设置密码，请使用验证码登录", http.StatusBadRequest)
	ErrLoginLocked     = New(20017, "密码错误次数过多，请 15 分钟后再试", http.StatusTooManyRequests)

	// 邮箱注册与登录。文案里不区分"邮箱已注册/未注册"以外的细节，
	// 但注册与登录两条路径的提示必须不同：注册要说清"去登录"，
	// 否则用户会以为换个邮箱就能重复注册。
	ErrEmailInvalid      = New(20018, "邮箱格式不正确", http.StatusBadRequest)
	ErrEmailTaken        = New(20019, "该邮箱已注册，请直接登录", http.StatusConflict)
	ErrEmailNotVerified  = New(20020, "请先完成邮箱验证", http.StatusForbidden)
	ErrEmailCodeInvalid  = New(20021, "邮箱验证码不正确", http.StatusBadRequest)
	ErrEmailSendTooFast  = New(20022, "验证码已发送，请稍后再试", http.StatusTooManyRequests)
	ErrEmailDailyLimit   = New(20023, "今日邮件发送次数已达上限", http.StatusTooManyRequests)
	ErrMailNotConfigured = New(20024, "邮件服务未配置，暂时无法注册，请联系管理员", http.StatusNotImplemented)
	ErrRegisterDisabled  = New(20025, "注册通道已关闭", http.StatusForbidden)

	// 发言权限。只读账号点任何"发布"按钮都会撞到这条，
	// 所以文案必须直接告诉他下一步该干什么。
	ErrSpeakNotAllowed  = New(20026, "当前账号为只读状态，请先提交实名申请并通过审核", http.StatusForbidden)
	ErrSpeakApplyDup    = New(20027, "已提交申请，请等待审核", http.StatusConflict)
	ErrSpeakApplyState  = New(20028, "该申请已审核完成，不可重复审核", http.StatusConflict)
	ErrSpeakApplyAbsent = New(20029, "请先提交实名申请", http.StatusForbidden)
	ErrSpeakApplyNote   = New(20030, "请填写申请理由", http.StatusBadRequest)
)

// ============================ 用户与专家 3xxxx ============================

var (
	ErrNicknameInvalid   = New(30001, "昵称不合法", http.StatusBadRequest)
	ErrExpertNotApplied  = New(30002, "请先提交专家入驻申请", http.StatusForbidden)
	ErrExpertUnderReview = New(30003, "专家资质审核中，请耐心等待", http.StatusForbidden)
	ErrExpertRejected    = New(30004, "专家资质审核未通过", http.StatusForbidden)
	ErrExpertSuspended   = New(30005, "专家账号已被停牌", http.StatusForbidden)
	ErrExpertApplyDup    = New(30006, "已提交过入驻申请", http.StatusConflict)

	ErrRealNameInvalid     = New(30007, "真实姓名不合法", http.StatusBadRequest)
	ErrIDCardInvalid       = New(30008, "身份证号格式不正确", http.StatusBadRequest)
	ErrCertRequired        = New(30009, "请至少上传一份资质证书", http.StatusBadRequest)
	ErrSkillInvalid        = New(30010, "技能标签不合法", http.StatusBadRequest)
	ErrPriceInvalid        = New(30011, "定价不合法", http.StatusBadRequest)
	ErrExpertNotFound      = New(30012, "专家不存在", http.StatusNotFound)
	ErrExpertNotAccepting  = New(30013, "该专家当前不接单", http.StatusConflict)
	ErrExpertAudited       = New(30014, "该申请已审核完成，不可重复审核", http.StatusConflict)
	ErrAuditRemarkRequired = New(30015, "驳回申请时必须填写原因", http.StatusBadRequest)
	ErrServiceModeInvalid  = New(30016, "服务方式不合法", http.StatusBadRequest)
	ErrIntroInvalid        = New(30017, "个人简介不合法", http.StatusBadRequest)

	ErrBalanceInsufficient    = New(30018, "可提现余额不足", http.StatusBadRequest)
	ErrWithdrawAmountInvalid  = New(30019, "提现金额不合法", http.StatusBadRequest)
	ErrWithdrawAccountInvalid = New(30020, "收款账户信息不合法", http.StatusBadRequest)
	ErrWithdrawPending        = New(30021, "已有待处理的提现申请，请等待处理完成", http.StatusConflict)
	// 后台审批用。状态冲突用 409 而不是 400：用户没有填错，是这笔单子已经被别人动过了。
	ErrWithdrawalNotFound     = New(30022, "提现申请不存在", http.StatusNotFound)
	ErrWithdrawalStateInvalid = New(30023, "提现状态已变化，请刷新后重试", http.StatusConflict)

	// 账号治理。这里的错误会出现在已登录用户的正常请求上（封禁是即时生效的），
	// 所以文案要能直接给用户看，且 403 而不是 401——重新登录解决不了封禁，
	// 前端据此提示"联系客服"而不是傻循环刷新令牌。
	ErrAccountBanned    = New(30024, "账号已被封禁，如有疑问请联系平台客服", http.StatusForbidden)
	ErrUserSelfOperate  = New(30025, "不能对自己的账号执行该操作", http.StatusBadRequest)
	ErrLastAdmin        = New(30026, "至少保留一个可用的管理员账号", http.StatusBadRequest)
	ErrStatusReasonReq  = New(30027, "封禁账号必须填写原因", http.StatusBadRequest)
	ErrUserStateChanged = New(30028, "账号状态已变化，请刷新后重试", http.StatusConflict)
	// 手工把角色改成 expert 会绕开入驻审核：RequireApprovedExpert 对空专家状态放行，
	// 于是一个没提交过申请的账号可以直接进专家工作台接单。这里必须堵住。
	ErrRoleNeedsExpert = New(30029, "该用户尚未通过专家入驻审核，不能直接设为专家", http.StatusBadRequest)

	// 技能字典由运营维护，但删除要拦一道：被专家档案引用的技能删掉后，
	// 专家页与搜索里就出现悬空 ID。有引用时拒绝并提示改用「停用」。
	ErrSkillNotFound = New(30030, "技能不存在", http.StatusNotFound)
	ErrSkillTaken    = New(30031, "技能名称已存在", http.StatusConflict)
	ErrSkillInUse    = New(30032, "该技能已被专家档案引用，不能删除；如需下架请改用停用", http.StatusConflict)
)

// ============================ 知识库 4xxxx ============================

var (
	ErrArticleNotFound     = New(40001, "文章不存在", http.StatusNotFound)
	ErrArticleNotPublished = New(40002, "文章不存在或已下线", http.StatusNotFound)
	ErrCategoryNotFound    = New(40003, "分类不存在", http.StatusNotFound)
	ErrCategoryNameTaken   = New(40004, "分类名称已存在", http.StatusConflict)
	ErrCategorySlugTaken   = New(40005, "分类标识已存在", http.StatusConflict)
	ErrCategoryInUse       = New(40006, "该分类下仍有文章，请先移出或删除文章", http.StatusConflict)
	ErrArticleStateInvalid = New(40007, "文章当前状态不支持该操作", http.StatusConflict)
	ErrTitleInvalid        = New(40008, "标题不合法", http.StatusBadRequest)
	ErrSlugInvalid         = New(40009, "URL 标识不合法", http.StatusBadRequest)
	ErrTagInvalid          = New(40010, "标签不合法", http.StatusBadRequest)
	ErrContentInvalid      = New(40011, "正文内容不合法", http.StatusBadRequest)
	ErrArticleSlugTaken    = New(40012, "该 URL 标识已被占用", http.StatusConflict)

	ErrChannelInvalid = New(40013, "栏目不存在", http.StatusBadRequest)

	// 开源工具推荐
	ErrToolNotFound     = New(40014, "工具不存在", http.StatusNotFound)
	ErrToolNotPublished = New(40015, "工具不存在或已下线", http.StatusNotFound)
	ErrToolSlugTaken    = New(40016, "该 URL 标识已被占用", http.StatusConflict)
	ErrToolStateInvalid = New(40017, "工具当前状态不支持该操作", http.StatusConflict)
	ErrToolDeployMode   = New(40018, "部署方式不合法", http.StatusBadRequest)
	ErrToolURLInvalid   = New(40019, "链接地址不合法", http.StatusBadRequest)

	// 运维题库
	ErrQuizNotFound      = New(40020, "题目不存在", http.StatusNotFound)
	ErrQuizNotPublished  = New(40021, "题目不存在或已下线", http.StatusNotFound)
	ErrQuizStateInvalid  = New(40022, "题目当前状态不支持该操作", http.StatusConflict)
	ErrQuizOptionInvalid = New(40023, "选项配置不合法", http.StatusBadRequest)
	ErrQuizAnswerInvalid = New(40024, "作答内容不合法", http.StatusBadRequest)
	ErrQuizPoolEmpty     = New(40025, "题库中暂无符合条件的题目", http.StatusNotFound)

	// 资源下载
	ErrResourceNotFound     = New(40026, "资源不存在", http.StatusNotFound)
	ErrResourceNotPublished = New(40027, "资源不存在或已下线", http.StatusNotFound)
	ErrResourceSlugTaken    = New(40028, "该 URL 标识已被占用", http.StatusConflict)
	ErrResourceStateInvalid = New(40029, "资源当前状态不支持该操作", http.StatusConflict)
	ErrResourceFileInvalid  = New(40030, "资源文件不合法，请重新上传", http.StatusBadRequest)
	ErrResourceLoginNeeded  = New(40031, "该资源需登录后下载", http.StatusUnauthorized)

	// 系统教程集（工具下的章节化教程）
	ErrToolBookNotFound     = New(40032, "教程集不存在", http.StatusNotFound)
	ErrToolBookStateInvalid = New(40033, "教程集当前状态不支持该操作", http.StatusConflict)
	ErrToolChapterInvalid   = New(40034, "章节结构不合法", http.StatusBadRequest)
)

// ============================ 问答广场 45xxx ============================
//
// 借 4xxxx 的内容段：问答与知识库同属内容侧，错误码前缀能让人一眼看出
// 「这不是钱的问题」。45xxx 起是为了不与题库/资源的 4002x 混在一起。

var (
	ErrQaNotFound           = New(45001, "问题不存在", http.StatusNotFound)
	ErrQaClosed             = New(45002, "该问题已关闭，不能再回答", http.StatusConflict)
	ErrQaSelfAnswer         = New(45003, "不能回答自己的提问", http.StatusBadRequest)
	ErrQaAlreadyAnswered    = New(45004, "你已经回答过这个问题，可以直接编辑原回答", http.StatusConflict)
	ErrQaNotQuestionOwner   = New(45005, "只有提问人能做这个操作", http.StatusForbidden)
	ErrQaAlreadySettled     = New(45006, "该问题已处理过，请刷新后再试", http.StatusConflict)
	ErrQaAnswerNotFound     = New(45007, "回答不存在或已删除", http.StatusNotFound)
	ErrQaAnswerNotPublished = New(45008, "该回答正在审核中，暂不能采纳", http.StatusConflict)
	ErrQaTitleTooShort      = New(45009, "标题至少 6 个字，把故障现象写清楚一些", http.StatusBadRequest)
	ErrQaContentTooShort    = New(45010, "内容太短了，至少 10 个字", http.StatusBadRequest)
	ErrQaAnswerRejected     = New(45011, "该回答未通过审核", http.StatusForbidden)
)

// ============================ 问题与订单 5xxxx ============================

var (
	// 悬赏问题
	ErrQuestionNotFound = New(50001, "问题不存在", http.StatusNotFound)
	ErrQuestionClosed   = New(50002, "该问题已结束，不能再操作", http.StatusConflict)
	ErrQuestionExpired  = New(50003, "悬赏已过期，请重新发布", http.StatusConflict)
	ErrBudgetInvalid    = New(50004, "悬赏金额不合法", http.StatusBadRequest)
	ErrQuestionTooShort = New(50005, "问题描述不能少于 10 个字", http.StatusBadRequest)
	ErrQuestionSkillReq = New(50006, "请至少选择一个技术栈", http.StatusBadRequest)
	ErrQuestionStateBad = New(50007, "问题当前状态不支持该操作", http.StatusConflict)
	ErrQuestionOwn      = New(50008, "不能接自己发布的问题", http.StatusBadRequest)

	// 订单
	ErrOrderNotFound     = New(50009, "订单不存在", http.StatusNotFound)
	ErrOrderStateInvalid = New(50010, "订单当前状态不支持该操作", http.StatusConflict)
	ErrOrderTaken        = New(50011, "手慢了，该订单已被其他专家接单", http.StatusConflict)
	ErrOrderNotOwner     = New(50012, "这不是你的订单", http.StatusForbidden)
	ErrOrderNotExpert    = New(50013, "该订单不属于你", http.StatusForbidden)
	ErrOrderCancelDenied = New(50014, "订单已进入服务流程，不能直接取消；如有异议请申请退款", http.StatusConflict)
	ErrOrderPayExpired   = New(50015, "支付已超时，订单已自动取消，请重新下单", http.StatusConflict)
	ErrServiceModeDenied = New(50016, "该专家不支持所选服务方式", http.StatusBadRequest)
	ErrBookTimeInvalid   = New(50017, "预约时间不合法", http.StatusBadRequest)
	ErrOrderSelfDealing  = New(50018, "不能下单给自己", http.StatusBadRequest)
	ErrExpertNotGrabable = New(50019, "只有已通过审核且开放接单的专家可以接单", http.StatusForbidden)

	// 评价
	ErrReviewNotAllowed = New(50020, "订单完成后才能评价", http.StatusConflict)
	ErrReviewDuplicated = New(50021, "该订单已评价过，评价提交后不可修改", http.StatusConflict)
	ErrReviewScore      = New(50022, "评分需在 1-5 之间", http.StatusBadRequest)
	ErrReviewContent    = New(50023, "评价内容不能少于 10 个字", http.StatusBadRequest)

	// 争议
	ErrDisputeReasonReq = New(50024, "请填写争议原因", http.StatusBadRequest)
	ErrDisputeExists    = New(50025, "该订单已在争议处理中", http.StatusConflict)
	ErrDisputeClosed    = New(50026, "该争议已处理完成", http.StatusConflict)

	// 服务会话
	ErrSessionNotFound = New(50027, "服务会话不存在", http.StatusNotFound)
	ErrSessionStateBad = New(50028, "会话当前状态不支持该操作", http.StatusConflict)
)

// ============================ 支付与结算 6xxxx ============================

var (
	ErrPayChannelInvalid = New(60001, "支付方式不合法", http.StatusBadRequest)
	// 真实通道还没接通时必须明确说"没开通"，不能假装支付成功——
	// 那会让订单进入服务中而钱从没到账，对账时无法区分是漏单还是漏洞。
	ErrPayChannelUnavailable = New(60002, "该支付方式暂未开通，请选择其他方式", http.StatusNotImplemented)
	ErrPayFailed             = New(60003, "支付失败，请重试或更换支付方式", http.StatusBadRequest)
	ErrPayAlreadyPaid        = New(60004, "订单已支付，请勿重复支付", http.StatusConflict)
	ErrPayCallbackInvalid    = New(60005, "支付回调校验失败", http.StatusForbidden)
	ErrPayNotPaid            = New(60006, "订单尚未支付", http.StatusConflict)

	ErrSettleNotAllowed = New(60007, "订单未完成，不能结算", http.StatusConflict)
	ErrSettleDuplicated = New(60008, "该订单已结算", http.StatusConflict)
	ErrSettleAmountBad  = New(60009, "结算金额异常，请联系技术支持", http.StatusInternalServerError)

	ErrRefundNotAllowed = New(60010, "当前订单状态不支持退款", http.StatusConflict)
	ErrRefundAmountBad  = New(60011, "退款金额不合法", http.StatusBadRequest)
)

// ============================ 文件与存储 7xxxx ============================

var (
	ErrFileTooLarge       = New(70001, "文件超过大小限制", http.StatusRequestEntityTooLarge)
	ErrFileTypeNotAllowed = New(70002, "不支持的文件类型", http.StatusUnsupportedMediaType)
	ErrFileMissing        = New(70003, "请选择要上传的文件", http.StatusBadRequest)
	ErrUploadFailed       = New(70004, "文件上传失败，请重试", http.StatusInternalServerError)
	ErrFileSignInvalid    = New(70005, "文件访问凭证无效", http.StatusForbidden)
	ErrFileSignExpired    = New(70006, "文件访问凭证已过期", http.StatusForbidden)
	ErrSceneInvalid       = New(70007, "上传场景不合法", http.StatusBadRequest)
)

// ============================ 内部依赖 9xxxx ============================

var (
	ErrDatabase   = New(90001, "数据服务异常，请稍后重试", http.StatusInternalServerError)
	ErrCache      = New(90002, "缓存服务异常，请稍后重试", http.StatusInternalServerError)
	ErrThirdParty = New(90003, "第三方服务异常，请稍后重试", http.StatusBadGateway)
	// ErrMailSendFailed 发信失败。文案不带 SMTP 细节：
	// 里面可能含收件地址与中继账号，回给前端等于泄露。
	ErrMailSendFailed = New(90004, "邮件发送失败，请稍后重试或联系管理员", http.StatusBadGateway)
)

// From 把任意 error 归一为 *Error；未知错误一律按内部错误处理，避免细节外泄。
func From(err error) *Error {
	if err == nil {
		return OK
	}
	var e *Error
	if errors.As(err, &e) {
		return e
	}
	return ErrInternal.WithCause(err)
}
