# UI 组件规范

> 本文件与 `05-ui.md` 同属 UI 宪法层。它只规定共享视觉基元和落地边界，不改变页面职责、API 合同或交互流程。

## 1. 目标

四个浏览器页面使用同一套按钮、表单、面板、表格、状态、反馈和浮层样式。统一视觉不得引入新的前端运行时，也不得把纯样式改造扩散到 Go handler、数据库或同步逻辑。

## 2. 技术边界

- 保留 Go `html/template`、Alpine.js CDN、Tailwind CSS CDN、内联 SVG 和 `//go:embed web/`。
- 共享样式放在 `web/static/ui.css`，由 `layout.html` 统一加载并随二进制内嵌。
- 禁止 React、Vue、Svelte、Webpack、npm、Node.js、CSS 构建器和常驻前端服务。
- 第一阶段不新增 Go sub-template 组件。当前动态属性与插槽差异较大，为复用而修改模板解析器会增加无必要复杂度。
- 视觉改造不得修改 `x-data`、`x-model`、`x-show`、事件表达式、API 路径、请求体和页面状态模型。

## 3. 视觉方向

这是日常高频使用的同步运维工作台，不是营销页。

- 信息密度：紧凑但可扫读，表格、筛选和操作优先。
- 背景与容器：页面 `#f8fafc`，工作面 `#ffffff`，边线 `#e2e8f0`。
- 主操作：蓝色 `#2563eb`；成功、警告、失败分别使用 emerald、amber、red。
- 圆角：控件 6px，面板最多 8px；禁止大圆角卡片和卡片套卡片。
- 阴影：仅用于 Toast、Dialog、Drawer 等浮层；普通页面区块以边线分组。
- 字体：系统无衬线字体；任务 ID、时间、耗时、记录数使用等宽或 tabular 数字。
- 动效：只保留加载旋转、抽屉进出和必要状态过渡；尊重 `prefers-reduced-motion`。

## 4. 共享组件

`ui.css` 只建立已经存在多个调用方的稳定组件：

| 组件 | 必备变体 |
|---|---|
| Button | primary、secondary、danger、ghost、icon、small |
| Field | label、input、select、checkbox、focus、disabled、error |
| Panel | panel、section header、toolbar |
| Tabs | tab、active、disabled |
| Badge | success、running、error、warning、disabled |
| Dense table | header、row、numeric cell、empty row、horizontal overflow |
| Feedback | loading、empty、inline error、Toast |
| Overlay | Confirm、Dialog、Drawer、backdrop |
| Pagination | previous、current、next、disabled |

组件类使用 `ui-` 前缀，避免与 Tailwind 工具类或 Alpine 状态类冲突。页面允许用少量 Tailwind 类处理该页面独有的布局，但不得复制组件的完整颜色、边框和交互状态。

## 5. 页面实施顺序

1. `/logs`：先验证筛选、密表、Badge、分页、行内操作和详情抽屉。
2. `/datasources`：复用面板、表格、刷新和展开状态。
3. `/settings/api`：复用表单、账号区、店铺表和确认操作。
4. `/sync`：最后处理选择器、Tabs、调度表和添加接口表单；该页行为最多，必须建立在前三页已稳定的组件上。

每次只迁移一页。上一页未完成自动化检查和浏览器复核，不进入下一页。

## 6. 每页验收

- 页面路由、API 请求、Alpine 方法和字段均未改变。
- 键盘焦点清晰；图标按钮有可访问名称；禁用态不可误触。
- 1440px 和 390px 视口无文本溢出、控件重叠或不可达操作。
- 密表在窄屏横向滚动，不压缩到文字互相遮挡。
- 加载、空、失败和成功状态均可见，不以空白代替错误。
- `node --check web/static/app.js`、相关 Node 测试、`go test ./...`、`go vet ./...` 通过。
- 浏览器截图与关键交互 smoke 通过；HTTP 200 只能证明页面可访问，不能替代视觉和行为验证。

## 7. 禁止事项

- 不复制外部 React/Radix 运行时，只能参考其视觉层级和交互语义。
- 不在样式改造中新增页面、字段、接口或业务操作。
- 不为单次样式创建抽象；只有两个及以上真实调用方才进入共享组件。
- 不把多个页面一次性重写后统一验证。
- 不用渐变、装饰光球、营销式大标题或低信息密度卡片墙。
