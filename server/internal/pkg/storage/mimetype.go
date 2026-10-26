// Package storage 提供文件存储抽象：本地磁盘（开发环境）与对象存储（生产环境）。
//
// 上传链路的三个安全前提，任何一个放松都会变成漏洞：
//  1. 文件类型只认文件头魔数，不信任客户端传来的 Content-Type 与扩展名；
//  2. 存储路径完全由服务端生成，绝不拼接用户提供的文件名（防路径穿越）；
//  3. 私有文件（资质证书、问题附件）不进入公开静态目录，只接受带过期时间的签名访问。
package storage

import "bytes"

// magicEntry 描述一条文件头匹配规则。
type magicEntry struct {
	mime   string
	prefix []byte
	offset int
}

var magicTable = []magicEntry{
	{"image/jpeg", []byte{0xFF, 0xD8, 0xFF}, 0},
	{"image/png", []byte{0x89, 'P', 'N', 'G', 0x0D, 0x0A, 0x1A, 0x0A}, 0},
	{"image/gif", []byte("GIF87a"), 0},
	{"image/gif", []byte("GIF89a"), 0},
	{"application/pdf", []byte("%PDF-"), 0},
	{"application/zip", []byte{0x50, 0x4B, 0x03, 0x04}, 0},
	{"application/zip", []byte{0x50, 0x4B, 0x05, 0x06}, 0},
	// 运维资源包绝大多数是 .tar.gz，其外层就是 gzip 流，先按 gzip 识别。
	{"application/gzip", []byte{0x1F, 0x8B}, 0},
	{"application/x-7z-compressed", []byte{'7', 'z', 0xBC, 0xAF, 0x27, 0x1C}, 0},
}

// DetectMIME 依据文件头判断真实类型，无法识别时返回空串。
func DetectMIME(head []byte) string {
	for _, m := range magicTable {
		if len(head) < m.offset+len(m.prefix) {
			continue
		}
		if bytes.Equal(head[m.offset:m.offset+len(m.prefix)], m.prefix) {
			return m.mime
		}
	}
	// WEBP 是 RIFF????WEBP 结构，中间 4 字节为文件长度，无法用固定前缀匹配。
	if len(head) >= 12 && bytes.Equal(head[0:4], []byte("RIFF")) && bytes.Equal(head[8:12], []byte("WEBP")) {
		return "image/webp"
	}
	return ""
}

// LooksLikeText 判断内容是否像纯文本。
// 日志类附件没有魔数，只能反向判断：不含控制字符（常见空白除外）即认为是文本。
func LooksLikeText(head []byte) bool {
	if len(head) == 0 {
		return false
	}
	for _, b := range head {
		if b == 0x00 {
			return false
		}
		if b < 0x09 || (b > 0x0D && b < 0x20) {
			return false
		}
	}
	return true
}

// ExtForMIME 返回该类型对应的规范扩展名。
// 服务端只按识别结果生成文件名，不采用客户端上传的文件名。
func ExtForMIME(mime string) string {
	switch mime {
	case "image/jpeg":
		return ".jpg"
	case "image/png":
		return ".png"
	case "image/gif":
		return ".gif"
	case "image/webp":
		return ".webp"
	case "application/pdf":
		return ".pdf"
	case "application/zip":
		return ".zip"
	case "application/gzip":
		// 用双扩展名而不是 .gz：运维拿到的资源包几乎都是 tar 归档，
		// 存成 .gz 会让人误以为解压后得到单文件。
		return ".tar.gz"
	case "application/x-7z-compressed":
		return ".7z"
	case "text/plain":
		return ".txt"
	default:
		return ".bin"
	}
}
