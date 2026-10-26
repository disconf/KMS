package service

import (
	"context"
	"strings"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/repository"
)

// defaultArticleChannel 是新建文章未指定栏目时的归属。
var defaultArticleChannel = model.ChannelLinuxTutorial

// ChannelService 负责栏目导航。
//
// 栏目本身是代码里的注册表（见 model/channel.go），这个 service 只做一件事：
// 把注册表和各栏目的实时内容数拼起来，让前端一次请求就能渲染出完整导航。
type ChannelService struct {
	articles  *repository.ArticleRepo
	tools     *repository.ToolRepo
	quizzes   *repository.QuizRepo
	resources *repository.ResourceRepo
}

// NewChannelService 创建 ChannelService。
func NewChannelService(
	articles *repository.ArticleRepo,
	tools *repository.ToolRepo,
	quizzes *repository.QuizRepo,
	resources *repository.ResourceRepo,
) *ChannelService {
	return &ChannelService{articles: articles, tools: tools, quizzes: quizzes, resources: resources}
}

// List 返回全部栏目及其已发布内容数。
//
// 计数失败不阻断返回：导航栏少了角标用户照样能点，
// 但如果因为一次统计失败整个导航都打不开，那就把一个装饰性数据变成了致命依赖。
func (s *ChannelService) List(ctx context.Context) []dto.ChannelItem {
	articleCounts, err := s.articles.CountPublishedByChannel(ctx)
	if err != nil {
		logger.Warn("统计栏目文章数失败，角标将显示为 0", "err", err.Error())
		articleCounts = map[string]int64{}
	}

	counts := map[string]int64{}
	for slug, n := range articleCounts {
		counts[slug] = n
	}
	if n, err := s.tools.CountPublished(ctx); err != nil {
		logger.Warn("统计工具栏目数量失败", "err", err.Error())
	} else {
		counts[model.ChannelOpenSource] = n
	}
	if n, err := s.quizzes.CountPublished(ctx); err != nil {
		logger.Warn("统计题库栏目数量失败", "err", err.Error())
	} else {
		counts[model.ChannelOpsQuiz] = n
	}
	if n, err := s.resources.CountPublished(ctx); err != nil {
		logger.Warn("统计资源栏目数量失败", "err", err.Error())
	} else {
		counts[model.ChannelResources] = n
	}

	metas := model.AllChannels()
	items := make([]dto.ChannelItem, 0, len(metas))
	for _, m := range metas {
		items = append(items, dto.ChannelItem{
			Slug:  m.Slug,
			Name:  m.Name,
			Icon:  m.Icon,
			Desc:  m.Desc,
			Kind:  m.Kind,
			Sort:  m.Sort,
			Path:  m.Path,
			Count: counts[m.Slug],
		})
	}
	return items
}

// ============================ 栏目取值校验 ============================

// resolveArticleChannel 校验并规范化文章栏目取值，空串归入默认栏目。
// 用于写入路径（新建 / 更新），会把用户传错的值挡下来。
func resolveArticleChannel(raw string) (string, error) {
	slug := strings.TrimSpace(raw)
	if slug == "" {
		return defaultArticleChannel, nil
	}
	if !model.IsArticleChannel(slug) {
		return "", errcode.ErrChannelInvalid.WithMsg("栏目不存在：" + slug)
	}
	return slug, nil
}

// resolveArticleChannelFilter 校验查询用的栏目条件，空串表示不限栏目。
// 与写入路径分开是因为两者的语义不同：查询时空值合法（表示全部），写入时空值要兜底。
func resolveArticleChannelFilter(raw string) (string, error) {
	slug := strings.TrimSpace(raw)
	if slug == "" {
		return "", nil
	}
	if !model.IsArticleChannel(slug) {
		return "", errcode.ErrChannelInvalid.WithMsg("栏目不存在：" + slug)
	}
	return slug, nil
}
