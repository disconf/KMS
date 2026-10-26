package bootstrap

import (
	"context"
	"errors"
	"fmt"
	"strconv"
	"time"

	"gorm.io/gorm"

	"github.com/opsexpert/platform/internal/model"
)

// 示例问答。
//
// 为什么问答也要种子数据：问答详情页的采纳标记、徽章、点赞入口、回答表单
// 全都只在「这条问题已经有回答」时才渲染。库里没有一条带回答的问题，
// 前端自检就只能靠「碰巧之前有人手工问过」——那不是检查，是抽签。
// 空库启动的开发者点开问答广场，看到的也会是一片空白，误判成功能没做。
//
// 数据要求自洽：`Score` 不直接写一个数字，而是真的插点赞行再累加——
// 写死「7 人点赞」却查不到谁点的，等于给后台留了一个查不清的账。

// qaAskUserSeed 示例提问人。
//
// 不拿管理员或专家当提问人：问答广场第一屏全是「平台管理员 提问」，
// 看着像系统公告而不像社区。
type qaAskUserSeed struct {
	Phone    string
	Nickname string
}

var qaAskUserSeeds = []qaAskUserSeed{
	{"13800000011", "小李运维"},
	{"13800000012", "阿杰"},
}

// qaSeedAnswer 一条示例回答。
type qaSeedAnswer struct {
	Phone    string // 回答人手机号，必须是本文件之前已建好的账号
	Content  string
	Accepted bool
	// LikedBy 点赞人手机号。逐个插行而不是直接写 Score，
	// 保证「点赞数」与「谁点了赞」两处永远对得上。
	LikedBy []string
}

// qaSeedQuestion 一条示例问题。
type qaSeedQuestion struct {
	Title       string
	Description string
	AskPhone    string
	SkillName   string
	Answers     []qaSeedAnswer
}

var qaSeedQuestions = []qaSeedQuestion{
	{
		Title: "K8s Pod 一直 CrashLoopBackOff，节点内存和 CPU 都很正常，该从哪查？",
		Description: "现象：一个 Java 服务滚动更新后新 Pod 起不来，`kubectl get pod` 显示 " +
			"CrashLoopBackOff，重启次数已经 40 多次。\n\n" +
			"已经确认过的：\n" +
			"- 节点内存、CPU、磁盘都正常，同节点其他 Pod 没事\n" +
			"- 镜像能正常拉下来\n" +
			"- `kubectl logs` 只有一行 `Started Application`，之后就没输出了\n\n" +
			"感觉不太像 OOMKilled，因为 `describe` 里 lastState 的 reason 是 Error 而不是 OOMKilled。" +
			"这种「日志正常但反复重启」的情况一般往哪里看？",
		AskPhone:  "13800000011",
		SkillName: "Kubernetes",
		Answers: []qaSeedAnswer{
			{
				Phone: "13900000001",
				Content: "先别急着看应用日志，`reason=Error` 说明是进程自己退出的，容器没被内核杀。\n\n" +
					"按这个顺序查：\n\n" +
					"1. **看退出码**：`kubectl describe pod` 里 lastState.terminated.exitCode。" +
					"137 是被 SIGKILL（可能是 OOM 也可能是探针超时后 kubelet 杀的），1 是应用自己报错退出，0 是正常结束。" +
					"你这种「打完 Started 就退出」最常见的是 exitCode=0 或 1。\n\n" +
					"2. **如果是 exitCode=0**：应用把「启动完」当成了「干完活」，主线程退出、进程结束。" +
					"Spring Boot 里常见于没引入 `spring-boot-starter-web`（非 Web 应用启动完就退出），" +
					"或者用了 `ApplicationRunner` 跑完任务没有阻塞主线程。\n\n" +
					"3. **看 liveness 探针**：`initialDelaySeconds` 小于应用真实启动时间时，" +
					"探针连续失败会让 kubelet 重启容器，而应用此时日志一切正常——现象和你描述的一模一样。" +
					"临时把 liveness 摘掉再跑一次，能起说明就是它。\n\n" +
					"4. **`previous` 日志**：`kubectl logs -p` 看上一个容器的日志，别只看当前这轮的。",
				Accepted: true,
				LikedBy:  []string{"13800000011", "13800000012", "13900000002"},
			},
			{
				Phone: "13900000003",
				Content: "补一个容易漏的点：**应用日志只写到 stdout 的一行就没了，不代表进程没打别的**。\n\n" +
					"如果容器里配置了 `terminationGracePeriodSeconds` 很小（比如 5 秒），" +
					"并且应用收到 SIGTERM 后还会继续往下走一段，那这段时间里的日志会随容器退出一起丢掉，" +
					"看起来就像「打完 Started 就没输出」。把宽限期调到 30 秒再复现一次，往往能看到真正的报错。\n\n" +
					"另外 `kubectl get events --sort-by=.lastTimestamp` 里能看到 Back-off 的原始时间线，" +
					"比 describe 里的当前状态更能看出重启节奏是「秒级」还是「分钟级」。",
				LikedBy: []string{"13800000012"},
			},
		},
	},
	{
		Title: "MySQL 主从延迟突然涨到 30 秒，但从库 CPU、IO 都不高，可能是什么原因？",
		Description: "一主两从，MySQL 5.7 半同步复制。昨天开始从库 2 的 `Seconds_Behind_Master` " +
			"一路涨到 30 秒左右并稳定在那，从库 1 正常。\n\n" +
			"从库 2 的 CPU 不到 20%，磁盘 iostat 的 %util 也不高，" +
			"`SHOW SLAVE STATUS` 里 SQL 线程和 IO 线程都是 Yes，没有报错。\n\n" +
			"主库那边写流量没有明显增长。这种「延迟涨了但资源不忙」的情况一般是什么问题？",
		AskPhone:  "13800000012",
		SkillName: "MySQL",
		Answers: []qaSeedAnswer{
			{
				Phone: "13900000002",
				Content: "资源不忙但延迟稳定在某个值，通常是**单线程回放的排队**，不是资源瓶颈。\n\n" +
					"先算一下是不是正好卡在大事务上：\n\n" +
					"```sql\n" +
					"-- 从库上执行，看正在回放的事件耗时\n" +
					"SHOW PROCESSLIST;\n" +
					"-- 主库上找长事务\n" +
					"SELECT trx_id, trx_started, trx_rows_modified\n" +
					"FROM information_schema.innodb_trx ORDER BY trx_started LIMIT 5;\n" +
					"```\n\n" +
					"5.7 的 SQL 线程默认单线程回放，一个大事务（比如批量 DELETE 十万行）在主库跑 20 秒，" +
					"从库就得原样重放 20 秒，期间后面所有事件全部排队——这正好解释「延迟涨到一个值后稳定不动」：" +
					"大事务回放完了延迟会自己掉回去。\n\n" +
					"几个可落地的处理：\n\n" +
					"1. **大事务拆小**：批量操作按 1000 行一批 + 每批 sleep 一点，从库回放就不会被单条堵住。\n" +
					"2. **开并行复制**：`slave_parallel_workers=8`、`slave_parallel_type=LOGICAL_CLOCK`，" +
					"要求主库 `binlog_group_commit_sync_delay` 能让同一组提交的事务被并行回放。\n" +
					"3. **别只盯 Seconds_Behind_Master**：这个值在 SQL 线程空闲时会归零误导人，" +
					"用 `SHOW SLAVE STATUS` 的 `Relay_Log_Pos` 和主库 `SHOW MASTER STATUS` 的 `Position` 差值换算更准。",
				LikedBy: []string{"13800000012", "13900000001"},
			},
		},
	},
	{
		Title: "Nginx 反代偶发 502，日志是 upstream prematurely closed connection，从哪查起？",
		Description: "Nginx 1.20 反代一组 Java 后端（8 个实例，权重相同），QPS 大概 300，" +
			"每天会出现十几次 502，时间点没有规律。\n\n" +
			"error.log 里的报错是：\n" +
			"`upstream prematurely closed connection while reading response header from upstream`\n\n" +
			"后端应用日志里对应时间点没有异常，GC 日志也看不出长暂停。" +
			"`proxy_read_timeout` 已经调到 60 秒，仍然出现。是否和 keepalive 有关？",
		AskPhone:  "13800000011",
		SkillName: "Nginx",
		// 故意留一条零回答的问题：列表的「待解答」标签页与零回答计数
		// 需要有真数据可看，否则那个 tab 点进去永远是空的。
		Answers: nil,
	},
}

// seedQa 写入示例问答，幂等（按标题判重）。
func seedQa(ctx context.Context, db *gorm.DB) error {
	ids := make(map[string]int64, len(qaAskUserSeeds)+len(expertSeeds))

	for _, u := range qaAskUserSeeds {
		id, err := ensureSeedUser(ctx, db, u.Phone, u.Nickname)
		if err != nil {
			return err
		}
		ids[u.Phone] = id
	}
	// 回答人都是种子专家，账号在 seedExperts 里已经建好，这里只取 ID。
	for _, e := range expertSeeds {
		var user model.User
		err := db.WithContext(ctx).Where("phone = ?", e.Phone).First(&user).Error
		if err != nil {
			if errors.Is(err, gorm.ErrRecordNotFound) {
				continue // 专家没建起来（比如被手工删了），回答时按缺失跳过
			}
			return fmt.Errorf("查询示例专家账号 %s 失败: %w", e.Phone, err)
		}
		ids[e.Phone] = user.ID
	}

	for _, q := range qaSeedQuestions {
		if err := seedOneQaQuestion(ctx, db, q, ids); err != nil {
			return err
		}
	}
	return nil
}

// ensureSeedUser 保证一个普通用户账号存在，返回其 ID。
func ensureSeedUser(ctx context.Context, db *gorm.DB, phone, nickname string) (int64, error) {
	var user model.User
	err := db.WithContext(ctx).Where("phone = ?", phone).First(&user).Error
	if err == nil {
		return user.ID, nil
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return 0, fmt.Errorf("查询示例用户 %s 失败: %w", phone, err)
	}
	user = model.User{
		Phone:    model.StrPtr(phone),
		Nickname: nickname,
		Role:     model.RoleUser,
		Status:   model.UserStatusActive,
	}
	if err := db.WithContext(ctx).Create(&user).Error; err != nil {
		return 0, fmt.Errorf("创建示例用户 %s 失败: %w", phone, err)
	}
	return user.ID, nil
}

// seedOneQaQuestion 写入一条问题及其回答、点赞与积分。
func seedOneQaQuestion(ctx context.Context, db *gorm.DB, s qaSeedQuestion, users map[string]int64) error {
	var existing model.QaQuestion
	err := db.WithContext(ctx).Where("title = ?", s.Title).First(&existing).Error
	if err == nil {
		return nil // 播过种了
	}
	if !errors.Is(err, gorm.ErrRecordNotFound) {
		return fmt.Errorf("查询示例问答失败: %w", err)
	}

	askerID := users[s.AskPhone]
	if askerID == 0 {
		return nil // 提问人账号缺失，整条跳过比写一条作者为 0 的脏数据好
	}

	now := time.Now()
	q := model.QaQuestion{
		UserID:      askerID,
		Title:       s.Title,
		Description: s.Description,
		Status:      model.QaStatusOpen,
		// 让浏览数看起来像有人看过，但不改回答数与状态——
		// 那两个字段由下面的真实数据算出来。
		ViewCount: 120 + int64(len([]rune(s.Title))),
	}
	if s.SkillName != "" {
		var skill model.Skill
		if err := db.WithContext(ctx).Where("name = ?", s.SkillName).First(&skill).Error; err == nil {
			q.SkillIDs = model.StringList{strconv.FormatInt(skill.ID, 10)}
		}
	}
	if err := db.WithContext(ctx).Create(&q).Error; err != nil {
		return fmt.Errorf("创建示例问答失败: %w", err)
	}

	var (
		answerCount int64
		lastAt      *time.Time
		acceptedID  int64
		acceptedAt  *time.Time
	)

	for i, a := range s.Answers {
		authorID := users[a.Phone]
		if authorID == 0 || authorID == askerID {
			continue
		}
		// 时间错开：第一条比现在早两小时，后面每条晚 40 分钟，
		// 列表按 last_answer_at 排序时才看得出先后。
		at := now.Add(time.Duration(i-2) * 40 * time.Minute)

		ans := model.QaAnswer{
			QuestionID: q.ID,
			UserID:     authorID,
			Content:    a.Content,
			Status:     model.QaAnswerPublished,
			IsAccepted: a.Accepted,
		}
		ans.CreatedAt = at
		ans.UpdatedAt = at
		if err := db.WithContext(ctx).Create(&ans).Error; err != nil {
			return fmt.Errorf("创建示例回答失败: %w", err)
		}

		var score int64
		for _, phone := range a.LikedBy {
			likerID := users[phone]
			if likerID == 0 || likerID == authorID {
				continue // 自己给自己点赞不算
			}
			like := model.QaAnswerLike{AnswerID: ans.ID, UserID: likerID}
			like.CreatedAt = at
			if err := db.WithContext(ctx).Create(&like).Error; err != nil {
				continue // 唯一索引冲突说明已经播过，跳过即可
			}
			score++
			if err := seedAwardPoints(ctx, db, authorID, model.PointActionAnswerLiked, 1,
				"answer", ans.ID, fmt.Sprintf("qa.like:%d:%d", ans.ID, likerID)); err != nil {
				return err
			}
		}
		if score > 0 {
			if err := db.WithContext(ctx).Model(&model.QaAnswer{}).Where("id = ?", ans.ID).
				UpdateColumn("score", score).Error; err != nil {
				return fmt.Errorf("回填示例回答点赞数失败: %w", err)
			}
		}

		if a.Accepted {
			acceptedID = ans.ID
			acceptedAt = &at
			if err := seedAwardPoints(ctx, db, authorID, model.PointActionAcceptAnswer, 10,
				"answer", ans.ID, fmt.Sprintf("qa.accept:%d", ans.ID)); err != nil {
				return err
			}
		}
		answerCount++
		t := at
		lastAt = &t
	}

	updates := map[string]any{
		"answer_count":   answerCount,
		"last_answer_at": lastAt,
	}
	if acceptedID > 0 {
		updates["status"] = model.QaStatusResolved
		updates["accepted_answer_id"] = acceptedID
		updates["accepted_at"] = acceptedAt
	}
	if err := db.WithContext(ctx).Model(&model.QaQuestion{}).Where("id = ?", q.ID).
		Updates(updates).Error; err != nil {
		return fmt.Errorf("回填示例问答统计失败: %w", err)
	}
	return nil
}

// seedAwardPoints 写一条积分流水并累加冗余总分。
//
// uniqKey 与 service 层用同一套格式：播种过的库再走一次真实采纳/点赞时，
// 唯一索引会挡住重复累加，不会出现「演示数据让积分虚高」。
func seedAwardPoints(ctx context.Context, db *gorm.DB, userID int64, action string, points int64, targetType string, targetID int64, uniqKey string) error {
	var n int64
	if err := db.WithContext(ctx).Model(&model.PointLog{}).
		Where("uniq_key = ?", uniqKey).Count(&n).Error; err != nil {
		return fmt.Errorf("查询积分流水失败: %w", err)
	}
	if n > 0 {
		return nil
	}
	log := model.PointLog{
		UserID: userID, Action: action, Points: points,
		TargetType: targetType, TargetID: targetID, UniqKey: uniqKey,
	}
	if err := db.WithContext(ctx).Create(&log).Error; err != nil {
		return nil // 并发播种撞唯一索引，忽略
	}
	return db.WithContext(ctx).Model(&model.User{}).Where("id = ?", userID).
		UpdateColumn("contribution_points", gorm.Expr("contribution_points + ?", points)).Error
}
