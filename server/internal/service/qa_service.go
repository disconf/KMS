package service

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/repository"
)

// 问答广场规则常量。
const (
	// qaAskCooldown 提问冷却：同一账号 10 分钟一条。
	// 不做更细的频率控制（滑动窗口那套）是因为这类内容不是钱，
	// 挡住"一分钟发十条"已经够了，再严会误伤真的连着问两个问题的人。
	qaAskCooldown = 10 * time.Minute

	// qaNewUserFreeAnswers 新账号前 N 条回答必须先审。
	// 阈值取 3：真实用户前三条几乎都是认真答的，而批量灌水机器人不会只发三条。
	qaNewUserFreeAnswers = 3

	// 积分值：只做展示，规则越简单越好解释。
	pointsAcceptAnswer = 10
	pointsAnswerLiked  = 1
)

// QaViewer 当前访问者。
//
// 由 handler 从登录态组装，service 不碰 gin.Context。
// 未登录时 UserID = 0，其余字段为空。
type QaViewer struct {
	UserID       int64
	Role         string
	ExpertStatus string
	// SpeakStatus 发言权限状态。只读账号（新注册默认）拿不到发布能力。
	SpeakStatus string
}

// LoggedIn 是否已登录。
func (v QaViewer) LoggedIn() bool { return v.UserID > 0 }

// IsAdmin 是否管理员。
func (v QaViewer) IsAdmin() bool { return v.Role == model.RoleAdmin }

// IsApprovedExpert 是否已过审核的专家。
//
// 两个条件都要：role=expert 只说明"被设成了专家"，
// 而专家状态为空表示还没提交过申请（历史数据里存在这种脏值）。
func (v QaViewer) IsApprovedExpert() bool {
	return v.Role == model.RoleExpert && v.ExpertStatus == model.ExpertStatusApproved
}

// CanPublishDirectly 是否免审直发（管理员与已过审专家）。
//
// 专家能直发是产品决定：他们回答的质量由入驻审核与订单评价背书，
// 让他们和刚注册的账号一起排队等审，等于劝退最该回答的那批人。
func (v QaViewer) CanPublishDirectly() bool {
	return v.IsAdmin() || v.IsApprovedExpert()
}

// CanSpeak 是否有发言资格。
//
// 与 CanPublishDirectly 的区别：后者决定"发出来要不要审"，
// 这里决定"能不能发"。新注册账号先只读，实名审核通过后才进入
// "可以发、但要审"的队列——两道闸是叠加的，不是二选一。
func (v QaViewer) CanSpeak() bool {
	if v.IsAdmin() || v.Role == model.RoleExpert {
		return true
	}
	return v.SpeakStatus == model.SpeakApproved
}

// QaService 问答广场业务。
type QaService struct {
	qa       *repository.QaRepo
	points   *repository.PointRepo
	users    *repository.UserRepo
	audits   *repository.AuditRepo
	articles *ArticleService
}

// NewQaService 创建 QaService。
//
// articles 只用于「精选转教程」：转出来的是一篇草稿，正文组织仍走文章系统，
// 这样编辑器、slug、分类、审核全套都不必再复制一份。
func NewQaService(
	qa *repository.QaRepo,
	points *repository.PointRepo,
	users *repository.UserRepo,
	audits *repository.AuditRepo,
	articles *ArticleService,
) *QaService {
	return &QaService{qa: qa, points: points, users: users, audits: audits, articles: articles}
}

/* ============================ 前台：读 ============================ */

// List 问题列表。
func (s *QaService) List(ctx context.Context, filter repository.QaFilter, q dto.PageQuery, viewer QaViewer) (*dto.QaListPage, error) {
	q.Normalize()
	rows, total, err := s.qa.List(ctx, filter, q.Offset(), q.Limit())
	if err != nil {
		return nil, err
	}
	list := make([]dto.QaQuestionItem, 0, len(rows))
	for i := range rows {
		list = append(list, toQaItem(&rows[i]))
	}
	page := &dto.QaListPage{List: list, Total: total, Page: q.Page, PageSize: q.Limit()}

	// 顶部计数只在第一页带：翻页时再算一遍纯属浪费，
	// 而前端本来也只需要在列表顶部显示一次。
	if q.Page == 1 {
		if st, err := s.qa.Stats(ctx); err == nil {
			page.Stats = &dto.QaStatsDTO{Open: st.Open, Resolved: st.Resolved, Answers: st.Answers}
		}
	}
	return page, nil
}

// Detail 问题详情（含回答）。
func (s *QaService) Detail(ctx context.Context, id int64, viewer QaViewer) (*dto.QaQuestionDetail, error) {
	row, err := s.qa.Get(ctx, id)
	if err != nil {
		return nil, err
	}

	// 浏览数自增放在读到之后、返回之前：计数器失败不该让人看不到内容。
	if err := s.qa.IncrView(ctx, id); err != nil {
		logger.Warn("问答浏览数自增失败", "id", id, "err", err.Error())
	}

	// 管理员能看到全部待审回答；作者自己的待审回答也要能看到（否则投完就消失）。
	includePending := viewer.IsAdmin()
	answers, err := s.qa.Answers(ctx, id, includePending, viewer.UserID)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(answers))
	for i := range answers {
		ids = append(ids, answers[i].ID)
	}
	liked, err := s.qa.LikedSet(ctx, viewer.UserID, ids)
	if err != nil {
		return nil, err
	}

	detail := &dto.QaQuestionDetail{
		QaQuestionItem: toQaItem(row),
		Description:    row.Description,
		Attachments:    []string(row.Attachments),
		AcceptedID:     row.AcceptedAnswerID,
	}
	if detail.Attachments == nil {
		detail.Attachments = []string{}
	}

	items := make([]dto.QaAnswerItem, 0, len(answers))
	for i := range answers {
		a := &answers[i]
		// 待审回答只对作者本人可见；别人（包括管理员以外的访客）看不到，
		// 但管理员看得到全部——这里对"自己那条"已经不靠 SQL 过滤，靠下面的判断。
		if a.Status != model.QaAnswerPublished && !viewer.IsAdmin() && a.UserID != viewer.UserID {
			continue
		}
		item := dto.QaAnswerItem{
			ID:                 a.ID,
			QuestionID:         a.QuestionID,
			UserID:             a.UserID,
			Nickname:           a.Nickname,
			Avatar:             a.Avatar,
			ContributionPoints: a.ContributionPoints,
			Badge:              BadgeOf(a.ContributionPoints),
			AuthorRole:         a.AuthorRole,
			IsExpert:           a.AuthorRole == model.RoleExpert,
			Content:            a.Content,
			Score:              a.Score,
			IsAccepted:         a.IsAccepted,
			Liked:              liked[a.ID],
			Status:             a.Status,
			CreatedAt:          a.CreatedAt,
			UpdatedAt:          a.UpdatedAt,
		}
		if a.Status == model.QaAnswerRejected && a.UserID == viewer.UserID {
			item.ReviewNote = a.ReviewNote
		}
		if a.UserID == viewer.UserID {
			detail.MyAnswerID = a.ID
		}
		items = append(items, item)
	}
	detail.Answers = items

	// 可操作项由服务端算：前端只用来决定按钮显隐，校验仍在写接口里做。
	isOwner := viewer.LoggedIn() && viewer.UserID == row.UserID
	detail.CanAccept = isOwner && row.Status == model.QaStatusOpen && row.AnswerCount > 0
	detail.CanClose = isOwner && row.Status == model.QaStatusOpen
	return detail, nil
}

/* ============================ 前台：写 ============================ */

// Ask 发布问题。
func (s *QaService) Ask(ctx context.Context, viewer QaViewer, req dto.QaAskReq) (*dto.QaQuestionItem, error) {
	if !viewer.LoggedIn() {
		return nil, errcode.ErrUnauthorized
	}
	if !viewer.CanSpeak() {
		return nil, errcode.ErrSpeakNotAllowed
	}

	title := strings.TrimSpace(req.Title)
	if n := runeLen(title); n < 6 {
		return nil, errcode.ErrQaTitleTooShort
	} else if n > 200 {
		return nil, errcode.ErrParamInvalid.WithMsg("标题不能超过 200 个字")
	}

	desc := strings.TrimSpace(req.Description)
	if n := runeLen(desc); n < 10 {
		return nil, errcode.ErrQaContentTooShort
	} else if n > 20000 {
		return nil, errcode.ErrParamInvalid.WithMsg("正文不能超过 20000 个字")
	}

	attachments, err := normalizeQaAttachments(req.Attachments)
	if err != nil {
		return nil, err
	}
	skillIDs, err := normalizeQaSkills(req.SkillIDs)
	if err != nil {
		return nil, err
	}

	// 限流：最后一条提问时间在冷却窗口内就拒绝。
	last, err := s.qa.LastAskedAt(ctx, viewer.UserID)
	if err != nil {
		return nil, err
	}
	if last != nil && time.Since(*last) < qaAskCooldown {
		wait := qaAskCooldown - time.Since(*last)
		return nil, errcode.ErrParamInvalid.WithMsg(
			fmt.Sprintf("提问太频繁了，请 %d 分钟后再发", int(wait.Minutes())+1))
	}

	q := &model.QaQuestion{
		UserID:      viewer.UserID,
		Title:       title,
		Description: desc,
		Attachments: model.StringList(attachments),
		SkillIDs:    model.StringList(int64sToStrings(skillIDs)),
		Status:      model.QaStatusOpen,
	}
	if err := s.qa.Create(ctx, q); err != nil {
		return nil, err
	}

	row, err := s.qa.Get(ctx, q.ID)
	if err != nil {
		// 建成功却读不回来只是展示问题，不该让用户以为没发出去；
		// 用内存里的数据兜底返回。
		item := toQaItem(&repository.QaQuestionRow{QaQuestion: *q})
		item.Nickname = ""
		return &item, nil
	}
	item := toQaItem(row)
	return &item, nil
}

// Answer 发布回答。
func (s *QaService) Answer(ctx context.Context, viewer QaViewer, questionID int64, req dto.QaAnswerReq) (*dto.QaAnswerItem, error) {
	if !viewer.LoggedIn() {
		return nil, errcode.ErrUnauthorized
	}
	if !viewer.CanSpeak() {
		return nil, errcode.ErrSpeakNotAllowed
	}
	q, err := s.qa.Get(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q.Status != model.QaStatusOpen {
		return nil, errcode.ErrQaClosed
	}
	if q.UserID == viewer.UserID {
		return nil, errcode.ErrQaSelfAnswer
	}

	content := strings.TrimSpace(req.Content)
	if n := runeLen(content); n < 10 {
		return nil, errcode.ErrQaContentTooShort
	} else if n > 20000 {
		return nil, errcode.ErrParamInvalid.WithMsg("回答不能超过 20000 个字")
	}

	status := model.QaAnswerPublished
	if !viewer.CanPublishDirectly() {
		n, err := s.qa.CountPublishedByUser(ctx, viewer.UserID)
		if err != nil {
			return nil, err
		}
		if n < qaNewUserFreeAnswers {
			status = model.QaAnswerPending
		}
	}

	a := &model.QaAnswer{
		QuestionID: questionID,
		UserID:     viewer.UserID,
		Content:    content,
		Status:     status,
	}
	if err := s.qa.CreateAnswer(ctx, a); err != nil {
		return nil, err
	}

	// 只有真正上线的回答才计入数与顶帖：待审的不该让帖子看起来"已有人答"，
	// 否则审核驳回后列表上的回答数会永远比实际多。
	if status == model.QaAnswerPublished {
		if err := s.qa.IncrAnswerCount(ctx, questionID, 1); err != nil {
			logger.Warn("回答计数自增失败", "question_id", questionID, "err", err.Error())
		}
		if err := s.qa.TouchLastAnswer(ctx, questionID); err != nil {
			logger.Warn("更新最近回答时间失败", "question_id", questionID, "err", err.Error())
		}
	}

	user, _ := s.users.FindByID(ctx, viewer.UserID)
	item := dto.QaAnswerItem{
		ID: a.ID, QuestionID: questionID, UserID: viewer.UserID,
		Content: content, Status: status,
		AuthorRole: viewer.Role, IsExpert: viewer.IsApprovedExpert(),
		Badge:     BadgeOf(0),
		CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
	}
	if user != nil {
		item.Nickname = user.Nickname
		item.Avatar = user.Avatar
		item.ContributionPoints = user.ContributionPoints
		item.Badge = BadgeOf(user.ContributionPoints)
	}
	return &item, nil
}

// UpdateAnswer 编辑自己的回答。
func (s *QaService) UpdateAnswer(ctx context.Context, viewer QaViewer, answerID int64, req dto.QaAnswerReq) error {
	if !viewer.LoggedIn() {
		return errcode.ErrUnauthorized
	}
	content := strings.TrimSpace(req.Content)
	if n := runeLen(content); n < 10 {
		return errcode.ErrQaContentTooShort
	} else if n > 20000 {
		return errcode.ErrParamInvalid.WithMsg("回答不能超过 20000 个字")
	}
	ok, err := s.qa.UpdateAnswerContent(ctx, answerID, viewer.UserID, content)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrQaNotQuestionOwner.WithMsg("只能编辑自己的回答")
	}
	return nil
}

// Accept 采纳回答。
//
// 规则：只有提问人、只能采纳一次、只能采纳已发布的回答。
// 三条都要在服务端判，前端按钮只是提示。
func (s *QaService) Accept(ctx context.Context, viewer QaViewer, questionID, answerID int64) error {
	if !viewer.LoggedIn() {
		return errcode.ErrUnauthorized
	}
	q, err := s.qa.Get(ctx, questionID)
	if err != nil {
		return err
	}
	if q.UserID != viewer.UserID {
		return errcode.ErrQaNotQuestionOwner
	}
	if q.Status != model.QaStatusOpen {
		return errcode.ErrQaAlreadySettled
	}

	a, err := s.qa.GetAnswer(ctx, answerID)
	if err != nil {
		return err
	}
	if a.QuestionID != questionID {
		// 拿别的问题的回答 ID 来采纳，是典型的越权尝试。
		return errcode.ErrQaAnswerNotFound
	}
	if a.Status != model.QaAnswerPublished {
		return errcode.ErrQaAnswerNotPublished
	}

	ok, err := s.qa.AcceptAnswer(ctx, questionID, answerID, viewer.UserID)
	if err != nil {
		return err
	}
	if !ok {
		// CAS 失败：并发下别人先采纳了，或问题已经被关闭。
		return errcode.ErrQaAlreadySettled
	}
	if err := s.qa.SetAnswerAccepted(ctx, answerID, true); err != nil {
		logger.Warn("更新采纳标记失败", "answer_id", answerID, "err", err.Error())
	}

	// 给回答者记积分。失败不影响采纳结果：采纳是事实，积分是荣誉，
	// 为了积分写失败去回滚采纳，等于把用户的确认动作当儿戏。
	if _, err := s.points.Award(ctx, &model.PointLog{
		UserID: a.UserID, Action: model.PointActionAcceptAnswer,
		Points: pointsAcceptAnswer, TargetType: "answer", TargetID: answerID,
		UniqKey: fmt.Sprintf("qa.accept:%d", answerID),
	}); err != nil {
		logger.Warn("记录采纳积分失败", "answer_id", answerID, "err", err.Error())
	}
	return nil
}

// Close 关闭问题（未解决也不再接收回答）。
func (s *QaService) Close(ctx context.Context, viewer QaViewer, questionID int64) error {
	if !viewer.LoggedIn() {
		return errcode.ErrUnauthorized
	}
	ok, err := s.qa.CloseQuestion(ctx, questionID, viewer.UserID)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrQaAlreadySettled
	}
	return nil
}

// Like 点赞 / 取消点赞（幂等）。
//
// 返回最新的点赞状态与总数，前端据此更新按钮，不必重新拉详情。
func (s *QaService) Like(ctx context.Context, viewer QaViewer, answerID int64) (bool, int64, error) {
	if !viewer.LoggedIn() {
		return false, 0, errcode.ErrUnauthorized
	}
	a, err := s.qa.GetAnswer(ctx, answerID)
	if err != nil {
		return false, 0, err
	}

	created, err := s.qa.LikeAnswer(ctx, answerID, viewer.UserID)
	if err != nil {
		return false, 0, err
	}
	// 自己给自己点赞不加分，也不该出现在「贡献」里；
	// 但点赞本身允许（有人就是习惯性点一下自己的答复），只是不记功。
	if created && a.UserID != viewer.UserID {
		if _, err := s.points.Award(ctx, &model.PointLog{
			UserID: a.UserID, Action: model.PointActionAnswerLiked,
			Points: pointsAnswerLiked, TargetType: "answer", TargetID: answerID,
			UniqKey: fmt.Sprintf("qa.like:%d:%d", answerID, viewer.UserID),
		}); err != nil {
			logger.Warn("记录点赞积分失败", "answer_id", answerID, "err", err.Error())
		}
	}

	fresh, err := s.qa.GetAnswer(ctx, answerID)
	if err != nil {
		return true, 0, nil
	}
	return true, fresh.Score, nil
}

// Unlike 取消点赞。
func (s *QaService) Unlike(ctx context.Context, viewer QaViewer, answerID int64) (int64, error) {
	if !viewer.LoggedIn() {
		return 0, errcode.ErrUnauthorized
	}
	if _, err := s.qa.UnlikeAnswer(ctx, answerID, viewer.UserID); err != nil {
		return 0, err
	}
	fresh, err := s.qa.GetAnswer(ctx, answerID)
	if err != nil {
		return 0, nil
	}
	return fresh.Score, nil
}

// DeleteQuestion 删除自己的问题（管理员可删任意）。
func (s *QaService) DeleteQuestion(ctx context.Context, viewer QaViewer, questionID int64) error {
	if !viewer.LoggedIn() {
		return errcode.ErrUnauthorized
	}
	q, err := s.qa.Get(ctx, questionID)
	if err != nil {
		return err
	}
	if q.UserID != viewer.UserID && !viewer.IsAdmin() {
		return errcode.ErrQaNotQuestionOwner.WithMsg("只能删除自己的提问")
	}
	return s.qa.DeleteQuestion(ctx, questionID)
}

// DeleteAnswer 删除回答。
//
// 已采纳的回答不允许删除：它已经是这个问题的"答案"，删掉会让页面变成
// 「已解决但看不到解法」。要改就编辑内容。
func (s *QaService) DeleteAnswer(ctx context.Context, viewer QaViewer, answerID int64) error {
	if !viewer.LoggedIn() {
		return errcode.ErrUnauthorized
	}
	a, err := s.qa.GetAnswer(ctx, answerID)
	if err != nil {
		return err
	}
	if a.UserID != viewer.UserID && !viewer.IsAdmin() {
		return errcode.ErrQaNotQuestionOwner.WithMsg("只能删除自己的回答")
	}
	if a.IsAccepted {
		return errcode.ErrQaAlreadySettled.WithMsg("已采纳的回答不能删除，如需修改请编辑内容")
	}
	if err := s.qa.DeleteAnswer(ctx, answerID); err != nil {
		return err
	}
	if a.Status == model.QaAnswerPublished {
		if err := s.qa.IncrAnswerCount(ctx, a.QuestionID, -1); err != nil {
			logger.Warn("回答计数自减失败", "question_id", a.QuestionID, "err", err.Error())
		}
	}
	return nil
}

/* ============================ 后台 ============================ */

// PendingAnswers 待审回答列表（附带问题标题，审核时要看上下文）。
func (s *QaService) PendingAnswers(ctx context.Context, q dto.PageQuery) (*dto.QaAdminList, error) {
	q.Normalize()
	rows, total, err := s.qa.PendingAnswers(ctx, q.Offset(), q.Limit())
	if err != nil {
		return nil, err
	}
	list := make([]dto.QaAdminAnswerItem, 0, len(rows))
	for i := range rows {
		a := &rows[i]
		list = append(list, dto.QaAdminAnswerItem{
			QaAnswerItem: dto.QaAnswerItem{
				ID: a.ID, QuestionID: a.QuestionID, UserID: a.UserID,
				Nickname: a.Nickname, Avatar: a.Avatar,
				ContributionPoints: a.ContributionPoints, Badge: BadgeOf(a.ContributionPoints),
				AuthorRole: a.AuthorRole, IsExpert: a.AuthorRole == model.RoleExpert,
				Content: a.Content, Score: a.Score, Status: a.Status,
				CreatedAt: a.CreatedAt, UpdatedAt: a.UpdatedAt,
			},
			QuestionTitle: s.qa.TitleOf(ctx, a.QuestionID),
		})
	}
	return &dto.QaAdminList{List: list, Total: total, Page: q.Page, PageSize: q.Limit()}, nil
}

// ReviewAnswer 审核回答。
//
// 用 CAS（只有 pending 能被处理）：两个运营同时点通过，
// 第二次命中 0 行并报错，而不是把回答计数加两次。
func (s *QaService) ReviewAnswer(ctx context.Context, adminID int64, adminRole string, answerID int64, req dto.QaReviewReq, meta AuditMeta) error {
	action := strings.TrimSpace(req.Action)
	var status string
	switch action {
	case "approve":
		status = model.QaAnswerPublished
	case "reject":
		status = model.QaAnswerRejected
	default:
		return errcode.ErrParamInvalid.WithMsg("action 只能是 approve 或 reject")
	}

	note := strings.TrimSpace(req.Note)
	if status == model.QaAnswerRejected && note == "" {
		// 驳回不写原因，作者只会看到内容消失，然后重发一遍。
		return errcode.ErrParamInvalid.WithMsg("驳回必须填写原因")
	}

	a, err := s.qa.GetAnswer(ctx, answerID)
	if err != nil {
		return err
	}

	ok, err := s.qa.ReviewAnswer(ctx, answerID, status, note, adminID)
	if err != nil {
		return err
	}
	if !ok {
		return errcode.ErrQaAlreadySettled.WithMsg("该回答已被处理，请刷新")
	}

	if status == model.QaAnswerPublished {
		if err := s.qa.IncrAnswerCount(ctx, a.QuestionID, 1); err != nil {
			logger.Warn("回答计数自增失败", "question_id", a.QuestionID, "err", err.Error())
		}
		if err := s.qa.TouchLastAnswer(ctx, a.QuestionID); err != nil {
			logger.Warn("更新最近回答时间失败", "question_id", a.QuestionID, "err", err.Error())
		}
	}

	appendAudit(ctx, s.audits, adminID, adminRole, "qa.answer_review", "qa_answer",
		answerID, map[string]any{"action": action, "note": note, "status": status}, meta)
	return nil
}

// ToArticle 把「问题 + 采纳回答」转成教程草稿。
//
// 只产出 draft：问答原文是一问一答的碎片，直接发布不像教程，
// 必须有人重写导语、补上下文、整理成章节。
func (s *QaService) ToArticle(ctx context.Context, adminID int64, adminRole string, questionID int64, req dto.QaToArticleReq, meta AuditMeta) (*dto.QaToArticleResp, error) {
	q, err := s.qa.Get(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q.AcceptedAnswerID == 0 {
		return nil, errcode.ErrParamInvalid.WithMsg("该问题还没有采纳回答，先采纳再转教程")
	}
	acc, err := s.qa.GetAnswer(ctx, q.AcceptedAnswerID)
	if err != nil {
		return nil, err
	}

	title := strings.TrimSpace(req.Title)
	if title == "" {
		title = q.Title
	}

	var b strings.Builder
	b.WriteString("> 本文由问答广场的提问与采纳回答整理而来。\n")
	b.WriteString(fmt.Sprintf("> 原问题：#%d「%s」\n\n", q.ID, q.Title))
	b.WriteString("## 问题\n\n")
	b.WriteString(q.Description)
	b.WriteString("\n\n## 解答\n\n")
	b.WriteString(acc.Content)
	b.WriteString("\n")

	channel := strings.TrimSpace(req.Channel)
	if channel == "" {
		channel = model.ChannelLinuxTutorial
	}
	if channel == model.ChannelOpsNews {
		return nil, errcode.ErrParamInvalid.WithMsg("不能转到资讯栏目")
	}

	article, err := s.articles.CreateDraft(ctx, adminID, title, autoSummary(b.String()), b.String(), channel, req.CategoryID)
	if err != nil {
		return nil, err
	}

	appendAudit(ctx, s.audits, adminID, adminRole, "qa.to_article", "qa_question",
		questionID, map[string]any{"articleId": article.ID, "slug": article.Slug}, meta)

	return &dto.QaToArticleResp{
		ArticleID: article.ID,
		Slug:      article.Slug,
		EditPath:  fmt.Sprintf("#/admin/articles/%d", article.ID),
	}, nil
}

// MyPoints 我的贡献积分与流水。
func (s *QaService) MyPoints(ctx context.Context, viewer QaViewer) (*dto.MyPointsResp, error) {
	if !viewer.LoggedIn() {
		return nil, errcode.ErrUnauthorized
	}
	points, err := s.points.PointsOf(ctx, viewer.UserID)
	if err != nil {
		return nil, err
	}
	logs, err := s.points.RecentOf(ctx, viewer.UserID, 20)
	if err != nil {
		return nil, err
	}

	resp := &dto.MyPointsResp{Points: points, Badge: BadgeOf(points)}
	resp.NextBadge, resp.NextAt = nextBadgeOf(points)
	for _, l := range logs {
		resp.Recent = append(resp.Recent, dto.PointLogItem{
			Action: l.Action, ActionText: pointActionText(l.Action), Points: l.Points,
			TargetType: l.TargetType, TargetID: l.TargetID, CreatedAt: l.CreatedAt,
		})
	}
	if resp.Recent == nil {
		resp.Recent = []dto.PointLogItem{}
	}
	return resp, nil
}

/* ============================ 纯函数 ============================ */

// 徽章门槛。数组有序，BadgeOf 取最后一个满足的等级。
var badgeLevels = []struct {
	At    int64
	Label string
}{
	{0, "新手"},
	{10, "活跃"},
	{50, "贡献者"},
	{200, "资深贡献者"},
}

// BadgeOf 按积分算徽章。纯函数，前端也照这份阈值显示进度条。
func BadgeOf(points int64) string {
	label := badgeLevels[0].Label
	for _, b := range badgeLevels {
		if points >= b.At {
			label = b.Label
		}
	}
	return label
}

// nextBadgeOf 返回下一级徽章与所需积分；已最高级时两者为空。
func nextBadgeOf(points int64) (string, int64) {
	for _, b := range badgeLevels {
		if points < b.At {
			return b.Label, b.At
		}
	}
	return "", 0
}

// pointActionText 把动作标识转成给人看的说明。
func pointActionText(action string) string {
	switch action {
	case model.PointActionAcceptAnswer:
		return "回答被采纳"
	case model.PointActionAnswerLiked:
		return "回答被点赞"
	case model.PointActionArticlePass:
		return "教程过审"
	case model.PointActionQuizPass:
		return "题目过审"
	}
	return "贡献积分"
}

// toQaItem 模型行转列表项。
func toQaItem(r *repository.QaQuestionRow) dto.QaQuestionItem {
	return dto.QaQuestionItem{
		ID:           r.ID,
		Title:        r.Title,
		Excerpt:      excerptOf(r.Description, 120),
		UserID:       r.UserID,
		Nickname:     r.Nickname,
		Avatar:       r.Avatar,
		AuthorRole:   r.AuthorRole,
		AnswerCount:  r.AnswerCount,
		ViewCount:    r.ViewCount,
		Status:       r.Status,
		SkillIDs:     stringsToInt64s(r.SkillIDs),
		CreatedAt:    r.CreatedAt,
		LastAnswerAt: r.LastAnswerAt,
		AcceptedAt:   r.AcceptedAt,
		HasAccepted:  r.AcceptedAnswerID > 0,
	}
}

// excerptOf 截取纯文本摘要：Markdown 标记不剥会让列表里出现一堆 ###。
func excerptOf(s string, max int) string {
	s = strings.TrimSpace(s)
	s = strings.NewReplacer("#", "", "*", "", "`", "", ">", "", "[", "", "]", "").Replace(s)
	s = strings.Join(strings.Fields(s), " ")
	if runeLen(s) <= max {
		return s
	}
	return string([]rune(s)[:max]) + "…"
}

// normalizeQaAttachments 校验附件 key。
//
// 与悬赏问题同一套规则：必须来自问题附件场景，否则可以拿私有文件的 key
// 混进来，一次引用就把私有文件变成公开可访问。
func normalizeQaAttachments(keys []string) ([]string, error) {
	maxAttachments, _ := dto.QuestionExtraLimits()
	out := make([]string, 0, len(keys))
	for _, k := range keys {
		k = strings.TrimSpace(k)
		if k == "" {
			continue
		}
		if !strings.HasPrefix(k, questionKeyPrefix) {
			return nil, errcode.ErrResourceFileInvalid.WithMsg("附件必须通过「问题附件」场景上传")
		}
		out = append(out, k)
	}
	if len(out) > maxAttachments {
		return nil, errcode.ErrParamInvalid.WithMsg(fmt.Sprintf("附件最多 %d 个", maxAttachments))
	}
	return out, nil
}

// normalizeQaSkills 校验技术标签个数并去重。
func normalizeQaSkills(ids []int64) ([]int64, error) {
	_, maxSkills := dto.QuestionExtraLimits()
	seen := make(map[int64]struct{}, len(ids))
	out := make([]int64, 0, len(ids))
	for _, id := range ids {
		if id <= 0 {
			continue
		}
		if _, ok := seen[id]; ok {
			continue
		}
		seen[id] = struct{}{}
		out = append(out, id)
	}
	if len(out) > maxSkills {
		return nil, errcode.ErrParamInvalid.WithMsg(fmt.Sprintf("技术标签最多 %d 个", maxSkills))
	}
	return out, nil
}

// int64sToStrings 把标签 ID 转成 StringList 的存储形态。
//
// SkillIDs 在悬赏问题里就是 string 数组（历史原因），问答沿用同一存储，
// 免得同一个概念在两张表里两种类型。
func int64sToStrings(ids []int64) []string {
	out := make([]string, 0, len(ids))
	for _, id := range ids {
		out = append(out, fmt.Sprintf("%d", id))
	}
	return out
}

// stringsToInt64s 反向解析，忽略非法值（脏数据不该让整页报错）。
func stringsToInt64s(list model.StringList) []int64 {
	out := make([]int64, 0, len(list))
	for _, s := range list {
		var id int64
		if _, err := fmt.Sscanf(strings.TrimSpace(s), "%d", &id); err == nil && id > 0 {
			out = append(out, id)
		}
	}
	return out
}
