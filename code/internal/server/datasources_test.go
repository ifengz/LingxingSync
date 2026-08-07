package server

import (
	"bytes"
	"fmt"
	"html/template"
	"os"
	"strings"
	"testing"

	"lingxing-sync/internal/config"
)

// TestNewPageDataInjectsDBWithoutPassword 证明 newPageData 把 cfg.Database 的
// host/port/user/db 填进 pageData（datasources 页面注入 window.__DB__ 的数据来源），
// 且 pageData 结构体不携带密码字段——密码在编译期就不可能下发到前端。
func TestNewPageDataInjectsDBWithoutPassword(t *testing.T) {
	s := &Server{cfg: &config.Config{
		Database: config.Database{
			Host:     "10.0.0.5",
			Port:     3307,
			User:     "lingsync_rw",
			Password: "super-secret-pw",
			DB:       "lingsync",
		},
	}}

	pd := s.newPageData("datasources")
	if pd.DBHost != "10.0.0.5" || pd.DBPort != 3307 || pd.DBUser != "lingsync_rw" || pd.DBName != "lingsync" {
		t.Fatalf("newPageData 未正确填充 DB 字段: %+v", pd)
	}

	// pageData 结构体不含密码字段：整体序列化后断言绝不出现密码明文。
	if blob := fmt.Sprintf("%+v", pd); strings.Contains(blob, "super-secret-pw") {
		t.Fatalf("pageData 泄露了数据库密码: %s", blob)
	}
}

// TestDataSourcesTemplateInjectsDBGlobal 渲染真实 datasources.html 的 content 块，
// 证明 window.__DB__ 被真实注入（host/port/user/db 可见），且密码绝不出现在输出里。
// 用真实模板文件而非 testdata 桩，确保这条断言对得上线上页面。
func TestDataSourcesTemplateInjectsDBGlobal(t *testing.T) {
	raw, err := os.ReadFile("../../web/templates/datasources.html")
	if err != nil {
		t.Fatalf("读 datasources.html: %v", err)
	}
	tpl, err := template.New("datasources").Funcs(sharedFuncs()).Parse(string(raw))
	if err != nil {
		t.Fatalf("parse datasources.html: %v", err)
	}

	var buf bytes.Buffer
	pd := pageData{DBHost: "10.0.0.5", DBPort: 3307, DBUser: "lingsync_rw", DBName: "lingsync"}
	if err := tpl.ExecuteTemplate(&buf, "content", pd); err != nil {
		t.Fatalf("execute content: %v", err)
	}

	out := buf.String()
	for _, want := range []string{"window.__DB__", "10.0.0.5", "3307", "lingsync_rw", "lingsync"} {
		if !strings.Contains(out, want) {
			t.Fatalf("datasources content 缺少 %q\n---\n%s", want, out)
		}
	}
	// 模板层面：密码从不进入 window.__DB__，输出里不应出现任何密码痕迹。
	if strings.Contains(out, "super-secret-pw") || strings.Contains(out, "password") {
		t.Fatalf("datasources 模板输出疑似泄露密码:\n%s", out)
	}
}
