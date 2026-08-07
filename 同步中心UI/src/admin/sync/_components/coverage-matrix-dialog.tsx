"use client";

import { Loader2 } from "lucide-react";
import { useCallback, useEffect, useMemo, useState } from "react";
import { toast } from "sonner";

import { Button } from "@/components/ui/button";
import {
  Dialog,
  DialogContent,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog";

import {
  listStoreCoverageDetailAction,
  showSyncActionUnavailable,
} from "../ui-action-adapter";
import type {
  CoverageDimension,
  StoreCoverageDetail,
  SyncOverviewStoreRow,
} from "../ui-types";

type MatrixCell = {
  key: string;
  date: string;
  dimension: CoverageDimension;
  status: "synced" | "failed" | "empty" | "gap" | "out";
};

type CoverageDimensionMeta = {
  key: CoverageDimension;
  label: string;
  short: string;
};
type CoverageDimensionTask = {
  syncType: string;
  label: string;
  snapshot: boolean;
};
type CoverageMatrixNotice = { message: string; lines: string[] };

const DIMENSIONS: CoverageDimensionMeta[] = [
  { key: "sales", label: "销量", short: "销" },
  { key: "ads", label: "广告", short: "广" },
  { key: "inventory", label: "库存", short: "库" },
  { key: "performance", label: "表现", short: "表" },
  { key: "traffic", label: "访问量", short: "访" },
  { key: "margin", label: "利润率", short: "利" },
];

export function CoverageMatrixDialog({
  row,
  open,
  onOpenChange,
}: {
  row: SyncOverviewStoreRow | null;
  open: boolean;
  onOpenChange: (open: boolean) => void;
}) {
  const [detail, setDetail] = useState<StoreCoverageDetail | null>(null);
  const [loading, setLoading] = useState(false);
  const [submitting, setSubmitting] = useState(false);
  const [selected, setSelected] = useState<Set<string>>(() => new Set());
  const [dragAnchor, setDragAnchor] = useState<MatrixCell | null>(null);
  const [hoveredMonth, setHoveredMonth] = useState<string | null>(null);
  const [notice, setNotice] = useState<CoverageMatrixNotice | null>(null);

  useEffect(() => {
    if (!open || !row?.data_source_id || !row.channel_type) return;
    let alive = true;
    setLoading(true);
    setSelected(new Set());
    listStoreCoverageDetailAction({
      dataSourceId: row.data_source_id,
      storeId: row.store_key,
      channelType: row.channel_type,
    })
      .then((nextDetail) => {
        if (alive) setDetail(nextDetail);
      })
      .catch((error) => {
        if (!alive) return;
        setDetail(null);
        setNotice({
          message: "加载覆盖矩阵失败",
          lines: [error instanceof Error ? error.message : String(error)],
        });
      })
      .finally(() => {
        if (alive) setLoading(false);
      });
    return () => {
      alive = false;
    };
  }, [open, row]);

  const cells = useMemo(() => buildCells(detail), [detail]);
  const selectedCells = useMemo(
    () =>
      cells
        .flatMap((month) => month.cells)
        .filter((cell) => selected.has(cell.key)),
    [cells, selected],
  );
  const selection = useMemo(
    () => describeSelection(selectedCells, row?.channel_type ?? null),
    [row?.channel_type, selectedCells],
  );

  const selectCells = useCallback(
    (nextCells: MatrixCell[], mode: "append" | "replace" = "append") => {
      const nextKeys = nextCells
        .filter(isSelectableCell)
        .map((cell) => cell.key);
      setSelected((current) => {
        if (mode === "replace") return new Set(nextKeys);
        const merged = new Set(current);
        nextKeys.forEach((key) => merged.add(key));
        return merged;
      });
    },
    [],
  );

  const handleSubmit = useCallback(async () => {
    if (!row?.data_source_id || !row.channel_type || !selection) return;
    if (selection.tasks.some((task) => task.dayCount > 90)) {
      setNotice({
        message: "补全同步提交失败",
        lines: ["单次补全同步不能超过 90 天"],
      });
      return;
    }
    setSubmitting(true);
    setNotice(null);
    try {
      for (const task of selection.tasks) {
        const form = new FormData();
        form.set("syncType", task.syncType);
        form.set("dataSourceId", row.data_source_id);
        form.set("stores", row.store_key);
        form.set("channelType", row.channel_type);
        form.set("start", task.startDate);
        form.set("end", task.endDate);
        await showSyncActionUnavailable(form);
      }
      toast.success("已提交补全任务，可在同步日志查看进度");
      onOpenChange(false);
    } catch (error) {
      setNotice({
        message: "补全同步提交失败",
        lines: [error instanceof Error ? error.message : "补全同步提交失败"],
      });
    } finally {
      setSubmitting(false);
    }
  }, [onOpenChange, row, selection]);

  const allCells = cells.flatMap((month) => month.cells);

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent
        className="!fixed !left-1/2 !right-auto !top-1/2 !bottom-auto w-[min(840px,94vw)] !-translate-x-1/2 !-translate-y-1/2"
        data-testid="coverage-matrix-dialog"
      >
        <DialogHeader>
          <DialogTitle>
            {row ? `${formatStoreLabel(row)} — 覆盖矩阵` : "同步覆盖矩阵"}
          </DialogTitle>
        </DialogHeader>

        <div
          className="grid min-h-0 gap-1 px-5 py-2"
          onMouseLeave={() => setDragAnchor(null)}
          onMouseUp={() => setDragAnchor(null)}
        >
          <CoverageMatrixNoticeBanner
            notice={notice}
            onClose={() => setNotice(null)}
          />
          <div
            data-testid="coverage-matrix-toolbar"
            className="flex flex-wrap items-center justify-between gap-1.5 border-b border-line bg-white pb-1.5 text-xs text-ink-sub"
          >
            <div className="flex flex-wrap items-center gap-2">
              <LegendDot className="bg-emerald-500" label="已同步" />
              <LegendDot className="bg-red-500" label="同步失败" />
              <LegendDot className="bg-slate-400" label="上游无数据" />
              <LegendDot
                className="rounded-[2px] border border-slate-300 bg-white"
                label="未同步(可选)"
              />
              <LegendDot className="bg-slate-200" label="范围外" />
              {DIMENSIONS.map((dimension) => {
                const display = resolveCoverageDimensionTask(
                  dimension,
                  row?.channel_type ?? null,
                );
                const label = display?.label ?? dimension.label;
                return (
                  <span key={dimension.key}>
                    {dimension.short}={label}
                    {display?.snapshot ? " (当日快照，不可补历史)" : ""}
                  </span>
                );
              })}
            </div>
            <div className="ml-auto flex items-center gap-2">
              <Button
                type="button"
                variant="outline"
                size="sm"
                onClick={() =>
                  selectCells(allCells.filter(isSelectableCell), "replace")
                }
                disabled={!allCells.length}
              >
                全选空值
              </Button>
              <Button
                type="button"
                size="sm"
                onClick={() => void handleSubmit()}
                disabled={!selection || submitting}
              >
                {submitting ? (
                  <Loader2 className="h-4 w-4 animate-spin" />
                ) : null}
                {submitting ? "处理中..." : "补全同步"}
              </Button>
            </div>
          </div>

          {loading ? (
            <div className="flex h-64 items-center justify-center text-sm text-ink-sub">
              <Loader2 className="mr-2 h-4 w-4 animate-spin" />
              加载覆盖矩阵...
            </div>
          ) : (
            <div
              data-testid="coverage-matrix-scroll"
              className="min-h-0 overflow-auto"
            >
              <table
                className="w-full min-w-[1120px] table-fixed text-xs leading-none"
                data-testid="sync-coverage-dialog"
              >
                <thead>
                  <tr>
                    <th
                      data-testid="coverage-day-column"
                      className="sticky left-0 z-[3] w-9 bg-white px-1 py-1"
                    />
                    {cells.map((month) => (
                      <th
                        key={month.month}
                        colSpan={DIMENSIONS.length}
                        className="px-5 py-1 text-center font-extrabold text-slate-700"
                      >
                        <label
                          data-testid="coverage-month-label"
                          className="inline-flex min-w-28 items-center justify-center gap-1.5 px-3 py-1"
                        >
                          <input
                            type="checkbox"
                            className="h-3.5 w-3.5"
                            aria-label={`全选 ${month.month} 空值`}
                            checked={
                              month.cells.some(isSelectableCell) &&
                              month.cells
                                .filter(isSelectableCell)
                                .every((cell) => selected.has(cell.key))
                            }
                            onChange={(event) => {
                              if (event.currentTarget.checked) {
                                selectCells(
                                  month.cells.filter(isSelectableCell),
                                );
                                return;
                              }
                              setSelected((current) => {
                                const next = new Set(current);
                                month.cells.forEach((cell) =>
                                  next.delete(cell.key),
                                );
                                return next;
                              });
                            }}
                          />
                          {month.month}
                        </label>
                      </th>
                    ))}
                  </tr>
                  <tr>
                    <th
                      data-testid="coverage-day-column"
                      className="sticky left-0 z-[3] bg-white px-1 py-1"
                    />
                    {cells.map((month, monthIndex) =>
                      DIMENSIONS.map((dimension, dimensionIndex) => (
                        <th
                          key={`${month.month}:${dimension.key}`}
                          className={monthCellClass({
                            month: month.month,
                            hoveredMonth,
                            monthIndex,
                            dimensionIndex,
                            section: "top",
                          })}
                          onMouseEnter={() => setHoveredMonth(month.month)}
                          onMouseLeave={() => setHoveredMonth(null)}
                        >
                          <button
                            type="button"
                            className="rounded px-0.5 hover:bg-slate-100"
                            onClick={() =>
                              selectCells(
                                month.cells.filter(
                                  (cell) => cell.dimension === dimension.key,
                                ),
                              )
                            }
                            title={`${month.month} ${resolveCoverageDimensionTask(dimension, row?.channel_type ?? null)?.label ?? dimension.label}`}
                            aria-label={`${month.month} ${resolveCoverageDimensionTask(dimension, row?.channel_type ?? null)?.label ?? dimension.label}`}
                          >
                            {dimension.short}
                          </button>
                        </th>
                      )),
                    )}
                  </tr>
                </thead>
                <tbody>
                  {Array.from({ length: 31 }, (_, index) => index + 1).map(
                    (day) => (
                      <tr key={day}>
                        <td
                          data-testid="coverage-day-column"
                          className="sticky left-0 z-[2] bg-white px-1 py-0 text-right font-semibold text-slate-400 shadow-[6px_0_10px_-10px_rgba(15,23,42,0.6)]"
                        >
                          <button
                            type="button"
                            className="rounded px-1 hover:bg-slate-100"
                            onClick={() =>
                              selectCells(
                                allCells.filter(
                                  (cell) =>
                                    Number(cell.date.slice(8, 10)) === day,
                                ),
                              )
                            }
                          >
                            {day}
                          </button>
                        </td>
                        {cells.map((month, monthIndex) =>
                          DIMENSIONS.map((dimension, dimensionIndex) => {
                            const cell = month.cells.find(
                              (item) =>
                                item.dimension === dimension.key &&
                                Number(item.date.slice(8, 10)) === day,
                            );
                            return (
                              <td
                                key={`${month.month}:${dimension.key}:${day}`}
                                data-testid="coverage-month-cell"
                                className={monthCellClass({
                                  month: month.month,
                                  hoveredMonth,
                                  monthIndex,
                                  dimensionIndex,
                                  section: day === 31 ? "bottom" : "middle",
                                })}
                                onMouseEnter={() =>
                                  setHoveredMonth(month.month)
                                }
                                onMouseLeave={() => setHoveredMonth(null)}
                              >
                                {cell ? (
                                  <button
                                    type="button"
                                    aria-label={`${cell.date} ${resolveCoverageDimensionTask(dimension, row?.channel_type ?? null)?.label ?? dimension.label}`}
                                    className={cellClass(
                                      cell,
                                      selected.has(cell.key),
                                    )}
                                    onMouseDown={() => {
                                      if (!isSelectableCell(cell)) return;
                                      setDragAnchor(cell);
                                      selectCells([cell]);
                                    }}
                                    onMouseEnter={() => {
                                      if (
                                        !dragAnchor ||
                                        dragAnchor.dimension !==
                                          cell.dimension ||
                                        cell.status === "out"
                                      )
                                        return;
                                      selectCells(
                                        allCells.filter(
                                          (item) =>
                                            item.dimension === cell.dimension &&
                                            inDateRange(
                                              item.date,
                                              dragAnchor.date,
                                              cell.date,
                                            ),
                                        ),
                                      );
                                    }}
                                    onClick={() => {
                                      if (!isSelectableCell(cell)) return;
                                      selectCells([cell]);
                                    }}
                                  />
                                ) : (
                                  <span className="inline-block h-[9px] w-[9px] rounded-full bg-slate-200" />
                                )}
                              </td>
                            );
                          }),
                        )}
                      </tr>
                    ),
                  )}
                </tbody>
              </table>
            </div>
          )}
        </div>
      </DialogContent>
    </Dialog>
  );
}

function CoverageMatrixNoticeBanner({
  notice,
  onClose,
}: {
  notice: CoverageMatrixNotice | null;
  onClose: () => void;
}) {
  if (!notice) return null;
  return (
    <div
      data-testid="coverage-matrix-local-notice"
      className="mb-2 flex items-start justify-between gap-3 rounded-lg border border-red-200 bg-red-50 px-3 py-2 text-sm font-semibold text-red-700"
    >
      <div className="grid min-w-0 gap-1">
        <span className="font-extrabold">{notice.message}</span>
        {notice.lines.map((line) => (
          <span
            key={line}
            className="whitespace-normal break-words [overflow-wrap:anywhere]"
          >
            {line}
          </span>
        ))}
      </div>
      <button
        type="button"
        className="shrink-0 text-lg font-bold leading-none opacity-70 hover:opacity-100"
        aria-label="关闭错误提示"
        onClick={onClose}
      >
        ×
      </button>
    </div>
  );
}

function buildCells(detail: StoreCoverageDetail | null) {
  if (!detail) return [];
  const months = monthStarts(detail.start_date, detail.end_date);
  const failedDatesByDimension = buildFailedDates(detail);
  const successDatesByDimension = buildSuccessDates(detail);
  return months.map((month) => {
    const monthDates = datesInMonth(month, detail.start_date, detail.end_date);
    return {
      month,
      cells: monthDates.flatMap((date) =>
        DIMENSIONS.map((dimension) => {
          const synced = new Set(detail.dates[dimension.key]).has(date);
          const failed = failedDatesByDimension[dimension.key].has(date);
          const empty =
            !synced && successDatesByDimension[dimension.key].has(date);
          const out = detail.unavailable.includes(dimension.key);
          const status: MatrixCell["status"] = out
            ? "out"
            : synced
              ? "synced"
              : empty
                ? "empty"
                : failed
                  ? "failed"
                  : "gap";
          return {
            key: `${date}:${dimension.key}`,
            date,
            dimension: dimension.key,
            status,
          };
        }),
      ),
    };
  });
}

function buildSuccessDates(detail: StoreCoverageDetail) {
  const map: Record<CoverageDimension, Set<string>> = {
    sales: new Set(),
    ads: new Set(),
    inventory: new Set(),
    performance: new Set(),
    traffic: new Set(),
    margin: new Set(),
  };
  for (const run of detail.successDates ?? []) {
    const dimension = syncTypeToDimension(run.sync_type);
    if (!dimension || !run.start_date) continue;
    for (const date of eachDate(run.start_date, run.end_date ?? run.start_date))
      map[dimension].add(date);
  }
  return map;
}

function buildFailedDates(detail: StoreCoverageDetail) {
  const map: Record<CoverageDimension, Set<string>> = {
    sales: new Set(),
    ads: new Set(),
    inventory: new Set(),
    performance: new Set(),
    traffic: new Set(),
    margin: new Set(),
  };
  for (const failed of detail.failed) {
    const dimension = syncTypeToDimension(failed.sync_type);
    if (!dimension || !failed.start_date) continue;
    for (const date of eachDate(
      failed.start_date,
      failed.end_date ?? failed.start_date,
    ))
      map[dimension].add(date);
  }
  return map;
}

function syncTypeToDimension(syncType: string): CoverageDimension | null {
  if (syncType === "sync:vc-traffic") return "traffic";
  if (syncType === "sync:vc-margin") return "margin";
  if (syncType.includes("sales")) return "sales";
  if (syncType.includes("ads")) return "ads";
  if (syncType.includes("inventory")) return "inventory";
  if (syncType.includes("performance")) return "performance";
  return null;
}

export function describeCoverageSelectionForTest(
  cells: MatrixCell[],
  channelType: string | null,
) {
  return describeSelection(cells, channelType);
}

function describeSelection(cells: MatrixCell[], channelType: string | null) {
  if (!cells.length) return null;
  const tasks = DIMENSIONS.map((dimensionMeta) => {
    const dates = [
      ...new Set(
        cells
          .filter((cell) => cell.dimension === dimensionMeta.key)
          .map((cell) => cell.date),
      ),
    ].sort();
    if (!dates.length) return null;
    const taskMeta = resolveCoverageDimensionTask(dimensionMeta, channelType);
    if (!taskMeta) return null;
    // 快照维度（如 SC 库存）不传日期窗口，由服务端强制当天
    if (taskMeta.snapshot) {
      return {
        startDate: "",
        endDate: "",
        syncType: taskMeta.syncType,
        dayCount: 0,
        dimensionLabel: taskMeta.label,
      };
    }
    const startDate = dates[0] ?? "";
    const endDate = dates[dates.length - 1] ?? startDate;
    return {
      startDate,
      endDate,
      syncType: taskMeta.syncType,
      dayCount: dates.length,
      dimensionLabel: taskMeta.label,
    };
  }).filter(
    (
      task,
    ): task is {
      startDate: string;
      endDate: string;
      syncType: string;
      dayCount: number;
      dimensionLabel: string;
    } => Boolean(task),
  );
  if (!tasks.length) return null;
  const allDates = [...new Set(cells.map((cell) => cell.date))].sort();
  return {
    tasks,
    label: `${allDates[0] ?? ""}~${allDates[allDates.length - 1] ?? ""} · ${tasks.map((task) => task.dimensionLabel).join("、")} · ${tasks.map((task) => task.syncType).join(" / ")}`,
  };
}

function resolveCoverageDimensionTask(
  dimension: CoverageDimensionMeta,
  channelType: string | null,
): CoverageDimensionTask | null {
  if (dimension.key === "sales")
    return {
      syncType: channelType === "vc" ? "sync:vc-sales" : "sync:sc-sales",
      label: dimension.label,
      snapshot: false,
    };
  if (dimension.key === "inventory") {
    if (channelType === "vc")
      return { syncType: "sync:vc-inventory", label: "库存", snapshot: false };
    if (channelType === "sc")
      return {
        syncType: "sync:sc-inventory",
        label: "库存(快照)",
        snapshot: true,
      };
    return null;
  }
  if (dimension.key === "traffic") {
    if (channelType === "vc")
      return { syncType: "sync:vc-traffic", label: "访问量", snapshot: false };
    return null;
  }
  if (dimension.key === "ads") {
    if (channelType === "vc")
      return {
        syncType: "sync:vc-ads",
        label: dimension.label,
        snapshot: false,
      };
    return { syncType: "sync:sc-ads", label: dimension.label, snapshot: false };
  }
  if (dimension.key === "performance") {
    return null;
  }
  if (dimension.key === "margin") {
    if (channelType === "vc")
      return { syncType: "sync:vc-margin", label: "利润率", snapshot: false };
    return null;
  }
  return null;
}

function isSelectableCell(cell: MatrixCell) {
  return cell.status === "gap" || cell.status === "failed";
}

function inDateRange(date: string, left: string, right: string) {
  const [start, end] = left <= right ? [left, right] : [right, left];
  return date >= start && date <= end;
}

function cellClass(cell: MatrixCell, selected: boolean) {
  const base = "inline-block h-[7px] w-[7px] align-middle transition";
  if (selected)
    return `${base} rounded-[2px] border-2 border-blue-600 bg-blue-100`;
  if (cell.status === "synced") return `${base} rounded-full bg-emerald-500`;
  if (cell.status === "failed") return `${base} rounded-full bg-red-500`;
  if (cell.status === "empty") return `${base} rounded-full bg-slate-400`;
  if (cell.status === "out") return `${base} rounded-full bg-slate-200`;
  return `${base} rounded-[2px] border border-slate-300 bg-white`;
}

function monthCellClass({
  month,
  hoveredMonth,
  monthIndex,
  dimensionIndex,
  section,
}: {
  month: string;
  hoveredMonth: string | null;
  monthIndex: number;
  dimensionIndex: number;
  section: "top" | "middle" | "bottom";
}) {
  const monthGap = dimensionIndex === 0 && monthIndex > 0 ? "pl-5" : "pl-0.5";
  const cardEdge = [
    dimensionIndex === 0 ? "border-l border-slate-200" : "",
    dimensionIndex === DIMENSIONS.length - 1 ? "border-r border-slate-200" : "",
    section === "top" ? "border-t border-slate-200" : "",
    section === "bottom" ? "border-b border-slate-200" : "",
  ]
    .filter(Boolean)
    .join(" ");
  const background = hoveredMonth === month ? "bg-slate-50" : "bg-white";
  return `${monthGap} ${cardEdge} pr-0.5 ${section === "top" ? "py-0.5 font-bold text-slate-500" : "py-0"} text-center ${background} transition-colors hover:bg-slate-50`;
}

function LegendDot({ className, label }: { className: string; label: string }) {
  return (
    <span>
      <span
        className={`mr-1 inline-block h-[9px] w-[9px] align-middle ${className}`}
      />
      {label}
    </span>
  );
}

function monthStarts(start: string, end: string) {
  const result: string[] = [];
  const cursor = parseDate("2026-01-01");
  const endDate = parseDate(`${end.slice(0, 7)}-01`);
  while (cursor <= endDate) {
    result.push(formatDate(cursor).slice(0, 7));
    cursor.setMonth(cursor.getMonth() + 1);
  }
  return result.reverse();
}

function datesInMonth(month: string, start: string, end: string) {
  const first = parseDate(`${month}-01`);
  const last = new Date(first);
  last.setMonth(first.getMonth() + 1, 0);
  return eachDate(
    maxDate(formatDate(first), "2026-01-01"),
    minDate(formatDate(last), end),
  );
}

function eachDate(start: string, end: string) {
  const result: string[] = [];
  const cursor = parseDate(start);
  const last = parseDate(end);
  while (cursor <= last) {
    result.push(formatDate(cursor));
    cursor.setDate(cursor.getDate() + 1);
  }
  return result;
}

function parseDate(value: string) {
  const [year, month, day] = value.split("-").map(Number);
  return new Date(year ?? 1970, (month ?? 1) - 1, day ?? 1);
}

function formatDate(value: Date) {
  return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, "0")}-${String(value.getDate()).padStart(2, "0")}`;
}

function minDate(left: string, right: string) {
  return left <= right ? left : right;
}

function maxDate(left: string, right: string) {
  return left >= right ? left : right;
}

function formatStoreLabel(row: SyncOverviewStoreRow) {
  const prefix = row.channel_type === "sc" ? "SC" : "VC";
  return `(${prefix}) ${row.store_label}`;
}
