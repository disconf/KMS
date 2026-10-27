package middleware

import (
	"context"
	"errors"
	"strings"

	"github.com/gin-gonic/gin"

	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/authctx"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/jwtx"
	"github.com/opsexpert/platform/internal/pkg/response"
	"github.com/opsexpert/platform/internal/pkg/tokenstore"
)

const bearerPrefix = "Bearer "

// AccountLookup 读取账号当前状态与专家审核状态。由 repository.UserRepo 实现。
//
// 单独抽成接口而不是直接依赖仓储，是为了让中间件只关心「这个账号现在是什么身份」，
// 以后要换成带缓存的实现，换的是注入的对象，不是这里的判断逻辑。
type AccountLookup interface {
	// FindAccount 返回账号本体、专家审核状态（无档案时为空串）。
	FindAccount(ctx context.Context, id int64) (*model.User, string, error)
}

// Auth 强制登录中间件：解析访问令牌、校验类型与吊销状态，
// 并以数据库为准复核账号状态与角色，最后把身份写入上下文。
//
// 为什么要查库：JWT 是自包含的，签发那一刻的 status/role 会被一直带在令牌里。
// 只信令牌就等于「封禁要等令牌自然过期才生效」「改了角色要用户重新登录」——
// 前者是安全问题（封了人还在下单），后者是运营每天都会碰到的坑。
// 代价是每个受保护请求多一次主键查询；对这套系统的量级来说，
// 换来「封禁即时生效」远比省这一次查询划算。
func Auth(mgr *jwtx.Manager, store *tokenstore.Store, accounts AccountLookup) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			response.Fail(c, errcode.ErrUnauthorized)
			return
		}

		claims, err := mgr.Parse(token)
		if err != nil {
			response.Fail(c, err)
			return
		}
		// 刷新令牌不能当访问令牌用，这是最常见的一类越权尝试。
		if claims.Type != jwtx.TypeAccess {
			response.Fail(c, errcode.ErrTokenInvalid.WithMsg("令牌类型错误"))
			return
		}
		if store.IsRevoked(c.Request.Context(), claims.ID) {
			response.Fail(c, errcode.ErrTokenInvalid.WithMsg("登录状态已失效，请重新登录"))
			return
		}

		account, expertStatus, err := accounts.FindAccount(c.Request.Context(), claims.UserID)
		if err != nil {
			// 令牌有效但账号已不存在（注销/物理删除）：当作登录失效，
			// 让前端走重新登录而不是把 404 当业务错误展示。
			if errors.Is(err, errcode.ErrAccountNotFound) {
				response.Fail(c, errcode.ErrTokenInvalid.WithMsg("登录状态已失效，请重新登录"))
				return
			}
			response.Fail(c, err)
			return
		}
		if !account.IsActive() {
			response.Fail(c, errcode.ErrAccountBanned)
			return
		}

		authctx.Set(c, authctx.Identity{
			UserID: account.ID,
			// 角色与专家状态都以库为准，不信令牌里的快照。
			Role:         account.Role,
			ExpertStatus: expertStatus,
			SpeakStatus:  account.SpeakStatus,
			JTI:          claims.ID,
		})
		c.Next()
	}
}

// OptionalAuth 可选登录：带令牌则解析身份，不带或无效也放行。
// 用于知识库这类"游客可读、登录后有额外能力"的接口。
//
// 这里刻意**不查库**：它的用途是给游客页面认一下"我点过赞没有"，
// 而所有真正写数据的接口都走 Auth，封禁在那一层已经拦住。
// 每请求多一次查询去拦一个「被禁用户读公开文章」的场景，不值得。
func OptionalAuth(mgr *jwtx.Manager, store *tokenstore.Store) gin.HandlerFunc {
	return func(c *gin.Context) {
		token := extractToken(c)
		if token == "" {
			c.Next()
			return
		}
		claims, err := mgr.Parse(token)
		if err != nil || claims.Type != jwtx.TypeAccess {
			c.Next()
			return
		}
		if store.IsRevoked(c.Request.Context(), claims.ID) {
			c.Next()
			return
		}
		authctx.Set(c, authctx.Identity{
			UserID:       claims.UserID,
			Role:         claims.Role,
			ExpertStatus: claims.ExpertStatus,
			JTI:          claims.ID,
		})
		c.Next()
	}
}

// RequireRole 角色守卫，必须置于 Auth 之后。
func RequireRole(roles ...string) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[r] = struct{}{}
	}
	return func(c *gin.Context) {
		role := authctx.Role(c)
		if role == "" {
			response.Fail(c, errcode.ErrUnauthorized)
			return
		}
		if _, ok := allowed[role]; !ok {
			response.Fail(c, errcode.ErrForbidden.WithMsg("当前角色无权访问该接口"))
			return
		}
		c.Next()
	}
}

// RequireApprovedExpert 专家守卫：登录 + 角色为 expert + 审核已通过。
//
// 三类失败给出不同错误码，前端才能把用户引导到"去申请 / 审核中 / 已停牌"等不同页面，
// 而不是统一弹一个 403 空白页。
func RequireApprovedExpert() gin.HandlerFunc {
	return func(c *gin.Context) {
		identity, ok := authctx.Get(c)
		if !ok {
			response.Fail(c, errcode.ErrUnauthorized)
			return
		}
		if identity.Role != model.RoleExpert && identity.Role != model.RoleAdmin {
			response.Fail(c, errcode.ErrExpertNotApplied)
			return
		}

		switch identity.ExpertStatus {
		case model.ExpertStatusApproved:
			c.Next()
		case "":
			// 空值只对管理员放行（管理员本来就没有专家档案）。
			// 专家角色却查不到档案，说明是「角色改了但没走审核」，
			// 从前这里一律放行，等于给了一条绕过入驻审核的捷径。
			if identity.Role == model.RoleAdmin {
				c.Next()
				return
			}
			response.Fail(c, errcode.ErrExpertNotApplied)
		case model.ExpertStatusPending:
			response.Fail(c, errcode.ErrExpertUnderReview)
		case model.ExpertStatusRejected:
			response.Fail(c, errcode.ErrExpertRejected)
		case model.ExpertStatusSuspended:
			response.Fail(c, errcode.ErrExpertSuspended)
		default:
			response.Fail(c, errcode.ErrExpertNotApplied)
		}
	}
}

// extractToken 按 "Authorization: Bearer <token>" 提取令牌，兼容裸令牌写法。
func extractToken(c *gin.Context) string {
	raw := strings.TrimSpace(c.GetHeader("Authorization"))
	if raw == "" {
		return ""
	}
	if len(raw) > len(bearerPrefix) && strings.EqualFold(raw[:len(bearerPrefix)], bearerPrefix) {
		return strings.TrimSpace(raw[len(bearerPrefix):])
	}
	return raw
}
