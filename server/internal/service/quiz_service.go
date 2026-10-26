package service

import (
	"context"
	"fmt"
	"sort"
	"strings"
	"time"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
	"github.com/opsexpert/platform/internal/pkg/logger"
	"github.com/opsexpert/platform/internal/repository"
)

const (
	// defaultPracticeCount 未指定抽题数量时的默认值。
	defaultPracticeCount = 10
	// quizHistoryLimit 答题历史返回条数。
	quizHistoryLimit = 20
	// pointsQuizPublished 用户投稿的题目过审后记的贡献分。
	//
	// 比教程（+20）低是因为工作量不同：写一篇能看的教程要几小时，
	// 出一道带解析的好题十几分钟。定成一样会让人刷题投稿凑分。
	pointsQuizPublished = 5
	// quizImportMaxErrors 导入结果里最多回传多少条行错误。
	//
	// 与行数上限配合：全错时也只有 50 条要展示，前端不必为极端情况做虚拟滚动。
	quizImportMaxErrors = 50
)

// QuizService 运维题库业务逻辑：录题、抽题、判分、错题本。
type QuizService struct {
	quizzes    *repository.QuizRepo
	categories *repository.CategoryRepo
	// points 记题目过审的贡献分；为 nil 时（单测）静默跳过。
	points *repository.PointRepo
}

// NewQuizService 创建 QuizService。
func NewQuizService(quizzes *repository.QuizRepo, categories *repository.CategoryRepo, points *repository.PointRepo) *QuizService {
	return &QuizService{quizzes: quizzes, categories: categories, points: points}
}

// ============================ 前台：浏览 ============================

// Categories 返回题库的分类卡片。
//
// 分类是全站共用的（文章、工具也用它），所以这里必须只返回**有题目的**分类吗？
// 不。返回全部启用分类，让用户看到"还有 Windows Server / 安全加固这些方向"，
// 这本身就是内容规划的信号；前端把 0 题的分类标成"筹备中"并禁用点击即可。
// 若只返回有题的分类，题库看起来永远只有三五个方向，像是做完了。
func (s *QuizService) Categories(ctx context.Context, userID int64) ([]dto.QuizCategoryItem, error) {
	cats, err := s.categories.List(ctx, true)
	if err != nil {
		return nil, err
	}

	counts := map[int64]int64{}
	rows, err := s.quizzes.CountPublishedByCategory(ctx)
	if err != nil {
		// 统计失败不该让分类页打不开：题目数只是角标，
		// 退回全 0 后卡片仍可点击（前端按 count>0 判断是否禁用，这里单独兜底）。
		logger.Warn("统计题库分类题目数失败", "err", err.Error())
	} else {
		for _, r := range rows {
			counts[r.CategoryID] = r.Count
		}
	}

	progress := map[int64]repository.CategoryProgress{}
	if userID > 0 {
		items, err := s.quizzes.CategoryProgressByUser(ctx, userID)
		if err != nil {
			logger.Warn("统计分类刷题进度失败", "err", err.Error())
		} else {
			for _, p := range items {
				progress[p.CategoryID] = p
			}
		}
	}

	out := make([]dto.QuizCategoryItem, 0, len(cats))
	for _, c := range cats {
		p := progress[c.ID]
		out = append(out, dto.QuizCategoryItem{
			ID:       c.ID,
			Name:     c.Name,
			Slug:     c.Slug,
			Count:    counts[c.ID],
			Answered: p.Answered,
			Correct:  p.Correct,
		})
	}
	return out, nil
}

// ListQuestions 查询已发布题目列表。
//
// 出参不含正确答案与解析，答题后才由 Answer / Submit 返回。
func (s *QuizService) ListQuestions(ctx context.Context, q dto.QuizListQuery) ([]dto.QuizQuestionItem, int64, error) {
	filter := repository.QuizFilter{
		Status:        model.QuizStatusPublished,
		CategoryID:    q.CategoryID,
		Difficulty:    q.Difficulty,
		Type:          q.Type,
		KeywordGroups: SearchGroups(q.Keyword),
		Tag:           q.Tag,
		Sort:          q.Sort,
	}
	if filter.CategoryID <= 0 && strings.TrimSpace(q.CategorySlug) != "" {
		cat, err := s.categories.GetBySlug(ctx, strings.TrimSpace(q.CategorySlug))
		if err != nil {
			return nil, 0, err
		}
		filter.CategoryID = cat.ID
	}

	rows, total, err := s.quizzes.List(ctx, filter, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	items, err := s.toItems(ctx, rows)
	if err != nil {
		return nil, 0, err
	}
	return items, total, nil
}

// Practice 随机抽题组成一轮练习。
//
// 抽不到题时返回空的业务错误而不是空列表：前端要据此提示"该条件下暂无题目"，
// 而不是渲染出一个空白页面让用户以为页面坏了。
func (s *QuizService) Practice(ctx context.Context, q dto.QuizPracticeQuery) ([]dto.QuizQuestionItem, error) {
	count := q.Count
	if count <= 0 {
		count = defaultPracticeCount
	}
	if maxCount := dto.QuizAnswerLimits(); count > maxCount {
		count = maxCount
	}

	rows, err := s.quizzes.SampleRandom(ctx, repository.QuizFilter{
		Status:     model.QuizStatusPublished,
		CategoryID: q.CategoryID,
		Difficulty: q.Difficulty,
		Type:       q.Type,
	}, count)
	if err != nil {
		return nil, err
	}
	if len(rows) == 0 {
		return nil, errcode.ErrQuizPoolEmpty
	}
	return s.toItems(ctx, rows)
}

// ============================ 前台：作答 ============================

// Answer 单题作答并判分。
func (s *QuizService) Answer(ctx context.Context, userID, questionID int64, req dto.QuizAnswerReq) (*dto.QuizAnswerResult, error) {
	if userID <= 0 {
		return nil, errcode.ErrUnauthorized
	}
	q, err := s.quizzes.GetByID(ctx, questionID)
	if err != nil {
		return nil, err
	}
	if q.Status != model.QuizStatusPublished {
		return nil, errcode.ErrQuizNotPublished
	}

	chosen, err := normalizeChosen(req.Chosen, q)
	if err != nil {
		return nil, err
	}
	correct := judgeAnswer(q, chosen)

	// 重复作答不重复累加题目统计：否则同一道题刷十遍就能把正确率刷成任意值，
	// 题目页展示的"正确率"就没有任何参考意义了。
	answered, err := s.quizzes.HasAnswered(ctx, userID, q.ID)
	if err != nil {
		logger.Warn("查询作答记录失败，按首次作答处理", "user_id", userID, "question_id", q.ID, "err", err.Error())
		answered = false
	}
	if !answered {
		if err := s.quizzes.AddAnswerStat(ctx, q.ID, correct); err != nil {
			logger.Warn("累加题目作答统计失败", "question_id", q.ID, "err", err.Error())
		}
	}

	rec := &model.QuizRecord{
		UserID:     userID,
		QuestionID: q.ID,
		Chosen:     chosen,
		Correct:    correct,
	}
	if err := s.quizzes.CreateRecord(ctx, rec); err != nil {
		// 流水写入失败不影响本次判分结果，用户该看到的解析照给。
		logger.Warn("写入作答记录失败", "user_id", userID, "question_id", q.ID, "err", err.Error())
	}

	return &dto.QuizAnswerResult{
		QuestionID:     q.ID,
		Chosen:         []string(chosen),
		Correct:        correct,
		CorrectOptions: []string(q.Correct),
		Analysis:       q.Analysis,
		Fresh:          !answered,
	}, nil
}

// Submit 整卷提交并生成成绩单。
func (s *QuizService) Submit(ctx context.Context, userID int64, req dto.QuizSubmitReq) (*dto.QuizSubmitResult, error) {
	if userID <= 0 {
		return nil, errcode.ErrUnauthorized
	}
	if len(req.Items) == 0 {
		return nil, errcode.ErrQuizAnswerInvalid.WithMsg("请至少作答一道题")
	}
	if maxCount := dto.QuizAnswerLimits(); len(req.Items) > maxCount {
		return nil, errcode.ErrQuizAnswerInvalid.WithMsg(
			fmt.Sprintf("单次提交最多 %d 道题", maxCount))
	}

	// 同一题在一次提交里出现两次会导致计分口径混乱，先去重（保留首次作答）。
	items, err := dedupeSubmitItems(req.Items)
	if err != nil {
		return nil, err
	}

	ids := make([]int64, 0, len(items))
	for _, it := range items {
		ids = append(ids, it.QuestionID)
	}
	questions, err := s.quizzes.GetMany(ctx, ids)
	if err != nil {
		return nil, err
	}
	index := make(map[int64]model.QuizQuestion, len(questions))
	for _, q := range questions {
		index[q.ID] = q
	}

	// 一次性查出本轮里用户已经答过的题，避免逐题查一次库。
	answeredSet, err := s.quizzes.AnsweredQuestionIDs(ctx, userID, ids)
	if err != nil {
		logger.Warn("查询已作答题号失败，本轮按首次作答处理", "err", err.Error())
		answeredSet = map[int64]struct{}{}
	}

	results := make([]dto.QuizAnswerResult, 0, len(items))
	records := make([]model.QuizRecord, 0, len(items))
	var correctCount int64

	for _, it := range items {
		q, ok := index[it.QuestionID]
		if !ok {
			// 题目被删除时不中断整卷：跳过并在结果里如实标记，
			// 否则用户答完一整套题却因为一道被删的题拿不到成绩。
			results = append(results, dto.QuizAnswerResult{
				QuestionID: it.QuestionID,
				Chosen:     it.Chosen,
				Correct:    false,
			})
			continue
		}
		if q.Status != model.QuizStatusPublished {
			results = append(results, dto.QuizAnswerResult{
				QuestionID:     q.ID,
				Chosen:         it.Chosen,
				Correct:        false,
				CorrectOptions: []string(q.Correct),
				Analysis:       q.Analysis,
			})
			continue
		}

		chosen, err := normalizeChosen(it.Chosen, &q)
		if err != nil {
			return nil, err
		}
		correct := judgeAnswer(&q, chosen)
		if correct {
			correctCount++
		}

		_, answered := answeredSet[q.ID]
		if !answered {
			if err := s.quizzes.AddAnswerStat(ctx, q.ID, correct); err != nil {
				logger.Warn("累加题目作答统计失败", "question_id", q.ID, "err", err.Error())
			}
		}

		results = append(results, dto.QuizAnswerResult{
			QuestionID:     q.ID,
			Chosen:         []string(chosen),
			Correct:        correct,
			CorrectOptions: []string(q.Correct),
			Analysis:       q.Analysis,
			Fresh:          !answered,
		})
		records = append(records, model.QuizRecord{
			UserID:     userID,
			QuestionID: q.ID,
			Chosen:     chosen,
			Correct:    correct,
		})
	}

	total := int64(len(results))
	score := 0
	if total > 0 {
		// 四舍五入到整数分，避免出现 66.67 这种前端还要再格式化一次的值。
		score = int((correctCount*100 + total/2) / total)
	}

	attempt := &model.QuizAttempt{
		UserID:        userID,
		QuestionCount: total,
		CorrectCount:  correctCount,
		Score:         score,
		DurationSec:   req.DurationSec,
		CategoryID:    req.CategoryID,
	}
	if err := s.quizzes.CreateAttemptWithRecords(ctx, attempt, records); err != nil {
		return nil, err
	}

	return &dto.QuizSubmitResult{
		AttemptID:     attempt.ID,
		QuestionCount: total,
		CorrectCount:  correctCount,
		Score:         score,
		DurationSec:   req.DurationSec,
		Items:         results,
	}, nil
}

// Attempts 答题历史。
func (s *QuizService) Attempts(ctx context.Context, userID int64) ([]dto.QuizAttemptItem, error) {
	if userID <= 0 {
		return []dto.QuizAttemptItem{}, nil
	}
	rows, err := s.quizzes.ListAttempts(ctx, userID, quizHistoryLimit)
	if err != nil {
		return nil, err
	}
	items := make([]dto.QuizAttemptItem, 0, len(rows))
	for _, a := range rows {
		items = append(items, dto.QuizAttemptItem{
			ID:            a.ID,
			QuestionCount: a.QuestionCount,
			CorrectCount:  a.CorrectCount,
			Score:         a.Score,
			DurationSec:   a.DurationSec,
			CategoryID:    a.CategoryID,
			CreatedAt:     a.CreatedAt,
		})
	}
	return items, nil
}

// Stats 个人答题总览。
func (s *QuizService) Stats(ctx context.Context, userID int64) (*dto.QuizStats, error) {
	out := &dto.QuizStats{}
	if userID <= 0 {
		return out, nil
	}

	row, err := s.quizzes.Stats(ctx, userID)
	if err != nil {
		return nil, err
	}
	out.TotalAnswered = row.TotalAnswered
	out.TotalCorrect = row.TotalCorrect
	if row.TotalAnswered > 0 {
		out.Accuracy = float64(row.TotalCorrect) / float64(row.TotalAnswered) * 100
	}

	_, total, err := s.quizzes.ListWrong(ctx, userID, 0, 0, 1)
	if err != nil {
		logger.Warn("统计错题数失败", "user_id", userID, "err", err.Error())
	} else {
		out.WrongCount = total
	}

	count, err := s.quizzes.CountAttempts(ctx, userID)
	if err != nil {
		logger.Warn("统计练习轮次失败", "user_id", userID, "err", err.Error())
	} else {
		out.AttemptCount = count
	}
	return out, nil
}

// WrongBook 错题本。
func (s *QuizService) WrongBook(ctx context.Context, userID int64, q dto.QuizWrongQuery) ([]dto.QuizWrongItem, int64, error) {
	if userID <= 0 {
		return []dto.QuizWrongItem{}, 0, nil
	}
	rows, total, err := s.quizzes.ListWrong(ctx, userID, q.CategoryID, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	if len(rows) == 0 {
		return []dto.QuizWrongItem{}, 0, nil
	}

	ids := make([]int64, 0, len(rows))
	for _, r := range rows {
		ids = append(ids, r.QuestionID)
	}
	questions, err := s.quizzes.GetMany(ctx, ids)
	if err != nil {
		return nil, 0, err
	}
	index := make(map[int64]model.QuizQuestion, len(questions))
	for _, item := range questions {
		index[item.ID] = item
	}

	cats, err := s.categoryIndex(ctx)
	if err != nil {
		logger.Warn("批量查询题目分类失败，分类名将留空", "err", err.Error())
		cats = map[int64]model.ArticleCategory{}
	}

	items := make([]dto.QuizWrongItem, 0, len(rows))
	for _, row := range rows {
		question, ok := index[row.QuestionID]
		if !ok {
			// 题目已被删除：跳过而不是报错，否则用户整个错题本都打不开。
			continue
		}
		items = append(items, dto.QuizWrongItem{
			QuestionID:     question.ID,
			Stem:           question.Stem,
			Type:           question.Type,
			Difficulty:     question.Difficulty,
			Options:        toOptionItems(question.Options),
			Chosen:         []string(row.Chosen),
			CorrectOptions: []string(question.Correct),
			Analysis:       question.Analysis,
			CategoryID:     question.CategoryID,
			CategoryName:   cats[question.CategoryID].Name,
			LastWrongAt:    row.LastWrongAt,
		})
	}
	return items, total, nil
}

// ============================ 后台 ============================

// AdminList 后台题目列表，可查全部状态，含答案与解析。
func (s *QuizService) AdminList(ctx context.Context, q dto.AdminQuizListQuery) ([]dto.AdminQuizItem, int64, error) {
	rows, total, err := s.quizzes.List(ctx, repository.QuizFilter{
		Status:        q.Status,
		CategoryID:    q.CategoryID,
		Difficulty:    q.Difficulty,
		KeywordGroups: SearchGroups(q.Keyword),
		Sort:          "latest",
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}

	base, err := s.toItems(ctx, rows)
	if err != nil {
		return nil, 0, err
	}

	items := make([]dto.AdminQuizItem, 0, len(base))
	for i := range base {
		items = append(items, dto.AdminQuizItem{
			QuizQuestionItem: base[i],
			Correct:          []string(rows[i].Correct),
			Analysis:         rows[i].Analysis,
			Status:           rows[i].Status,
			AuthorID:         rows[i].AuthorID,
			CreatedAt:        rows[i].CreatedAt,
			UpdatedAt:        rows[i].UpdatedAt,
			ReviewNote:       rows[i].ReviewNote,
			ReviewerID:       rows[i].ReviewerID,
			ReviewedAt:       rows[i].ReviewedAt,
		})
	}
	return items, total, nil
}

// AdminGet 后台读取单题（含答案）。
func (s *QuizService) AdminGet(ctx context.Context, id int64) (*dto.AdminQuizItem, error) {
	q, err := s.quizzes.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	base, err := s.toItems(ctx, []model.QuizQuestion{*q})
	if err != nil {
		return nil, err
	}
	return &dto.AdminQuizItem{
		QuizQuestionItem: base[0],
		Correct:          []string(q.Correct),
		Analysis:         q.Analysis,
		Status:           q.Status,
		AuthorID:         q.AuthorID,
		CreatedAt:        q.CreatedAt,
		UpdatedAt:        q.UpdatedAt,
		ReviewNote:       q.ReviewNote,
		ReviewerID:       q.ReviewerID,
		ReviewedAt:       q.ReviewedAt,
	}, nil
}

// Create 新建题目。
func (s *QuizService) Create(ctx context.Context, adminID int64, req dto.QuizQuestionCreateReq) (*dto.AdminQuizItem, error) {
	fields, err := s.prepareFields(ctx, quizInput{
		Stem:       req.Stem,
		Type:       req.Type,
		Options:    req.Options,
		Correct:    req.Correct,
		Analysis:   req.Analysis,
		Difficulty: req.Difficulty,
		CategoryID: req.CategoryID,
		Tags:       req.Tags,
	})
	if err != nil {
		return nil, err
	}

	status := model.QuizStatusDraft
	if req.Status == model.QuizStatusPublished {
		status = model.QuizStatusPublished
	}

	now := time.Now()
	q := &model.QuizQuestion{
		Stem:       fields.stem,
		Type:       fields.qType,
		Options:    fields.options,
		Correct:    fields.correct,
		Analysis:   fields.analysis,
		Difficulty: fields.difficulty,
		CategoryID: fields.categoryID,
		Tags:       fields.tags,
		Status:     status,
		AuthorID:   adminID,
	}
	if status == model.QuizStatusPublished {
		q.PublishedAt = &now
	}
	if err := s.quizzes.Create(ctx, q); err != nil {
		return nil, err
	}
	return s.AdminGet(ctx, q.ID)
}

// Update 局部更新题目。
func (s *QuizService) Update(ctx context.Context, id int64, req dto.QuizQuestionUpdateReq) (*dto.AdminQuizItem, error) {
	current, err := s.quizzes.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}

	updates := make(map[string]any, 8)

	if req.Stem != nil {
		stem, err := validateText(*req.Stem, 1, quizLimits().stem, errcode.ErrParamInvalid, "题干")
		if err != nil {
			return nil, err
		}
		updates["stem"] = stem
	}
	if req.Analysis != nil {
		analysis, err := validateOptionalText(*req.Analysis, quizLimits().analysis, errcode.ErrParamInvalid, "解析")
		if err != nil {
			return nil, err
		}
		updates["analysis"] = analysis
	}
	if req.Difficulty != nil {
		difficulty, err := normalizeDifficulty(*req.Difficulty)
		if err != nil {
			return nil, err
		}
		updates["difficulty"] = difficulty
	}
	if req.CategoryID != nil {
		if err := s.checkCategory(ctx, *req.CategoryID); err != nil {
			return nil, err
		}
		updates["category_id"] = *req.CategoryID
	}
	if req.Tags != nil {
		tags, err := normalizeContentTags(*req.Tags)
		if err != nil {
			return nil, err
		}
		updates["tags"] = tags
	}

	// 选项、题型、答案三者互相约束，任一被改动都要整体重算一遍校验，
	// 否则会出现"改了选项但答案还指向已被删掉的选项"这种脏数据。
	if req.Options != nil || req.Type != nil || req.Correct != nil {
		merged := quizInput{Type: current.Type}
		merged.Options = toOptionInputs(current.Options)
		merged.Correct = []string(current.Correct)
		if req.Type != nil {
			merged.Type = *req.Type
		}
		if req.Options != nil {
			merged.Options = *req.Options
		}
		if req.Correct != nil {
			merged.Correct = *req.Correct
		}

		qType, err := normalizeQuizType(merged.Type)
		if err != nil {
			return nil, err
		}
		options, correct, err := normalizeOptions(qType, merged.Options, merged.Correct)
		if err != nil {
			return nil, err
		}
		updates["type"] = qType
		updates["options"] = options
		updates["correct"] = correct
	}

	if err := s.quizzes.UpdateFields(ctx, id, updates); err != nil {
		return nil, err
	}
	return s.AdminGet(ctx, id)
}

// Delete 删除题目。
func (s *QuizService) Delete(ctx context.Context, id int64) error {
	if _, err := s.quizzes.GetByID(ctx, id); err != nil {
		return err
	}
	return s.quizzes.Delete(ctx, id)
}

// Publish 发布题目。
func (s *QuizService) Publish(ctx context.Context, id int64) (*dto.AdminQuizItem, error) {
	q, err := s.quizzes.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q.Status == model.QuizStatusPublished {
		return nil, errcode.ErrQuizStateInvalid.WithMsg("该题目已是发布状态")
	}

	updates := map[string]any{"status": model.QuizStatusPublished}
	if q.PublishedAt == nil {
		updates["published_at"] = time.Now()
	}
	if err := s.quizzes.UpdateFields(ctx, id, updates); err != nil {
		return nil, err
	}
	return s.AdminGet(ctx, id)
}

// Offline 下线题目。
func (s *QuizService) Offline(ctx context.Context, id int64) (*dto.AdminQuizItem, error) {
	q, err := s.quizzes.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q.Status != model.QuizStatusPublished {
		return nil, errcode.ErrQuizStateInvalid.WithMsg("只有已发布的题目可以下线")
	}
	if err := s.quizzes.UpdateFields(ctx, id, map[string]any{"status": model.QuizStatusOffline}); err != nil {
		return nil, err
	}
	return s.AdminGet(ctx, id)
}

// Import 批量导入题目。
//
// 三条规则，缺一条这个功能就不好用：
//  1. **先全量校验，再一次性落库**。边校验边插的话，插到第 7 行发现第 8 行错，
//     前 7 条已经进库；用户改完重传，那 7 条就重复了——题库没有天然唯一键，
//     重复题不会被任何约束挡住，只会靠人工发现。
//  2. **错误带行号**，且一次给出全部。只报第一条的话几十道题要传几十趟。
//  3. **部分失败就一条都不落**。半批入库是最难收拾的状态：用户不知道哪几条进去了，
//     再传一次又怕重复。
//
// 返回 200 + 结果体而不是 400：错误清单本身就是要给用户逐行看的数据，
// 塞进 errcode 只能带出第一条。
func (s *QuizService) Import(ctx context.Context, adminID int64, req dto.QuizQuestionImportReq) (*dto.QuizImportResult, error) {
	if len(req.Items) == 0 {
		return nil, errcode.ErrParamInvalid.WithMsg("请提供要导入的题目")
	}
	if len(req.Items) > dto.QuizImportMaxRows() {
		return nil, errcode.ErrParamInvalid.WithMsg(
			fmt.Sprintf("单次最多导入 %d 道题，请分批提交", dto.QuizImportMaxRows()))
	}

	cats, err := s.categoryIndex(ctx)
	if err != nil {
		// 分类索引取不到不该让整批导入失败——分类只影响展示与筛选，
		// 题目本身照样能入库。降级成"不校验分类"，让运营先把题导进来。
		logger.Warn("批量导入时读取分类失败，跳过分类校验", "err", err.Error())
		cats = map[int64]model.ArticleCategory{}
	}

	prepared := make([]preparedQuiz, 0, len(req.Items))
	errs := make([]dto.QuizImportError, 0)

	for i, in := range req.Items {
		row := i + 1
		p, perr := prepareQuizFields(quizInput{
			Stem: in.Stem, Type: in.Type, Options: in.Options, Correct: in.Correct,
			Analysis: in.Analysis, Difficulty: in.Difficulty, CategoryID: in.CategoryID, Tags: in.Tags,
		})
		if perr == nil && p.categoryID > 0 {
			if _, ok := cats[p.categoryID]; !ok {
				perr = errcode.ErrParamInvalid.WithMsg(fmt.Sprintf("分类 %d 不存在", p.categoryID))
			}
		}
		if perr != nil {
			if len(errs) < quizImportMaxErrors {
				errs = append(errs, dto.QuizImportError{
					Row: row, Stem: truncateRunes(strings.TrimSpace(in.Stem), 30), Message: errcode.From(perr).Msg,
				})
			}
			continue
		}
		prepared = append(prepared, p)
	}

	result := &dto.QuizImportResult{Total: len(req.Items), Imported: 0, Errors: errs}
	if len(errs) > 0 {
		// 一条都不落：prepared 里已经校验通过的也不插，避免半批入库。
		return result, nil
	}

	items := make([]*model.QuizQuestion, 0, len(prepared))
	for _, p := range prepared {
		items = append(items, &model.QuizQuestion{
			Stem: p.stem, Type: p.qType, Options: p.options, Correct: p.correct,
			Analysis: p.analysis, Difficulty: p.difficulty, CategoryID: p.categoryID, Tags: p.tags,
			// 导入的题一律先落草稿：批量灌进来的题没人逐条看过，
			// 直接发布等于把未经检查的内容推给刷题用户。
			// 运营确认后再用「发布」批量上线（列表支持按状态筛，逐条点也不慢）。
			Status:   model.QuizStatusDraft,
			AuthorID: adminID,
		})
	}
	if err := s.quizzes.CreateMany(ctx, items); err != nil {
		return nil, err
	}
	result.Imported = len(items)
	return result, nil
}

// Contribute 用户投稿一道题。
//
// status 固定 pending，作者固定取登录态——这两个值一旦让调用方传，
// 就有「自己发布」和「冒名投稿」两个后门（与文章投稿同一口径）。
// directPublish 由 handler 依据「已过审专家 / 管理员」判定后传入，
// 真正的判定只在一处，免得 service 里再拼一遍角色条件、两边慢慢走岔。
func (s *QuizService) Contribute(
	ctx context.Context, userID int64, req dto.QuizContributeReq, directPublish bool,
) (*dto.QuizContributeResp, error) {
	fields, err := s.prepareFields(ctx, quizInput{
		Stem: req.Stem, Type: req.Type, Options: req.Options, Correct: req.Correct,
		Analysis: req.Analysis, Difficulty: req.Difficulty, CategoryID: req.CategoryID, Tags: req.Tags,
	})
	if err != nil {
		return nil, err
	}
	// 后台录题允许解析留空（运营补历史题时常见），投稿不允许：
	// 用户是照着"我错过这道题"来的，没有解析他就只记住了答案字母。
	if strings.TrimSpace(fields.analysis) == "" {
		return nil, errcode.ErrParamInvalid.WithMsg("请填写答案解析：没有解析的题，答错的人也不知道错在哪")
	}

	status := model.QuizStatusPending
	if directPublish {
		status = model.QuizStatusPublished
	}

	now := time.Now()
	q := &model.QuizQuestion{
		Stem: fields.stem, Type: fields.qType, Options: fields.options, Correct: fields.correct,
		Analysis: fields.analysis, Difficulty: fields.difficulty, CategoryID: fields.categoryID,
		Tags: fields.tags, Status: status, AuthorID: userID,
	}
	if status == model.QuizStatusPublished {
		q.PublishedAt = &now
	}
	if err := s.quizzes.Create(ctx, q); err != nil {
		return nil, err
	}
	// 直发也记分：否则「专家直发」比「投稿过审」少 5 分，等于惩罚效率高的人。
	if status == model.QuizStatusPublished {
		s.awardQuizPoints(ctx, q.ID, userID)
	}
	return &dto.QuizContributeResp{ID: q.ID, Status: status}, nil
}

// ReviewQuestion 后台审核投稿题目。
//
// 用 CAS（WHERE status='pending'）：两个管理员同时点「通过」，
// 第二次命中 0 行并报"已被其他人处理"，而不是把审核人再覆盖一遍。
func (s *QuizService) ReviewQuestion(
	ctx context.Context, id, reviewerID int64, req dto.QuizReviewReq,
) (*dto.AdminQuizItem, error) {
	q, err := s.quizzes.GetByID(ctx, id)
	if err != nil {
		return nil, err
	}
	if q.Status != model.QuizStatusPending {
		return nil, errcode.ErrQuizStateInvalid.WithMsg("这道题不在待审核状态")
	}

	note := strings.TrimSpace(req.Note)
	if n := runeLen(note); n > 200 {
		return nil, errcode.ErrParamInvalid.WithMsg("审核说明不能超过 200 个字")
	}
	now := time.Now()
	updates := map[string]any{
		"reviewer_id": reviewerID,
		"reviewed_at": now, // 不用 CURRENT_TIMESTAMP：SQLite 会写成 UTC，差 8 小时
		"review_note": note,
	}

	pass := req.Action == "pass"
	if pass {
		updates["status"] = model.QuizStatusPublished
		updates["published_at"] = now
	} else {
		// 驳回必须填原因：不填的话投稿人只能反复试错，而题目不像文章
		// 那样能从上下文猜出问题——可能是选项设置，也可能是答案标错。
		if note == "" {
			return nil, errcode.ErrParamInvalid.WithMsg("驳回必须填写原因，方便投稿人修改")
		}
		updates["status"] = model.QuizStatusRejected
	}

	ok, err := s.quizzes.UpdateFieldsIfStatus(ctx, id, model.QuizStatusPending, updates)
	if err != nil {
		return nil, err
	}
	if !ok {
		return nil, errcode.ErrQuizStateInvalid.WithMsg("这道题已被其他人处理，请刷新后再看")
	}
	if pass {
		s.awardQuizPoints(ctx, id, q.AuthorID)
	}
	return s.AdminGet(ctx, id)
}

// quizPointsAwarder 是记分依赖的最小接口，便于测试替换。
func (s *QuizService) awardQuizPoints(ctx context.Context, questionID, userID int64) {
	if s.points == nil || userID <= 0 {
		return
	}
	if _, err := s.points.Award(ctx, &model.PointLog{
		UserID: userID, Action: model.PointActionQuizPass,
		Points: pointsQuizPublished, TargetType: "quiz", TargetID: questionID,
		UniqKey: fmt.Sprintf("quiz.publish:%d", questionID),
	}); err != nil {
		logger.Warn("记录题目积分失败", "question_id", questionID, "err", err.Error())
	}
}

// MyContributions 我投的题。
//
// 单独一个接口而不是塞进「我的投稿」：文章与题目在不同表、翻页要跨表排序，
// 合并成一个游标列表的复杂度远大于收益。前端在同一页用两个 tab 展示，
// 用户看到的仍是一个入口。
func (s *QuizService) MyContributions(ctx context.Context, userID int64, q dto.PageQuery) ([]dto.MyContributionItem, int64, error) {
	q.Normalize()
	rows, total, err := s.quizzes.List(ctx, repository.QuizFilter{
		AuthorID: userID,
		Sort:     "latest",
	}, q.Offset(), q.Limit())
	if err != nil {
		return nil, 0, err
	}
	items := make([]dto.MyContributionItem, 0, len(rows))
	for i := range rows {
		items = append(items, dto.MyContributionItem{
			Kind:        "quiz",
			ID:          rows[i].ID,
			Title:       truncateRunes(rows[i].Stem, 80),
			Status:      rows[i].Status,
			ReviewNote:  rows[i].ReviewNote,
			ReviewedAt:  rows[i].ReviewedAt,
			CreatedAt:   rows[i].CreatedAt,
			PublishedAt: rows[i].PublishedAt,
		})
	}
	return items, total, nil
}

// ============================ 内部 ============================

type quizInput struct {
	Stem       string
	Type       string
	Options    []dto.QuizOptionInput
	Correct    []string
	Analysis   string
	Difficulty string
	CategoryID int64
	Tags       []string
}

type preparedQuiz struct {
	stem       string
	qType      string
	options    model.QuizOptionList
	correct    model.StringList
	analysis   string
	difficulty string
	categoryID int64
	tags       model.StringList
}

func (s *QuizService) prepareFields(ctx context.Context, in quizInput) (preparedQuiz, error) {
	out, err := prepareQuizFields(in)
	if err != nil {
		return preparedQuiz{}, err
	}
	if err := s.checkCategory(ctx, in.CategoryID); err != nil {
		return preparedQuiz{}, err
	}
	return out, nil
}

// prepareQuizFields 是纯校验，不碰数据库。
//
// 单独拆出来是为了批量导入：200 行逐条走 checkCategory 会查 200 次分类表，
// 而分类只有十几个。导入时先取一次分类索引、再对每行调这个纯函数。
func prepareQuizFields(in quizInput) (preparedQuiz, error) {
	var out preparedQuiz
	var err error

	if out.stem, err = validateText(in.Stem, 1, quizLimits().stem, errcode.ErrParamInvalid, "题干"); err != nil {
		return out, err
	}
	if out.qType, err = normalizeQuizType(in.Type); err != nil {
		return out, err
	}
	if out.options, out.correct, err = normalizeOptions(out.qType, in.Options, in.Correct); err != nil {
		return out, err
	}
	if out.analysis, err = validateOptionalText(in.Analysis, quizLimits().analysis, errcode.ErrParamInvalid, "解析"); err != nil {
		return out, err
	}
	if out.difficulty, err = normalizeDifficulty(in.Difficulty); err != nil {
		return out, err
	}
	if out.tags, err = normalizeContentTags(in.Tags); err != nil {
		return out, err
	}
	out.categoryID = in.CategoryID
	return out, nil
}

func (s *QuizService) checkCategory(ctx context.Context, categoryID int64) error {
	if categoryID <= 0 {
		return nil
	}
	_, err := s.categories.GetByID(ctx, categoryID)
	return err
}

func (s *QuizService) categoryIndex(ctx context.Context) (map[int64]model.ArticleCategory, error) {
	cats, err := s.categories.List(ctx, false)
	if err != nil {
		return nil, err
	}
	index := make(map[int64]model.ArticleCategory, len(cats))
	for _, c := range cats {
		index[c.ID] = c
	}
	return index, nil
}

func (s *QuizService) toItems(ctx context.Context, rows []model.QuizQuestion) ([]dto.QuizQuestionItem, error) {
	items := make([]dto.QuizQuestionItem, 0, len(rows))
	if len(rows) == 0 {
		return items, nil
	}

	cats, err := s.categoryIndex(ctx)
	if err != nil {
		logger.Warn("批量查询题目分类失败，分类名将留空", "err", err.Error())
		cats = map[int64]model.ArticleCategory{}
	}
	for i := range rows {
		items = append(items, buildQuizItem(rows[i], cats))
	}
	return items, nil
}

func buildQuizItem(q model.QuizQuestion, cats map[int64]model.ArticleCategory) dto.QuizQuestionItem {
	item := dto.QuizQuestionItem{
		ID:          q.ID,
		Stem:        q.Stem,
		Type:        q.Type,
		Options:     toOptionItems(q.Options),
		Difficulty:  q.Difficulty,
		CategoryID:  q.CategoryID,
		Tags:        []string(q.Tags),
		AnswerCount: q.AnswerCount,
		CorrectRate: q.CorrectRate(),
		PublishedAt: q.PublishedAt,
	}
	if item.Tags == nil {
		item.Tags = []string{}
	}
	if cat, ok := cats[q.CategoryID]; ok {
		item.CategoryName = cat.Name
		item.CategorySlug = cat.Slug
	}
	return item
}

func toOptionItems(options model.QuizOptionList) []dto.QuizOptionItem {
	items := make([]dto.QuizOptionItem, 0, len(options))
	for _, o := range options {
		items = append(items, dto.QuizOptionItem{Key: o.Key, Text: o.Text})
	}
	return items
}

func toOptionInputs(options model.QuizOptionList) []dto.QuizOptionInput {
	items := make([]dto.QuizOptionInput, 0, len(options))
	for _, o := range options {
		items = append(items, dto.QuizOptionInput{Key: o.Key, Text: o.Text})
	}
	return items
}

// normalizeQuizType 校验题型，空串按单选处理。
func normalizeQuizType(raw string) (string, error) {
	switch t := strings.ToLower(strings.TrimSpace(raw)); t {
	case "":
		return model.QuizTypeSingle, nil
	case model.QuizTypeSingle, model.QuizTypeMultiple, model.QuizTypeJudge:
		return t, nil
	default:
		return "", errcode.ErrParamInvalid.WithMsg("题型不合法：" + raw)
	}
}

// normalizeDifficulty 校验难度，空串按中等处理。
func normalizeDifficulty(raw string) (string, error) {
	switch d := strings.ToLower(strings.TrimSpace(raw)); d {
	case "":
		return model.QuizDifficultyMedium, nil
	case model.QuizDifficultyEasy, model.QuizDifficultyMedium, model.QuizDifficultyHard:
		return d, nil
	default:
		return "", errcode.ErrParamInvalid.WithMsg("难度不合法：" + raw)
	}
}

// normalizeOptions 校验并规范化选项与答案。
//
// 三种题型各有约束，这里一次讲清：
//   - 判断题固定「正确 / 错误」两个选项，不允许自由录入，否则同一个意思
//     会出现"对/错""是/否""True/False"三种写法，统计与复习都会乱；
//   - 单选与判断的答案必须恰好一个；
//   - 多选至少一个，且不能等于全部选项——全选即正确答案会让题目失去区分度。
func normalizeOptions(qType string, rawOptions []dto.QuizOptionInput, rawCorrect []string) (model.QuizOptionList, model.StringList, error) {
	minOptions, maxOptions := dto.QuizOptionLimits()

	var options model.QuizOptionList
	if qType == model.QuizTypeJudge {
		if len(rawOptions) != 0 && len(rawOptions) != 2 {
			return nil, nil, errcode.ErrQuizOptionInvalid.WithMsg("判断题固定为两个选项")
		}
		options = model.QuizOptionList{
			{Key: "A", Text: "正确"},
			{Key: "B", Text: "错误"},
		}
	} else {
		if len(rawOptions) < minOptions || len(rawOptions) > maxOptions {
			return nil, nil, errcode.ErrQuizOptionInvalid.WithMsg(
				fmt.Sprintf("选项数量需为 %d-%d 个", minOptions, maxOptions))
		}
		options = make(model.QuizOptionList, 0, len(rawOptions))
		seen := make(map[string]struct{}, len(rawOptions))
		for _, o := range rawOptions {
			key := normalizeOptionKey(o.Key)
			if key == "" {
				return nil, nil, errcode.ErrQuizOptionInvalid.WithMsg("选项标识不能为空")
			}
			if _, dup := seen[key]; dup {
				return nil, nil, errcode.ErrQuizOptionInvalid.WithMsg("选项标识重复：" + key)
			}
			text := strings.TrimSpace(o.Text)
			if text == "" {
				return nil, nil, errcode.ErrQuizOptionInvalid.WithMsg("选项内容不能为空")
			}
			if len([]rune(text)) > optionTextMaxLen {
				return nil, nil, errcode.ErrQuizOptionInvalid.WithMsg(
					fmt.Sprintf("选项内容不超过 %d 个字符", optionTextMaxLen))
			}
			seen[key] = struct{}{}
			options = append(options, model.QuizOption{Key: key, Text: text})
		}
	}

	valid := make(map[string]struct{}, len(options))
	for _, o := range options {
		valid[o.Key] = struct{}{}
	}

	correct := make(model.StringList, 0, len(rawCorrect))
	seenCorrect := make(map[string]struct{}, len(rawCorrect))
	for _, c := range rawCorrect {
		key := normalizeOptionKey(c)
		if key == "" {
			continue
		}
		if _, ok := valid[key]; !ok {
			return nil, nil, errcode.ErrQuizOptionInvalid.WithMsg("答案指向了不存在的选项：" + key)
		}
		if _, dup := seenCorrect[key]; dup {
			continue
		}
		seenCorrect[key] = struct{}{}
		correct = append(correct, key)
	}
	// 排序后入库，保证同样的答案集合在数据库里是同一个值，
	// 便于日后按答案做统计或人工比对。
	sort.Strings(correct)

	if len(correct) == 0 {
		return nil, nil, errcode.ErrQuizOptionInvalid.WithMsg("请设置正确答案")
	}
	switch qType {
	case model.QuizTypeSingle, model.QuizTypeJudge:
		if len(correct) != 1 {
			return nil, nil, errcode.ErrQuizOptionInvalid.WithMsg("单选题与判断题只能有一个正确答案")
		}
	case model.QuizTypeMultiple:
		if len(correct) >= len(options) {
			return nil, nil, errcode.ErrQuizOptionInvalid.WithMsg("多选题的正确答案不能覆盖全部选项")
		}
	}
	return options, correct, nil
}

// optionTextMaxLen 单个选项内容的长度上限。
const optionTextMaxLen = 500

// normalizeOptionKey 把选项标识规整为大写单字符。
// 只允许单字符是因为前端按 A/B/C/D 渲染键盘快捷键，多字符标识无法对应。
func normalizeOptionKey(raw string) string {
	key := strings.ToUpper(strings.TrimSpace(raw))
	if len([]rune(key)) != 1 {
		return ""
	}
	r := []rune(key)[0]
	switch {
	case r >= 'A' && r <= 'Z', r >= '0' && r <= '9':
		return key
	default:
		return ""
	}
}

// normalizeChosen 校验用户作答：选项必须真实存在，去重并排序。
func normalizeChosen(raw []string, q *model.QuizQuestion) (model.StringList, error) {
	if len(raw) == 0 {
		return nil, errcode.ErrQuizAnswerInvalid.WithMsg("请选择答案")
	}
	if q.Type != model.QuizTypeMultiple && len(raw) > 1 {
		return nil, errcode.ErrQuizAnswerInvalid.WithMsg("该题只能选择一个答案")
	}

	valid := make(map[string]struct{}, len(q.Options))
	for _, o := range q.Options {
		valid[o.Key] = struct{}{}
	}

	seen := make(map[string]struct{}, len(raw))
	out := make(model.StringList, 0, len(raw))
	for _, c := range raw {
		key := normalizeOptionKey(c)
		if key == "" {
			continue
		}
		if _, ok := valid[key]; !ok {
			return nil, errcode.ErrQuizAnswerInvalid.WithMsg("选项不存在：" + key)
		}
		if _, dup := seen[key]; dup {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, key)
	}
	if len(out) == 0 {
		return nil, errcode.ErrQuizAnswerInvalid.WithMsg("请选择答案")
	}
	sort.Strings(out)
	return out, nil
}

// judgeAnswer 判分：集合相等才算对。
//
// 不用"答对部分选项就给部分分"：运维场景里的多选往往是操作规程
// （"以下哪些操作是安全的"），漏选一项就意味着操作不完整，
// 给半分反而会传递错误的掌握信号。
func judgeAnswer(q *model.QuizQuestion, chosen model.StringList) bool {
	if len(chosen) != len(q.Correct) {
		return false
	}
	expected := make(map[string]struct{}, len(q.Correct))
	for _, c := range q.Correct {
		expected[strings.ToUpper(c)] = struct{}{}
	}
	for _, c := range chosen {
		if _, ok := expected[strings.ToUpper(c)]; !ok {
			return false
		}
	}
	return true
}

// dedupeSubmitItems 去掉同一题在单次提交里的重复项，保留首次作答。
func dedupeSubmitItems(items []dto.QuizSubmitItem) ([]dto.QuizSubmitItem, error) {
	seen := make(map[int64]struct{}, len(items))
	out := make([]dto.QuizSubmitItem, 0, len(items))
	for _, it := range items {
		if it.QuestionID <= 0 {
			return nil, errcode.ErrQuizAnswerInvalid.WithMsg("题目 ID 不合法")
		}
		if _, dup := seen[it.QuestionID]; dup {
			continue
		}
		seen[it.QuestionID] = struct{}{}
		out = append(out, it)
	}
	return out, nil
}

type quizLimitSet struct {
	stem, analysis int
}

func quizLimits() quizLimitSet {
	stem, analysis := dto.QuizLimits()
	return quizLimitSet{stem, analysis}
}
