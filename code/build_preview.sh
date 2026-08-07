#!/usr/bin/env bash
# 一次性构建 /sync 静态预览页：把 app.js 内联、插入内容块与 toast/confirm。
# 只为本地预览，不进 git/embed。
set -euo pipefail
cd "$(dirname "$0")"

TPL=web/templates
OUT=web/static/preview_sync.html

# 抽取片段（写到 web/static/，沙箱可写）
sed -n '8,343p'  "$TPL/sync_manage.html" > web/static/_frag_content.html
sed -n '85,116p' "$TPL/layout.html"      > web/static/_frag_toastconfirm.html

# 用 awk 替换三个占位/引用点
awk '
  /<script src="\.\/app\.js"><\/script>/ {
    print "  <script>";
    while ((getline line < "web/static/app.js") > 0) print line;
    print "  </script>";
    next;
  }
  /<!-- CONTENT_PLACEHOLDER -->/ {
    while ((getline line < "web/static/_frag_content.html") > 0) print line;
    next;
  }
  /<!-- TOAST_CONFIRM_PLACEHOLDER -->/ {
    while ((getline line < "web/static/_frag_toastconfirm.html") > 0) print line;
    next;
  }
  { print }
' "$OUT" > web/static/preview_sync.built.html

mv web/static/preview_sync.built.html "$OUT"
rm -f web/static/_frag_content.html web/static/_frag_toastconfirm.html
echo "=== built: $OUT ==="
wc -l "$OUT"
grep -c 'window.syncManage' "$OUT" || true
