package storage

import "testing"

func TestPolicyForKey(t *testing.T) {
	tests := []struct {
		key      string
		wantDir  string
		wantOK   bool
		wantPriv bool
	}{
		// 同前缀的两个目录必须能分开：article/image 不能被 article/cover 抢先匹配。
		{"article/image/202610/123.png", "article/image", true, false},
		{"article/cover/202610/123.png", "article/cover", true, false},
		{"avatar/202610/123.png", "avatar", true, false},
		{"expert/cert/202610/123.png", "expert/cert", true, true},
		// 目录名只是前缀的一部分：article/images 不是 article/image 下的文件。
		{"article/images/202610/123.png", "", false, false},
		{"unknown/202610/123.png", "", false, false},
		{"", "", false, false},
	}

	for _, tc := range tests {
		p, ok := PolicyForKey(tc.key)
		if ok != tc.wantOK {
			t.Fatalf("PolicyForKey(%q) ok = %v, want %v", tc.key, ok, tc.wantOK)
		}
		if !ok {
			continue
		}
		if p.Dir != tc.wantDir {
			t.Errorf("PolicyForKey(%q) dir = %q, want %q", tc.key, p.Dir, tc.wantDir)
		}
		if p.Private != tc.wantPriv {
			t.Errorf("PolicyForKey(%q) private = %v, want %v", tc.key, p.Private, tc.wantPriv)
		}
	}
}
