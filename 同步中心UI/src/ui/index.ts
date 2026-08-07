/**
 * @polabel2/ui —— 共享 UI 占位包（Phase 5 主用）
 *
 * Phase 5 将实现：
 * - components/button.tsx
 * - components/table.tsx
 * - components/dialog.tsx
 * - components/form-field.tsx
 * - components/status-badge.tsx
 * - tokens/theme.ts
 *
 * Phase 1-3：
 * - 仅保留包边界，apps/web 可直接用 Tailwind 默认样式
 * - 不放业务请求，不读数据库，不持有 sync 状态
 *
 * 边界（ADR-0008）：
 * - 只依赖 ui 自身
 * - 禁止依赖 apps / db / read-models / workflows / sync-engine
 * - 组件只接 props，不做业务查询
 */

export const PACKAGE_NAME = "@polabel2/ui";
export {
  DateRangePicker,
  buildCalendarCells,
  formatSlashDate,
  normalizeDateRange,
  resolveDateRangePreset,
  type CalendarCell,
  type DateRangePreset,
  type DateRangePresetKey,
  type DateRangeValue,
} from "./date-range-picker";
