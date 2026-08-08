// shape.go —— 落库前的「行整形」：把领星返回的行摆成通用 Upsert 能吃的形状。
//
// 通用 Upsert 的前提是「每个字段都在顶层，列名 = 字段名」。领星少数接口不满足：
//   - 产品表现 /bd/productPerformance/openApi/asinList：唯一键 asin 埋在 asins[0].asin，
//     顶层 138 个字段全是指标；店铺号 sid 压根不返回（它是请求参数）。
//
// 两个机制都由 config 驱动（Endpoint.FieldPaths / Endpoint.InjectParams），
// 不针对任何单一接口写死逻辑——守 CLAUDE.md §1.3「加接口零代码改动」。
//
// 共同语义：**只填空，不覆盖**。目标列已有非空值时一律跳过，以领星实际返回为准。
// 这样领星哪天把字段提到顶层、或开始回显请求参数，行为自动跟随，配置不必回滚。
//
// 两类失败严格分开（这是本文件最要紧的设计）：
//   - 路径**写法**非法（配置 typo，如 asins[abc]）→ 返回 error，fail-loud。
//     静默空转的后果是「任务显示成功、库里没数据」，最难查。
//   - 路径合法但**取不到值**（某行 asins 是空数组）→ 不报错，跳过该列。
//     这类缺失是数据的常态；若该列是 NOT NULL 主键，DB 会用 1048 兜住，
//     不需要 shape 层重复判断「哪些列是身份列」。
package worker

import (
	"fmt"
	"strconv"
	"strings"
)

// shapeRows 就地整形 rows。两步都只在目标列「缺失或为空」时才写入。
//
//	fieldPaths   目标列名 → 取值路径（如 {"asin": "asins[0].asin"}）
//	injectParams 要回填进每行的请求参数名（如 ["sid"]）
//	params       本页实际发出的请求参数（injectParams 的取值来源）
//
// rows 为空、两个机制都没配时是纯空转，对绝大多数接口零开销。
// 返回 error 仅代表 fieldPaths 里有非法路径写法（配置错误）。
func shapeRows(rows []map[string]any, fieldPaths map[string]string, injectParams []string, params map[string]any) error {
	if len(fieldPaths) == 0 && len(injectParams) == 0 {
		return nil
	}

	// 先把所有路径解析好（每页只解析一次，不在行循环里重复解析）。
	// 任一路径写法非法立即返回——这是配置错误，早失败比跑完一轮没数据好查。
	parsed := make(map[string][]pathSeg, len(fieldPaths))
	for target, path := range fieldPaths {
		segs, err := parsePath(path)
		if err != nil {
			return fmt.Errorf("field_paths[%s] = %q 路径非法: %w", target, path, err)
		}
		parsed[target] = segs
	}

	if len(rows) == 0 {
		return nil // 路径已校验过，空页无事可做
	}

	// resolved 统计每个路径在本页「成功取到值」的行数，用于下面的整页判定。
	resolved := make(map[string]int, len(parsed))

	for _, row := range rows {
		if row == nil {
			continue
		}
		// 1. 嵌套字段提升到顶层。单行取不到不报错（数据稀疏是常态，见文件头说明）。
		for target, segs := range parsed {
			if !isBlank(row[target]) {
				resolved[target]++ // 顶层本来就有真值，以领星为准，也算「这个字段有解」
				continue
			}
			if v, ok := resolvePath(row, segs); ok && !isBlank(v) {
				row[target] = v
				resolved[target]++
			}
		}
		// 2. 请求参数回填（领星不回显的请求参数，如迭代的 sid）。
		for _, name := range injectParams {
			if name == "" || !isBlank(row[name]) {
				continue
			}
			if v, ok := params[name]; ok && !isBlank(v) {
				row[name] = v
			}
		}
	}

	// 整页零命中判定：某个配好的路径在整页里一行都没取到值，几乎只有一种解释——
	// 路径写错了（键名 typo、层级看错）。这种错语法检查抓不到：asinz[0].asin
	// 语法完全合法，只是键名不存在。
	//
	// 不做这个判定的后果，就是本次会话查了半天的那类 bug：整页静默写 NULL，
	// 一路撞到 "Column 'asin' cannot be null"，错误信息离病根隔着好几层。
	// 单行取不到仍然放过（稀疏数据正常），只有「整页全军覆没」才算配置错。
	for target := range parsed {
		if resolved[target] == 0 {
			return fmt.Errorf(
				"field_paths[%s] = %q 在本页 %d 行里一行都没取到值，几乎肯定是路径写错（键名/层级）；"+
					"若确认路径无误而领星本页确实不返回该字段，请从 field_paths 里去掉它",
				target, fieldPaths[target], len(rows))
		}
	}
	return nil
}

// ValidateFieldPaths 供 config 校验期调用：只查路径写法，不碰数据。
// 让配置 typo 在启动时就暴露，而不是等到第一次同步。
func ValidateFieldPaths(fieldPaths map[string]string) error {
	for target, path := range fieldPaths {
		if _, err := parsePath(path); err != nil {
			return fmt.Errorf("field_paths[%s] = %q 路径非法: %w", target, path, err)
		}
	}
	return nil
}

// resolvePath 按已解析的路径段从 row 里取值。
// 任何一步走不通（键不存在 / 不是对象或数组 / 下标越界）都返回 ok=false，不是错误。
func resolvePath(row map[string]any, segs []pathSeg) (any, bool) {
	if len(segs) == 0 {
		return nil, false
	}
	var cur any = row
	for _, s := range segs {
		if s.isIndex {
			arr, ok := cur.([]any)
			if !ok || s.index < 0 || s.index >= len(arr) {
				return nil, false
			}
			cur = arr[s.index]
			continue
		}
		m, ok := cur.(map[string]any)
		if !ok {
			return nil, false
		}
		cur, ok = m[s.key]
		if !ok {
			return nil, false
		}
	}
	return cur, true
}

// pathSeg 是路径的一段：要么是对象键，要么是数组下标。
type pathSeg struct {
	key     string
	index   int
	isIndex bool
}

// parsePath 把 "asins[0].asin" 解析成 [key:asins][idx:0][key:asin]。
// 语法：点号进对象，[n] 进数组，支持连续下标 a[0][1]。
// 写法非法时返回 error（调用方据此 fail-loud），不返回空切片装作没事。
func parsePath(path string) ([]pathSeg, error) {
	path = strings.TrimSpace(path)
	if path == "" {
		return nil, fmt.Errorf("路径为空")
	}
	var segs []pathSeg
	for _, part := range strings.Split(path, ".") {
		if part == "" {
			return nil, fmt.Errorf("有空路径段（多余的点号？）")
		}
		// 一段里可能带多个下标：a[0][1]
		for {
			open := strings.IndexByte(part, '[')
			if open < 0 {
				break
			}
			closeIdx := strings.IndexByte(part[open:], ']')
			if closeIdx < 0 {
				return nil, fmt.Errorf("方括号未闭合")
			}
			closeIdx += open
			if open > 0 {
				segs = append(segs, pathSeg{key: part[:open]})
			}
			raw := strings.TrimSpace(part[open+1 : closeIdx])
			idx, err := strconv.Atoi(raw)
			if err != nil {
				return nil, fmt.Errorf("下标 %q 不是整数", raw)
			}
			if idx < 0 {
				return nil, fmt.Errorf("下标 %d 为负", idx)
			}
			segs = append(segs, pathSeg{index: idx, isIndex: true})
			part = part[closeIdx+1:]
			if part == "" {
				break
			}
		}
		if part != "" {
			segs = append(segs, pathSeg{key: part})
		}
	}
	if len(segs) == 0 {
		return nil, fmt.Errorf("解析后无有效路径段")
	}
	return segs, nil
}

// isBlank 判定「该列还没有真值」：nil、空串、纯空白都算空。
// 数字 0 / false 不算空——它们是有意义的业务值，不该被路径值或请求参数顶掉。
func isBlank(v any) bool {
	if v == nil {
		return true
	}
	s, ok := v.(string)
	return ok && strings.TrimSpace(s) == ""
}
