/**
 * 前端专用：判断一条同步记录的状态是否属于"进行中"。
 *
 * 仅保留 UI 展示需要的状态判定，不含任何后端/同步链路实现
 * （原 SQL 片段、raw intent 状态、DB 查询辅助已移除）。
 * 新的 Go 同步后端只需产出与下列字符串一致的 status 值即可复用此判定。
 */
export const PROJECTED_ACTIVE_SYNC_RUN_STATUSES = [
  "queued",
  "pending",
  "resource_ready",
  "waiting_resource",
  "admitted",
  "running",
  "retry_wait",
] as const;

export function isProjectedActiveSyncRunStatus(status: string): boolean {
  return (PROJECTED_ACTIVE_SYNC_RUN_STATUSES as readonly string[]).includes(
    status,
  );
}
