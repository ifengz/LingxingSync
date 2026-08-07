/**
 * 前端专用：运行时配置面板（数据源 tab）渲染所需的行结构。
 *
 * 原文件包含的后端/同步链路实现已全部移除：
 * - readSyncRuntimeConfigRows()：读取 process.env、计算 MySQL pool / worker 并发 / 同步历史 purge 等
 * - resolve* / isValid* / buildConfigRow 等 Node 端辅助
 * - 绑定具体 env 变量名的 SyncRuntimeConfigKey 联合类型
 *
 * 这些属于旧 Node/MySQL 后端，交由新的 Go 同步后端从零实现。
 * 前端只消费下面这个数据结构：由适配层（见 ui-action-adapter.ts）
 * 从 Go 后端取回 SyncRuntimeConfigRow[] 后传入 UI 渲染即可。
 * `key` 保持为普通 string，避免把 Node 端 env 名称固化进前端契约。
 */
export type SyncRuntimeConfigRow = {
  /** 配置项标识（由后端定义，前端仅透传/展示） */
  key: string;
  label: string;
  /** 当前运行中的实际有效值 */
  effectiveValue: number | string;
  /** UI 输入框的初始值：后端已保存的覆盖值，无则为 null */
  configuredValue: string | null;
  sourceLabel: string;
  rangeLabel: string;
  recommendedValue: string;
  reloadLabel: string;
  warning: string | null;
};
