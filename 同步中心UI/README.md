# 同步中心前端

纯前端副本，来源：`/Users/ifengz/CodingCase/polabel2/code/apps/web/app/admin/sync`。
用途：把这套同步中心 UI 套接到一个全新的、以 Go 为基础的同步链路后端上。

## 保留（纯前端）

五个 tab、表格、筛选、分页、日期范围、展开收起、覆盖矩阵弹层、按钮与加载/错误状态、
刷新、SSE 前端客户端与共享 UI 控件。仅接 props、只做展示与交互，不含任何后端逻辑。

## 已移除（后端 / 同步链路）

- 同步执行、任务取消/重试、计划保存、运行参数保存、接口限流保存
- 数据库访问、worker health 探测、同步事件路由、队列 / admission / workflow
- `runtime-config.ts` 原有的 Node/MySQL 实现：读取 `process.env`、计算 MySQL pool /
  worker 并发 / 同步历史 purge 等，现只保留 UI 渲染用的 `SyncRuntimeConfigRow` 类型
- `sync-engine/sync-status.ts` 原有的 SQL 片段与 raw intent 状态辅助，现只保留
  UI 判定"进行中"用的 `isProjectedActiveSyncRunStatus`

这些后端逻辑一律不在本副本内实现，交由新的 Go 同步后端从零重写，避免旧 Node 实现污染新链路。

## 接入 Go 后端的两个 seam

1. `src/admin/sync/ui-action-adapter.ts` —— 唯一动作边界。当前所有会改变同步或配置状态的
   按钮都会明确报错，不请求也不触发任何同步。接入时由 Go 后端提供对应 API，在此实现真正的调用。
2. `src/admin/sync/sync-events-client.tsx` —— 实时快照的 SSE 客户端，监听 `/api/sync-events`。
   由 Go 后端实现该 SSE 端点即可驱动同步日志实时刷新；不实现也不影响手动刷新。

UI 仍依赖目标项目已有的 React、Next.js、Tailwind、Radix、lucide 和 sonner 运行环境；
`sync-center.css` 是当前同步中心专用样式。
