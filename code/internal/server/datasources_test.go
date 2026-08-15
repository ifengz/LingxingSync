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

func TestNewPageDataDoesNotExposeDatabaseConfig(t *testing.T) {
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
	blob := fmt.Sprintf("%+v", pd)
	for _, forbidden := range []string{"10.0.0.5", "3307", "lingsync_rw", "super-secret-pw", "lingsync"} {
		if strings.Contains(blob, forbidden) {
			t.Fatalf("pageData 不应包含数据库配置 %q: %s", forbidden, blob)
		}
	}
}

func TestDataSourcesTemplateOmitsDatabaseConnectionCard(t *testing.T) {
	raw, err := os.ReadFile("../../web/templates/datasources.html")
	if err != nil {
		t.Fatalf("读 datasources.html: %v", err)
	}
	tpl, err := template.New("datasources").Funcs(sharedFuncs()).Parse(string(raw))
	if err != nil {
		t.Fatalf("parse datasources.html: %v", err)
	}

	var buf bytes.Buffer
	if err := tpl.ExecuteTemplate(&buf, "content", pageData{}); err != nil {
		t.Fatalf("execute content: %v", err)
	}
	out := buf.String()
	for _, forbidden := range []string{"window.__DB__", "MySQL 连接信息", "connStr", "复制连接串"} {
		if strings.Contains(out, forbidden) {
			t.Fatalf("数据源页面不应输出未授权连接信息 %q:\n%s", forbidden, out)
		}
	}
}

func TestDataSourcesTemplateUsesSingleRootForExpandedEndpoint(t *testing.T) {
	raw, err := os.ReadFile("../../web/templates/datasources.html")
	if err != nil {
		t.Fatalf("读 datasources.html: %v", err)
	}
	source := string(raw)
	const start = `<template x-for="(e, idx) in endpoints" :key="e.name">`
	idx := strings.Index(source, start)
	if idx < 0 {
		t.Fatal("数据源列表缺少 endpoint x-for")
	}
	root := strings.TrimSpace(source[idx+len(start):])
	if !strings.HasPrefix(root, `<tbody`) {
		t.Fatal("endpoint x-for 必须以单个 tbody 作为根节点，才能稳定渲染主行和详情行")
	}
}

func TestDatasetFieldsTemplateKeepsBothColumnsVisibleWhenConfigurationIsEmpty(t *testing.T) {
	raw, err := os.ReadFile("../../web/templates/dataset_fields.html")
	if err != nil {
		t.Fatalf("读 dataset_fields.html: %v", err)
	}
	source := string(raw)
	for _, required := range []string{"数据表配置", "新增项目", "数据表 ID", "固定字段", "业务字段", "Token ID", "店铺范围", "h-[1000px]", "overflow-y-auto", "createDatasetProjectToken"} {
		if !strings.Contains(source, required) {
			t.Fatalf("数据集字段页缺少 %q", required)
		}
	}
	if strings.Contains(source, `x-show="!fieldsLoading && !fieldsError && fieldGroups.length>0" class="grid`) {
		t.Fatal("字段双栏不得因 fieldGroups 为空而整体隐藏")
	}
}

func TestDataSourcesTemplateDoesNotContainDatasetFieldEditor(t *testing.T) {
	raw, err := os.ReadFile("../../web/templates/datasources.html")
	if err != nil {
		t.Fatalf("读 datasources.html: %v", err)
	}
	if strings.Contains(string(raw), "API 数据集返回字段") {
		t.Fatal("数据源页不应继续嵌入数据集字段编辑器")
	}
}
