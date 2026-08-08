package db

import (
	"reflect"
	"testing"
)

func TestNormalizeUpsertValueIsColumnAware(t *testing.T) {
	if got := normalizeUpsertValue("", true); got != nil {
		t.Fatalf("JSON 空字符串 = %#v, want nil", got)
	}
	if got := normalizeUpsertValue("", false); got != "" {
		t.Fatalf("普通列空字符串 = %#v, want empty string", got)
	}
	if got := normalizeUpsertValue(`{"key":"value"}`, true); got != `{"key":"value"}` {
		t.Fatalf("JSON 字符串被错误改写: %#v", got)
	}
	if got := normalizeUpsertValue(map[string]any{"key": "value"}, true); !reflect.DeepEqual(got, `{"key":"value"}`) {
		t.Fatalf("JSON 对象序列化结果 = %#v", got)
	}
}
