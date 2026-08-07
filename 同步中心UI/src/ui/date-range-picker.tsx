"use client";

import * as React from "react";
import { createPortal } from "react-dom";

export type DateRangeValue = { start: string; end: string };
export type DateRangePresetKey =
  | "7d"
  | "30d"
  | "month_current"
  | "month_previous"
  | "year_current"
  | "last_12_months"
  | "all"
  | "custom";

export type DateRangePreset = {
  key: Exclude<DateRangePresetKey, "custom">;
  label: string;
};

export type CalendarCell = {
  date: string;
  day: number;
  muted: boolean;
};

type DateRangePickerProps = {
  value?: DateRangeValue;
  presetKey?: DateRangePresetKey;
  presets?: DateRangePreset[];
  defaultOpen?: boolean;
  icon?: React.ReactNode;
  className?: string;
  triggerClassName?: string;
  onChange?: (next: {
    value: DateRangeValue;
    presetKey: DateRangePresetKey;
  }) => void;
};

const DEFAULT_PRESETS: DateRangePreset[] = [
  { key: "7d", label: "7天" },
  { key: "30d", label: "30天" },
  { key: "month_current", label: "本月" },
  { key: "month_previous", label: "上个月" },
  { key: "year_current", label: "本年" },
  { key: "last_12_months", label: "最近12个月" },
  { key: "all", label: "全部" },
];

const WEEKDAYS = ["一", "二", "三", "四", "五", "六", "日"];

export function DateRangePicker({
  value,
  presetKey = "7d",
  presets = DEFAULT_PRESETS,
  defaultOpen = false,
  icon,
  className = "",
  triggerClassName = "",
  onChange,
}: DateRangePickerProps) {
  const initialValue =
    value ?? resolveDateRangePreset(presetKey === "custom" ? "7d" : presetKey);
  const [open, setOpen] = React.useState(defaultOpen);
  const [currentValue, setCurrentValue] = React.useState<DateRangeValue>(
    normalizeDateRange(initialValue),
  );
  const [activePresetKey, setActivePresetKey] =
    React.useState<DateRangePresetKey>(presetKey);
  const [draftValue, setDraftValue] = React.useState<DateRangeValue>(
    normalizeDateRange(initialValue),
  );
  const [draftPresetKey, setDraftPresetKey] =
    React.useState<DateRangePresetKey>(presetKey);
  const [selecting, setSelecting] = React.useState<"start" | "end">("start");
  const [baseMonth, setBaseMonth] = React.useState(() =>
    startOfMonth(initialValue.start || initialValue.end),
  );
  const [popoverStyle, setPopoverStyle] = React.useState({
    left: 24,
    top: 0,
    width: 700,
    arrowLeft: 120,
  });
  const triggerRef = React.useRef<HTMLButtonElement>(null);
  const popoverRef = React.useRef<HTMLDivElement>(null);
  const months = React.useMemo(
    () => [baseMonth, addMonths(baseMonth, 1)],
    [baseMonth],
  );
  const committedValue = value ? normalizeDateRange(value) : currentValue;
  const committedPresetKey = value && presetKey ? presetKey : activePresetKey;

  React.useEffect(() => {
    if (!value) return;
    const normalized = normalizeDateRange(value);
    setCurrentValue(normalized);
    setDraftValue(normalized);
    setSelecting("start");
    setBaseMonth(startOfMonth(normalized.start || normalized.end));
  }, [value]);

  React.useEffect(() => {
    setActivePresetKey(presetKey);
    setDraftPresetKey(presetKey);
  }, [presetKey]);

  React.useEffect(() => {
    if (!open) return;
    positionPopover();
    const handlePointerDown = (event: PointerEvent) => {
      const target = event.target instanceof Node ? event.target : null;
      if (
        target &&
        (triggerRef.current?.contains(target) ||
          popoverRef.current?.contains(target))
      )
        return;
      discardAndClose();
    };
    const handleKeyDown = (event: KeyboardEvent) => {
      if (event.key === "Escape") discardAndClose();
    };
    window.addEventListener("resize", positionPopover);
    window.addEventListener("scroll", positionPopover, true);
    document.addEventListener("pointerdown", handlePointerDown, true);
    document.addEventListener("keydown", handleKeyDown, true);
    return () => {
      window.removeEventListener("resize", positionPopover);
      window.removeEventListener("scroll", positionPopover, true);
      document.removeEventListener("pointerdown", handlePointerDown, true);
      document.removeEventListener("keydown", handleKeyDown, true);
    };
  }, [open, committedValue.start, committedValue.end, committedPresetKey]);

  const canConfirm =
    draftPresetKey === "all" || (!!draftValue.start && !!draftValue.end);

  const popover = open ? (
    <div
      ref={popoverRef}
      className="ui-date-range-popover show"
      role="dialog"
      aria-label="日期范围"
      style={{
        left: popoverStyle.left,
        top: popoverStyle.top,
        width: popoverStyle.width,
        ["--date-popover-arrow-left" as string]: `${popoverStyle.arrowLeft}px`,
      }}
    >
      <div className="ui-date-range-main">
        <div className="ui-date-range-body">
          <div className="ui-date-range-presets">
            {presets.map((preset) => (
              <button
                key={preset.key}
                type="button"
                className={`ui-date-range-preset ${preset.key === draftPresetKey ? "active" : ""}`}
                onClick={() => selectPreset(preset.key)}
              >
                {preset.label}
              </button>
            ))}
          </div>
          <div className="ui-date-range-content">
            <div className="ui-date-range-calendars">
              {months.map((month, index) => (
                <CalendarMonth
                  key={formatDateInput(month)}
                  month={month}
                  showLeadingNav={index === 0}
                  showTrailingNav={index === months.length - 1}
                  value={draftValue}
                  onSelect={selectDate}
                  onShiftMonth={(amount) =>
                    setBaseMonth((current) => addMonths(current, amount))
                  }
                  onShiftYear={(amount) =>
                    setBaseMonth((current) => addYears(current, amount))
                  }
                />
              ))}
            </div>
            <div className="ui-date-range-actions">
              <button
                type="button"
                className="ui-date-range-cancel"
                onClick={discardAndClose}
              >
                取消
              </button>
              <button
                type="button"
                className="ui-date-range-confirm"
                disabled={!canConfirm}
                onClick={confirmSelection}
              >
                确定
              </button>
            </div>
          </div>
        </div>
      </div>
    </div>
  ) : null;

  return (
    <div className={`date-range-compact ui-date-range-compact ${className}`}>
      <button
        ref={triggerRef}
        type="button"
        className={`date-range-trigger ui-date-range-trigger ${open ? "open" : ""} ${triggerClassName}`}
        aria-expanded={open}
        onClick={() => {
          if (open) {
            discardAndClose();
            return;
          }
          setDraftValue(committedValue);
          setDraftPresetKey(committedPresetKey);
          setSelecting("start");
          setBaseMonth(
            startOfMonth(committedValue.start || committedValue.end),
          );
          setOpen(true);
        }}
      >
        <span className="date-range-current-label">
          {presetLabel(committedPresetKey, presets)}
        </span>
        <span className="date-range-trigger-inputs ui-date-range-trigger-inputs">
          <span
            className={`date-trigger-segment ui-date-trigger-segment start ${committedValue.start ? "" : "placeholder"}`}
          >
            {committedValue.start
              ? formatSlashDate(committedValue.start)
              : "开始日期"}
          </span>
          <span className="date-trigger-separator ui-date-trigger-separator">
            —
          </span>
          <span
            className={`date-trigger-segment ui-date-trigger-segment end ${committedValue.end ? "" : "placeholder"}`}
          >
            {committedValue.end
              ? formatSlashDate(committedValue.end)
              : "结束日期"}
          </span>
        </span>
        <span className="date-range-trigger-icon ui-date-range-trigger-icon">
          {icon ?? "▣"}
        </span>
      </button>
      {typeof document === "undefined"
        ? popover
        : createPortal(popover, document.body)}
    </div>
  );

  function positionPopover() {
    const anchor = triggerRef.current;
    if (!anchor) return;
    const rect = anchor.getBoundingClientRect();
    const viewportWidth = window.innerWidth;
    const viewportHeight = window.innerHeight;
    const viewportPadding = 24;
    const availableWidth = Math.max(280, viewportWidth - viewportPadding * 2);
    const width =
      availableWidth < 620 ? availableWidth : Math.min(700, availableWidth);
    const left = Math.min(
      Math.max(viewportPadding, rect.right - width),
      Math.max(viewportPadding, viewportWidth - width - viewportPadding),
    );
    const popoverHeight = popoverRef.current?.offsetHeight ?? 360;
    const top = Math.min(
      rect.bottom + 10,
      Math.max(
        viewportPadding,
        viewportHeight - popoverHeight - viewportPadding,
      ),
    );
    const arrowLeft = Math.min(
      Math.max(18, rect.left + rect.width / 2 - left - 7),
      Math.max(18, width - 32),
    );
    setPopoverStyle({ left, top, width, arrowLeft });
  }

  function selectPreset(nextPresetKey: Exclude<DateRangePresetKey, "custom">) {
    const nextValue = resolveDateRangePreset(nextPresetKey);
    setDraftValue(nextValue);
    setDraftPresetKey(nextPresetKey);
    setSelecting("start");
    setBaseMonth(startOfMonth(nextValue.start || nextValue.end));
  }

  function selectDate(date: string) {
    setDraftPresetKey("custom");
    if (
      !draftValue.start ||
      (draftValue.start && draftValue.end && selecting === "start")
    ) {
      setDraftValue({ start: date, end: "" });
      setSelecting("end");
      setBaseMonth(startOfMonth(date));
      return;
    }
    const nextValue = normalizeDateRange({
      start: draftValue.start,
      end: date,
    });
    setDraftValue(nextValue);
    setSelecting("start");
  }

  function discardAndClose() {
    setDraftValue(committedValue);
    setDraftPresetKey(committedPresetKey);
    setSelecting("start");
    setBaseMonth(startOfMonth(committedValue.start || committedValue.end));
    setOpen(false);
  }

  function confirmSelection() {
    const nextValue = normalizeDateRange(draftValue);
    if (!canConfirm) return;
    const changed =
      nextValue.start !== committedValue.start ||
      nextValue.end !== committedValue.end ||
      draftPresetKey !== committedPresetKey;
    setCurrentValue(nextValue);
    setDraftValue(nextValue);
    setActivePresetKey(draftPresetKey);
    setSelecting("start");
    setOpen(false);
    if (changed) onChange?.({ value: nextValue, presetKey: draftPresetKey });
  }
}

function CalendarMonth({
  month,
  value,
  showLeadingNav,
  showTrailingNav,
  onSelect,
  onShiftMonth,
  onShiftYear,
}: {
  month: Date;
  value: DateRangeValue;
  showLeadingNav: boolean;
  showTrailingNav: boolean;
  onSelect: (date: string) => void;
  onShiftMonth: (amount: number) => void;
  onShiftYear: (amount: number) => void;
}) {
  return (
    <div className="calendar-pane">
      <div className="calendar-head">
        <div className="calendar-nav">
          {showLeadingNav ? (
            <>
              <button
                type="button"
                aria-label="上一年"
                onClick={() => onShiftYear(-1)}
              >
                «
              </button>
              <button
                type="button"
                aria-label="上一月"
                onClick={() => onShiftMonth(-1)}
              >
                ‹
              </button>
            </>
          ) : null}
        </div>
        <div className="calendar-title">{formatCalendarTitle(month)}</div>
        <div className="calendar-nav trailing">
          {showTrailingNav ? (
            <>
              <button
                type="button"
                aria-label="下一月"
                onClick={() => onShiftMonth(1)}
              >
                ›
              </button>
              <button
                type="button"
                aria-label="下一年"
                onClick={() => onShiftYear(1)}
              >
                »
              </button>
            </>
          ) : null}
        </div>
      </div>
      <div className="calendar-grid">
        <div className="calendar-weekdays">
          {WEEKDAYS.map((weekday) => (
            <span key={weekday}>{weekday}</span>
          ))}
        </div>
        <div className="calendar-days">
          {buildCalendarCells(month).map((cell) => (
            <CalendarDay
              key={cell.date}
              cell={cell}
              value={value}
              onSelect={onSelect}
            />
          ))}
        </div>
      </div>
    </div>
  );
}

function CalendarDay({
  cell,
  value,
  onSelect,
}: {
  cell: CalendarCell;
  value: DateRangeValue;
  onSelect: (date: string) => void;
}) {
  const isStart = !cell.muted && value.start && cell.date === value.start;
  const isEnd = !cell.muted && value.end && cell.date === value.end;
  const isSingle = !!(isStart && isEnd);
  const inRange =
    !cell.muted &&
    !isStart &&
    !isEnd &&
    isDateBetween(cell.date, value.start, value.end);
  const previewEdge =
    !cell.muted && !value.end && value.start && cell.date === value.start;
  const className = [
    "calendar-day",
    cell.muted ? "muted" : "",
    inRange ? "in-range" : "",
    isStart ? "range-start" : "",
    isEnd ? "range-end" : "",
    isSingle ? "single" : "",
    previewEdge ? "preview-edge" : "",
  ]
    .filter(Boolean)
    .join(" ");
  const labelClassName = ["day-label"].filter(Boolean).join(" ");
  return (
    <button
      type="button"
      className={className}
      data-date={cell.date}
      onClick={() => onSelect(cell.date)}
    >
      <span className={labelClassName}>{cell.day}</span>
    </button>
  );
}

export function resolveDateRangePreset(
  presetKey: Exclude<DateRangePresetKey, "custom">,
  seedDate = new Date(),
): DateRangeValue {
  const today = new Date(
    seedDate.getFullYear(),
    seedDate.getMonth(),
    seedDate.getDate(),
  );
  if (presetKey === "30d")
    return {
      start: formatDateInput(addDays(today, -29)),
      end: formatDateInput(today),
    };
  if (presetKey === "month_current")
    return {
      start: formatDateInput(
        new Date(today.getFullYear(), today.getMonth(), 1),
      ),
      end: formatDateInput(today),
    };
  if (presetKey === "month_previous") {
    return {
      start: formatDateInput(
        new Date(today.getFullYear(), today.getMonth() - 1, 1),
      ),
      end: formatDateInput(new Date(today.getFullYear(), today.getMonth(), 0)),
    };
  }
  if (presetKey === "year_current")
    return {
      start: formatDateInput(new Date(today.getFullYear(), 0, 1)),
      end: formatDateInput(today),
    };
  if (presetKey === "last_12_months")
    return {
      start: formatDateInput(addMonths(today, -12)),
      end: formatDateInput(today),
    };
  if (presetKey === "all") return { start: "", end: "" };
  return {
    start: formatDateInput(addDays(today, -6)),
    end: formatDateInput(today),
  };
}

export function buildCalendarCells(monthDate: Date): CalendarCell[] {
  const firstDay = new Date(monthDate.getFullYear(), monthDate.getMonth(), 1);
  const weekday = (firstDay.getDay() + 6) % 7;
  const start = new Date(firstDay);
  start.setDate(firstDay.getDate() - weekday);
  return Array.from({ length: 42 }, (_, index) => {
    const current = new Date(start);
    current.setDate(start.getDate() + index);
    return {
      date: formatDateInput(current),
      day: current.getDate(),
      muted: current.getMonth() !== monthDate.getMonth(),
    };
  });
}

export function normalizeDateRange(
  value: Partial<DateRangeValue>,
): DateRangeValue {
  const start = String(value.start || "").trim();
  const end = String(value.end || "").trim();
  if (!start && !end) return { start: "", end: "" };
  if (!start) return { start: "", end };
  if (!end) return { start, end: "" };
  return start <= end ? { start, end } : { start: end, end: start };
}

export function formatSlashDate(value: string): string {
  return String(value || "").replaceAll("-", "/");
}

function presetLabel(
  key: DateRangePresetKey,
  presets: DateRangePreset[],
): string {
  if (key === "custom") return "自定义";
  return presets.find((preset) => preset.key === key)?.label ?? "自定义";
}

function startOfMonth(value = formatDateInput(new Date())): Date {
  const date = new Date(
    `${String(value || formatDateInput(new Date())).slice(0, 10)}T00:00:00`,
  );
  return new Date(date.getFullYear(), date.getMonth(), 1);
}

function addDays(date: Date, days: number): Date {
  const next = new Date(date);
  next.setDate(next.getDate() + days);
  return next;
}

function addMonths(date: Date, months: number): Date {
  return new Date(date.getFullYear(), date.getMonth() + months, 1);
}

function addYears(date: Date, years: number): Date {
  return new Date(date.getFullYear() + years, date.getMonth(), 1);
}

function formatCalendarTitle(date: Date): string {
  return `${date.getFullYear()} 年 ${date.getMonth() + 1} 月`;
}

function formatDateInput(date: Date): string {
  return `${date.getFullYear()}-${String(date.getMonth() + 1).padStart(2, "0")}-${String(date.getDate()).padStart(2, "0")}`;
}

function isDateBetween(date: string, start: string, end: string): boolean {
  if (!start || !end) return false;
  return date >= start && date <= end;
}
