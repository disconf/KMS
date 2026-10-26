package dto

import "time"

// ============================ 查询入参 ============================

// QuizListQuery 前台题目列表查询条件。
type QuizListQuery struct {
	PageQuery

	CategoryID   int64  `form:"categoryId"`
	CategorySlug string `form:"categorySlug"`
	Keyword      string `form:"keyword"`
	Difficulty   string `form:"difficulty" binding:"omitempty,oneof=easy medium hard"`
	Type         string `form:"type" binding:"omitempty,oneof=single multiple judge"`
	Tag          string `form:"tag"`
	// Sort 取值 latest（默认）/ hot（按作答数）/ hard（正确率升序）。
	Sort string `form:"sort" binding:"omitempty,oneof=latest hot hard"`
}

// AdminQuizListQuery 后台题目列表查询条件。
type AdminQuizListQuery struct {
	PageQuery

	Status     string `form:"status" binding:"omitempty,oneof=draft pending published rejected offline"`
	CategoryID int64  `form:"categoryId"`
	Difficulty string `form:"difficulty" binding:"omitempty,oneof=easy medium hard"`
	Keyword    string `form:"keyword"`
}

// QuizPracticeQuery 随机练习抽题条件。
type QuizPracticeQuery struct {
	// Count 抽题数量，超出题库总量时按实际数量返回，不报错。
	Count      int    `form:"count" binding:"omitempty,min=1,max=50"`
	CategoryID int64  `form:"categoryId"`
	Difficulty string `form:"difficulty" binding:"omitempty,oneof=easy medium hard"`
	Type       string `form:"type" binding:"omitempty,oneof=single multiple judge"`
}

// QuizWrongQuery 错题本查询条件。
type QuizWrongQuery struct {
	PageQuery

	CategoryID int64 `form:"categoryId"`
}

// ============================ 题目出参 ============================

// QuizCategoryItem 题库分类卡片：一个运维方向的题目数与我的进度。
//
// Answered / Correct 按题目去重而不是按作答次数：
// 同一道题刷三遍，进度条不该走到三倍。未登录时两者恒为 0。
type QuizCategoryItem struct {
	ID    int64  `json:"id"`
	Name  string `json:"name"`
	Slug  string `json:"slug"`
	Count int64  `json:"count"`
	// Answered 已作答过的题目数；Correct 其中答对过的题目数。
	Answered int64 `json:"answered"`
	Correct  int64 `json:"correct"`
}

// QuizOptionItem 选项。key 用 A/B/C/D，前端直接渲染。
type QuizOptionItem struct {
	Key  string `json:"key"`
	Text string `json:"text"`
}

// QuizQuestionItem 题目卡片。
//
// 刻意不含 Correct 与 Analysis：列表页拿到答案就等于开卷考试，
// 前端也可能因为顺手渲染而把答案漏出去。这两项只在提交作答后返回。
type QuizQuestionItem struct {
	ID           int64            `json:"id"`
	Stem         string           `json:"stem"`
	Type         string           `json:"type"`
	Options      []QuizOptionItem `json:"options"`
	Difficulty   string           `json:"difficulty"`
	CategoryID   int64            `json:"categoryId"`
	CategoryName string           `json:"categoryName"`
	CategorySlug string           `json:"categorySlug"`
	Tags         []string         `json:"tags"`
	AnswerCount  int64            `json:"answerCount"`
	CorrectRate  float64          `json:"correctRate"`
	PublishedAt  *time.Time       `json:"publishedAt"`
}

// AdminQuizItem 后台列表项，多出答案、解析与状态。
type AdminQuizItem struct {
	QuizQuestionItem

	Correct   []string  `json:"correct"`
	Analysis  string    `json:"analysis"`
	Status    string    `json:"status"`
	AuthorID  int64     `json:"authorId"`
	CreatedAt time.Time `json:"createdAt"`
	UpdatedAt time.Time `json:"updatedAt"`

	// 审核痕迹。投稿题目与运营录题共表，靠 Status 区分，
	// 所以待审队列直接复用同一套列表字段——驳回原因必须能回传给投稿人，
	// 否则他只知道"被拒了"却不知道改哪里。
	ReviewNote string     `json:"reviewNote,omitempty"`
	ReviewerID int64      `json:"reviewerId,omitempty"`
	ReviewedAt *time.Time `json:"reviewedAt,omitempty"`
}

// ============================ 录入入参 ============================

// QuizQuestionCreateReq 新建题目。
type QuizQuestionCreateReq struct {
	Stem string `json:"stem" binding:"required"`
	// Type 省略按单选处理，这是运维题库里最主流的题型。
	Type string `json:"type" binding:"omitempty,oneof=single multiple judge"`
	// Options 判断题可以整个省略——选项固定是「正确 / 错误」，由服务端填，
	// 不让调用方传是为了避免同一个意思出现"对/错""是/否""True/False"三种写法。
	// 所以这里用 omitempty 而不是 required：写成 required 的话空切片会被校验层
	// 提前拒掉，service 里"判断题允许 0 个选项"那条分支就永远走不到了。
	// 单选与多选的个数下限由 service 兜底校验。
	Options []QuizOptionInput `json:"options" binding:"omitempty,min=2,max=8"`
	// Correct 是正确选项 key 的集合。多选传多个，单选与判断只传一个。
	Correct  []string `json:"correct" binding:"required,min=1,max=8"`
	Analysis string   `json:"analysis"`

	Difficulty string `json:"difficulty" binding:"omitempty,oneof=easy medium hard"`

	CategoryID int64    `json:"categoryId"`
	Tags       []string `json:"tags"`

	Status string `json:"status" binding:"omitempty,oneof=draft published"`
}

// QuizOptionInput 录入时的选项。
type QuizOptionInput struct {
	Key  string `json:"key" binding:"required"`
	Text string `json:"text" binding:"required"`
}

// ============================ 批量导入 ============================

// quizImportMaxRows 单次导入的行数上限。
//
// 上限不是为了性能（200 条一次插完是毫秒级），而是为了让"整体失败"这件事
// 仍然可用：行数一多，逐行校验出来的错误清单会长到没人看得完，
// 用户只能整批重来。真要导上千题，分批导比一次导更好排错。
const quizImportMaxRows = 200

// QuizImportMaxRows 暴露给 handler 做提示文案。
func QuizImportMaxRows() int { return quizImportMaxRows }

// QuizQuestionImportReq 批量导入题目的入参。
//
// 收的是 JSON 数组而不是 Excel：xlsx 解析要额外引一个几十 KB 的库，
// 而前端本来就能读文件、转成 JSON 再提交。转换放在浏览器做，
// 后端只处理一种格式，错误行号也与用户看到的表格行一一对应。
type QuizQuestionImportReq struct {
	Items []QuizQuestionCreateReq `json:"items" binding:"required,min=1,max=200"`
}

// QuizImportError 一行导入失败的原因。
type QuizImportError struct {
	// Row 从 1 开始，对应 items 的下标 +1，也对应表格里去掉表头后的行号。
	Row int `json:"row"`
	// Stem 是题干前缀，便于在几十条错误里一眼认出是哪一题。
	Stem    string `json:"stem"`
	Message string `json:"message"`
}

// QuizImportResult 导入结果。
//
// 有意做成 200 + 结果体，而不是校验失败返回 400：
// 错误清单本身就是要给用户逐行看的数据，塞进 errcode 的 message 里
// 只能带出第一条，用户改一条传一条，几十道题要来回十几趟。
// 判定规则由 Imported 与 Errors 组合表达：
//   - Errors 为空        → 全部成功，Imported == Total
//   - Errors 非空        → 一条都没落库（Imported 恒为 0），照单改完再传
type QuizImportResult struct {
	Total    int               `json:"total"`
	Imported int               `json:"imported"`
	Errors   []QuizImportError `json:"errors"`
}

// ============================ 题目投稿 ============================

// QuizContributeReq 用户投稿一道题。
//
// 与后台录入的关键差别：
//   - 没有 Status 字段：投稿固定 pending，让客户端传状态等于把发布权限交给浏览器；
//   - Analysis 必填：没有解析的题入库只会污染刷题体验，用户照着错题本复习
//     却看不到"为什么错"，那道题等于白做；
//   - 不收 AuthorID，作者固定取登录态。
type QuizContributeReq struct {
	Stem       string            `json:"stem" binding:"required"`
	Type       string            `json:"type" binding:"omitempty,oneof=single multiple judge"`
	Options    []QuizOptionInput `json:"options" binding:"omitempty,min=2,max=8"`
	Correct    []string          `json:"correct" binding:"required,min=1,max=8"`
	Analysis   string            `json:"analysis" binding:"required"`
	Difficulty string            `json:"difficulty" binding:"omitempty,oneof=easy medium hard"`
	CategoryID int64             `json:"categoryId"`
	Tags       []string          `json:"tags"`
}

// QuizContributeResp 投稿结果。
type QuizContributeResp struct {
	ID int64 `json:"id"`
	// Status 用于前端提示"已提交待审核"还是"已直接发布"（专家 / 管理员）。
	Status string `json:"status"`
}

// QuizReviewReq 后台审核一题。
//
// Action 只有 pass / reject 两个值，不做"打回草稿"：题目投稿是一次性动作，
// 投稿人没有草稿箱（与文章投稿口径一致）。
type QuizReviewReq struct {
	Action string `json:"action" binding:"required,oneof=pass reject"`
	Note   string `json:"note"`
}

// QuizQuestionUpdateReq 更新题目，指针字段实现"未传即不改"。
type QuizQuestionUpdateReq struct {
	Stem       *string            `json:"stem"`
	Type       *string            `json:"type"`
	Options    *[]QuizOptionInput `json:"options"`
	Correct    *[]string          `json:"correct"`
	Analysis   *string            `json:"analysis"`
	Difficulty *string            `json:"difficulty"`
	CategoryID *int64             `json:"categoryId"`
	Tags       *[]string          `json:"tags"`
}

// ============================ 答题 ============================

// QuizAnswerReq 单题作答。
type QuizAnswerReq struct {
	// Chosen 是用户选择的选项 key。多选题传多个，最少一个。
	Chosen []string `json:"chosen" binding:"required,min=1,max=8"`
}

// QuizAnswerResult 单题判分结果。
//
// 无论答对答错都返回 Correct 与 Analysis —— 题库的核心价值是让人学会，
// 答错时不给解析等于只告诉人"你错了"。
type QuizAnswerResult struct {
	QuestionID     int64    `json:"questionId"`
	Chosen         []string `json:"chosen"`
	Correct        bool     `json:"correct"`
	CorrectOptions []string `json:"correctOptions"`
	Analysis       string   `json:"analysis"`
	// Fresh 标记这道题是否为首次作答；重复作答不重复累加统计数据。
	Fresh bool `json:"fresh"`
}

// QuizSubmitItem 一次提交中的单题作答。
type QuizSubmitItem struct {
	QuestionID int64    `json:"questionId" binding:"required"`
	Chosen     []string `json:"chosen" binding:"required,min=1,max=8"`
}

// QuizSubmitReq 整卷提交。
type QuizSubmitReq struct {
	Items []QuizSubmitItem `json:"items" binding:"required,min=1,max=50"`
	// DurationSec 为 0 表示前端未计时，后端不做校验。
	DurationSec int64 `json:"durationSec" binding:"gte=0"`
	// CategoryID 非 0 表示这轮是按分类刷题，用于分项统计。
	CategoryID int64 `json:"categoryId"`
}

// QuizSubmitResult 整卷成绩单。
type QuizSubmitResult struct {
	AttemptID     int64              `json:"attemptId"`
	QuestionCount int64              `json:"questionCount"`
	CorrectCount  int64              `json:"correctCount"`
	Score         int                `json:"score"`
	DurationSec   int64              `json:"durationSec"`
	Items         []QuizAnswerResult `json:"items"`
}

// QuizAttemptItem 答题历史项。
type QuizAttemptItem struct {
	ID            int64     `json:"id"`
	QuestionCount int64     `json:"questionCount"`
	CorrectCount  int64     `json:"correctCount"`
	Score         int       `json:"score"`
	DurationSec   int64     `json:"durationSec"`
	CategoryID    int64     `json:"categoryId"`
	CreatedAt     time.Time `json:"createdAt"`
}

// QuizStats 个人答题总览，用于题库首页的"我的战绩"。
type QuizStats struct {
	/* TotalAnswered 去重后的作答题数，不是作答次数。 */
	TotalAnswered int64   `json:"totalAnswered"`
	TotalCorrect  int64   `json:"totalCorrect"`
	Accuracy      float64 `json:"accuracy"`
	WrongCount    int64   `json:"wrongCount"`
	AttemptCount  int64   `json:"attemptCount"`
}

// QuizWrongItem 错题本条目。
//
// 带上选项与解析：错题本的作用是复习，点进去还要再跳一次详情就太重了。
type QuizWrongItem struct {
	QuestionID     int64            `json:"questionId"`
	Stem           string           `json:"stem"`
	Type           string           `json:"type"`
	Difficulty     string           `json:"difficulty"`
	Options        []QuizOptionItem `json:"options"`
	Chosen         []string         `json:"chosen"`
	CorrectOptions []string         `json:"correctOptions"`
	Analysis       string           `json:"analysis"`
	CategoryID     int64            `json:"categoryId"`
	CategoryName   string           `json:"categoryName"`
	LastWrongAt    time.Time        `json:"lastWrongAt"`
}

// ============================ 约束 ============================

const (
	quizStemMaxLen     = 2000
	quizAnalysisMaxLen = 2000
	maxQuizOptions     = 8
	minQuizOptions     = 2
	// maxPracticeCount 单次练习抽题上限。
	maxPracticeCount = 50
)

// QuizLimits 返回题干与解析长度上限。
func QuizLimits() (stem, analysis int) { return quizStemMaxLen, quizAnalysisMaxLen }

// QuizOptionLimits 返回选项个数上下限。
func QuizOptionLimits() (min, max int) { return minQuizOptions, maxQuizOptions }

// QuizAnswerLimits 返回单次提交的题目数上限。
func QuizAnswerLimits() int { return maxPracticeCount }
