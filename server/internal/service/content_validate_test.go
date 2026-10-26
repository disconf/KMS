package service

import (
	"strings"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	"github.com/opsexpert/platform/internal/dto"
	"github.com/opsexpert/platform/internal/pkg/errcode"
)

// 本文件只测纯函数：没有数据库、没有网络，跑得快，用来把边界条件钉死。
// 涉及落库的行为放在各模块自己的 _test.go 里。

func TestValidateTextCountsRunesNotBytes(t *testing.T) {
	// 40 个汉字是 120 字节。按字节算会把一个本该通过的标题判成超长。
	text := strings.Repeat("运", 40)
	out, err := validateText(text, 1, 40, errcode.ErrParamInvalid, "标题")
	require.NoError(t, err, "40 个汉字应正好等于上限")
	assert.Equal(t, text, out)

	_, err = validateText(strings.Repeat("运", 41), 1, 40, errcode.ErrParamInvalid, "标题")
	assert.ErrorIs(t, err, errcode.ErrParamInvalid)

	_, err = validateText("  ", 1, 40, errcode.ErrParamInvalid, "标题")
	assert.ErrorIs(t, err, errcode.ErrParamInvalid, "只有空白的内容应视为空")
}

func TestValidateContentTextKeepsInnerNewlines(t *testing.T) {
	// Markdown 的换行本身就是内容，只裁尾部空白。
	out, err := validateContentText("第一行\n\n第二行\n", 100, errcode.ErrContentInvalid, "正文")
	require.NoError(t, err)
	assert.Equal(t, "第一行\n\n第二行", out, "尾部换行被裁掉，中间的空行必须保留")

	_, err = validateContentText(strings.Repeat("a", 101), 100, errcode.ErrContentInvalid, "正文")
	assert.ErrorIs(t, err, errcode.ErrContentInvalid)
}

func TestNormalizeURLAcceptsOnlyHTTPAndHTTPS(t *testing.T) {
	cases := []struct {
		name string
		in   string
		ok   bool
	}{
		{"https 正常", "https://github.com/prometheus/prometheus", true},
		{"http 也允许", "http://example.com/a", true},
		{"空串合法（链接是可选字段）", "", true},
		{"javascript 协议挡掉", "javascript:alert(1)", false},
		{"data 协议挡掉", "data:text/html;base64,PHNjcmlwdD4=", false},
		{"file 协议挡掉", "file:///etc/passwd", false},
		{"缺少协议", "github.com/foo", false},
		{"缺少域名", "https://", false},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			out, err := normalizeURL(c.in, 512, "仓库地址")
			if c.ok {
				require.NoError(t, err)
				if c.in == "" {
					assert.Equal(t, "", out)
				} else {
					assert.NotEmpty(t, out)
				}
				return
			}
			assert.ErrorIs(t, err, errcode.ErrToolURLInvalid)
		})
	}
}

func TestNormalizeURLRejectsOverlongLink(t *testing.T) {
	_, err := normalizeURL("https://example.com/"+strings.Repeat("a", 600), 512, "官网地址")
	assert.ErrorIs(t, err, errcode.ErrToolURLInvalid)
}

func TestNormalizeTagsLimitDedupesCaseInsensitively(t *testing.T) {
	out, err := normalizeTagsLimit([]string{"K8s", "k8s", " K8S ", "MySQL", "", "  "}, 10, 32)
	require.NoError(t, err)
	// 三种写法是同一个标签，去重后只留第一个的原始大小写。
	assert.Equal(t, []string{"K8s", "MySQL"}, []string(out))
}

func TestNormalizeTagsLimitRejectsCountAndLength(t *testing.T) {
	many := make([]string, 0, 11)
	for i := 0; i < 11; i++ {
		many = append(many, string(rune('a'+i)))
	}
	_, err := normalizeTagsLimit(many, 10, 32)
	assert.ErrorIs(t, err, errcode.ErrTagInvalid, "超过个数上限应被拒")

	_, err = normalizeTagsLimit([]string{strings.Repeat("字", 33)}, 10, 32)
	assert.ErrorIs(t, err, errcode.ErrTagInvalid, "单个标签超长应被拒")
}

func TestNormalizeDeployModesIsWhitelisted(t *testing.T) {
	out, err := normalizeDeployModes([]string{"Docker", " docker ", "helm", "", "HELM"})
	require.NoError(t, err)
	// 转小写、去重后入库，保证同一取值在库里只有一种写法。
	assert.Equal(t, []string{"docker", "helm"}, []string(out))

	_, err = normalizeDeployModes([]string{"k8s"})
	assert.ErrorIs(t, err, errcode.ErrToolDeployMode,
		"k8s 不在白名单里（正确取值是 k8s-operator），必须拒绝而不是放行")
}

func TestNormalizeDeployModesCountLimitIsUnreachableByDesign(t *testing.T) {
	// 白名单只有 6 个取值，而个数上限是 8，所以"超个数"这条分支在当前
	// 取值集合下永远走不到。这里把它钉住：如果哪天有人往白名单里加值加到超过
	// 8 个，这个测试会提醒他顺带确认上限分支还需不需要。
	require.Equal(t, 6, len(dto.ToolDeployModes()))
	require.Equal(t, 8, dto.DeployModeLimit())
	assert.Less(t, len(dto.ToolDeployModes()), dto.DeployModeLimit())

	out, err := normalizeDeployModes([]string{"binary", "docker", "helm", "k8s-operator", "source", "cloud"})
	require.NoError(t, err)
	assert.Len(t, out, 6)
}

func TestIsNumericKey(t *testing.T) {
	assert.True(t, isNumericKey("123"))
	assert.False(t, isNumericKey(""))
	assert.False(t, isNumericKey("12a"))
	assert.False(t, isNumericKey("-1"))
	assert.False(t, isNumericKey("1.5"))
	assert.False(t, isNumericKey("k8s-pod-restart"))
}

func TestNormalizeOptionKeyKeepsSingleAlnumOnly(t *testing.T) {
	assert.Equal(t, "A", normalizeOptionKey(" a "))
	assert.Equal(t, "D", normalizeOptionKey("D"))
	assert.Equal(t, "1", normalizeOptionKey("1"))
	assert.Equal(t, "", normalizeOptionKey(""))
	assert.Equal(t, "", normalizeOptionKey("AB"), "多字符标识前端没法映射快捷键")
	assert.Equal(t, "", normalizeOptionKey("中"))
	assert.Equal(t, "", normalizeOptionKey("-"))
}

func TestSanitizeFileNameStripsPathAndControlChars(t *testing.T) {
	cases := []struct {
		in, want string
	}{
		{"deploy.sh", "deploy.sh"},
		{"  deploy.sh  ", "deploy.sh"},
		// 上传可能来自 Windows 客户端，两种分隔符都要处理。
		{`C:\Users\me\deploy.sh`, "deploy.sh"},
		{"/tmp/a/b/nginx.conf", "nginx.conf"},
		{"../../etc/passwd", "passwd"},
		{"a\x00b.sh", "ab.sh"},
		{"..", ""},
		{".", ""},
		{"/", ""},
		{"", ""},
	}
	for _, c := range cases {
		t.Run(c.in, func(t *testing.T) {
			assert.Equal(t, c.want, sanitizeFileName(c.in))
		})
	}
}

func TestSanitizeFileNameTruncatesToLimit(t *testing.T) {
	long := strings.Repeat("a", 300) + ".sh"
	out := sanitizeFileName(long)
	assert.Equal(t, fileNameMaxLen, len([]rune(out)))
}

func TestNormalizeResourceFileNamePrefersOriginalExtension(t *testing.T) {
	// 存储 key 里的扩展名是服务端按识别出的 MIME 生成的：.sh 会被存成 .txt。
	// 展示给用户的必须还是 .sh，否则筛选器里全是 txt。
	file := &UploadedFile{Key: "resource/202510/abc.txt", Ext: "txt"}
	name, ext := normalizeResourceFileName(`C:\tmp\deploy.sh`, file)
	assert.Equal(t, "deploy.sh", name)
	assert.Equal(t, "sh", ext)

	// 原始文件名为空时退回 key 的 basename 与识别出的类型。
	name, ext = normalizeResourceFileName("", file)
	assert.Equal(t, "abc.txt", name)
	assert.Equal(t, "txt", ext)
}

func TestNormalizeResourceFileNameFallsBackOnBadExtension(t *testing.T) {
	file := &UploadedFile{Key: "resource/202510/abc.txt", Ext: "txt"}
	_, ext := normalizeResourceFileName("archive.thisisaverylongextension", file)
	assert.Equal(t, "txt", ext, "扩展名异常长时应回退到服务端识别出的类型")
}
