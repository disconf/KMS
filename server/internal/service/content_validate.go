package service

import (
	"fmt"
	"net/url"
	"strings"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/model"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// 本文件收敛知识库各栏目共用的字段校验。
//
// 工具、题库、资源三类内容的校验规则形状一致（去空白、限字符数、去重），
// 差别只在阈值与错误码。写成一个通用函数比各写一份更不容易漏掉边界。

// validateText 去空白并按字符数（不是字节数）校验长度。
// 中文标题与英文标题的"长度"必须按字符算，否则一个 40 字的英文标题会通过、
// 一个 40 字的中文标题却被判超长。
func validateText(raw string, minRunes, maxRunes int, code *errcode.Error, label string) (string, error) {
	text := strings.TrimSpace(raw)
	n := len([]rune(text))
	if n < minRunes || n > maxRunes {
		return "", code.WithMsg(fmt.Sprintf("%s长度需为 %d-%d 个字符", label, minRunes, maxRunes))
	}
	return text, nil
}

// validateOptionalText 允许为空的可选文本字段，只校验上限。
func validateOptionalText(raw string, maxRunes int, code *errcode.Error, label string) (string, error) {
	text := strings.TrimSpace(raw)
	if len([]rune(text)) > maxRunes {
		return "", code.WithMsg(fmt.Sprintf("%s不超过 %d 个字符", label, maxRunes))
	}
	return text, nil
}

// validateContentText 正文类字段：允许为空（先存草稿），只校验上限。
// 保留尾部之外的所有空白，因为 Markdown 的换行本身就是内容。
func validateContentText(raw string, maxRunes int, code *errcode.Error, label string) (string, error) {
	text := strings.TrimRight(raw, " \t\r\n")
	if len([]rune(text)) > maxRunes {
		return "", code.WithMsg(fmt.Sprintf("%s不超过 %d 个字符", label, maxRunes))
	}
	return text, nil
}

// normalizeURL 校验并规范化链接。
//
// 只接受 http/https：允许 javascript: 或 data: 协议等于给后台开了一个
// 存储型 XSS 的口子——运营录入的链接会被渲染到前台详情页上。
func normalizeURL(raw string, maxLen int, label string) (string, error) {
	link := strings.TrimSpace(raw)
	if link == "" {
		return "", nil
	}
	if len(link) > maxLen {
		return "", errcode.ErrToolURLInvalid.WithMsg(label + "过长")
	}
	u, err := url.Parse(link)
	if err != nil {
		return "", errcode.ErrToolURLInvalid.WithMsg(label + "格式不正确")
	}
	if u.Scheme != "http" && u.Scheme != "https" {
		return "", errcode.ErrToolURLInvalid.WithMsg(label + "必须以 http:// 或 https:// 开头")
	}
	if strings.TrimSpace(u.Host) == "" {
		return "", errcode.ErrToolURLInvalid.WithMsg(label + "缺少域名")
	}
	return u.String(), nil
}

// normalizeTagsLimit 去空、去重（忽略大小写）、限个数与单条长度。
//
// 去重忽略大小写：K8s 与 k8s 是同一个标签，不去重会让筛选器里出现两条
// 看起来一样、点下去结果却不同的选项。
func normalizeTagsLimit(raw []string, maxCount, maxLen int) (model.StringList, error) {
	seen := make(map[string]struct{}, len(raw))
	out := make(model.StringList, 0, len(raw))

	for _, t := range raw {
		t = strings.TrimSpace(t)
		if t == "" {
			continue
		}
		if n := len([]rune(t)); n > maxLen {
			return nil, errcode.ErrTagInvalid.WithMsg(fmt.Sprintf("单个标签不超过 %d 个字符", maxLen))
		}
		key := strings.ToLower(t)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		out = append(out, t)
	}
	if len(out) > maxCount {
		return nil, errcode.ErrTagInvalid.WithMsg(fmt.Sprintf("标签最多 %d 个", maxCount))
	}
	return out, nil
}

// normalizeContentTags 校验知识库各栏目的标签。
func normalizeContentTags(raw []string) (model.StringList, error) {
	maxCount, maxLen := dto.ContentTagLimits()
	return normalizeTagsLimit(raw, maxCount, maxLen)
}

// normalizeDeployModes 校验工具的部署方式取值。
//
// 与标签的关键差别是这里做白名单校验：部署方式会被前端用来渲染徽标与做筛选，
// 取值一旦发散（"k8s" 和 "kubernetes" 并存），筛选功能就废了。
func normalizeDeployModes(raw []string) (model.StringList, error) {
	allowed := make(map[string]struct{})
	for _, m := range dto.ToolDeployModes() {
		allowed[m] = struct{}{}
	}

	seen := make(map[string]struct{}, len(raw))
	out := make(model.StringList, 0, len(raw))
	for _, m := range raw {
		m = strings.ToLower(strings.TrimSpace(m))
		if m == "" {
			continue
		}
		if _, ok := allowed[m]; !ok {
			// 把可选值一并列出来。只说"不支持的部署方式：k8s"最容易被误解为
			// k8s 不在支持范围内，而正确答案是 k8s-operator——运营得猜三次才填对。
			return nil, errcode.ErrToolDeployMode.WithMsg(
				"不支持的部署方式：" + m + "，可选：" + strings.Join(dto.ToolDeployModes(), " / "))
		}
		if _, dup := seen[m]; dup {
			continue
		}
		seen[m] = struct{}{}
		out = append(out, m)
	}
	if len(out) > dto.DeployModeLimit() {
		return nil, errcode.ErrToolDeployMode.WithMsg(
			fmt.Sprintf("部署方式最多 %d 个", dto.DeployModeLimit()))
	}
	return out, nil
}

// isNumericKey 判断详情接口的 key 是不是纯数字（用于区分 ID 与 slug）。
func isNumericKey(key string) bool {
	if key == "" {
		return false
	}
	for _, r := range key {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
