import * as React from "react";

type PaginationQuery = Record<string, string | number | undefined>;

type PaginationProps = {
  basePath: string;
  page: number;
  pageSize: number;
  total: number;
  query?: PaginationQuery;
};

type PaginationButtonProps = {
  disabled?: boolean;
  href?: string;
  label: string;
  onClick?: () => void;
};

type PaginationButtonsProps = {
  disabled?: boolean;
  hasNext?: boolean;
  hrefForPage?: (page: number) => string;
  onPageChange?: (page: number) => void;
  page: number;
  totalPages: number;
};

type PaginationStatusInfoProps = {
  hasNext?: boolean;
  page: number;
  total: number;
  totalPages: number;
};

type PaginationStatusBarProps = {
  className?: string;
  leftItems: React.ReactNode;
  middleItems?: React.ReactNode;
  onPageChange?: (page: number) => void;
  page: number;
  total: number;
  totalPages: number;
  hrefForPage?: (page: number) => string;
  disabled?: boolean;
  hasNext?: boolean;
  testId?: string;
};

export function Pagination({
  basePath,
  page,
  pageSize,
  total,
  query,
}: PaginationProps) {
  const totalPages = Math.max(1, Math.ceil(total / pageSize));

  return (
    <PaginationStatusBar
      leftItems={
        <span>
          本页数量：
          <strong>
            {Math.min(pageSize, Math.max(0, total - (page - 1) * pageSize))}
          </strong>
        </span>
      }
      middleItems={null}
      page={page}
      total={total}
      totalPages={totalPages}
      hrefForPage={(targetPage) =>
        buildPaginationHref(basePath, targetPage, query)
      }
    />
  );
}

export function PaginationStatusBar({
  className,
  disabled,
  hrefForPage,
  hasNext,
  leftItems,
  middleItems,
  onPageChange,
  page,
  testId,
  total,
  totalPages,
}: PaginationStatusBarProps) {
  return (
    <div
      aria-label="分页状态栏"
      className={
        className ??
        "flex min-h-10 shrink-0 items-center justify-between gap-3 border-t border-line bg-white px-[18px] py-1 text-[13px] leading-none"
      }
      data-testid={testId}
    >
      <PaginationStatusItems items={leftItems} />
      {middleItems ? (
        <div className="inline-flex min-h-8 flex-1 items-center gap-2 overflow-hidden text-ellipsis whitespace-nowrap text-xs text-slate-500">
          {middleItems}
        </div>
      ) : (
        <span className="flex-1" />
      )}
      <nav
        aria-label="分页"
        className="flex h-8 min-w-[360px] shrink-0 items-center justify-between text-xs font-semibold text-ink-sub"
      >
        <PaginationStatusInfo
          hasNext={hasNext}
          page={page}
          total={total}
          totalPages={totalPages}
        />
        <PaginationButtons
          disabled={disabled}
          hasNext={hasNext}
          hrefForPage={hrefForPage}
          onPageChange={onPageChange}
          page={page}
          totalPages={totalPages}
        />
      </nav>
    </div>
  );
}

export function PaginationStatusInfo({
  hasNext,
  page,
  total,
  totalPages,
}: PaginationStatusInfoProps) {
  if (hasNext !== undefined)
    return (
      <span className="inline-flex items-center">
        第 {page} 页 · {hasNext ? "还有下一页" : "已到底"}
      </span>
    );
  return (
    <span className="inline-flex items-center">
      共 <b className="text-ink">{total}</b> 条 · 第 {page} / {totalPages} 页
    </span>
  );
}

export function PaginationButtons({
  disabled,
  hasNext,
  hrefForPage,
  onPageChange,
  page,
  totalPages,
}: PaginationButtonsProps) {
  const go = (targetPage: number) => {
    if (onPageChange) onPageChange(targetPage);
  };
  return (
    <span className="inline-flex items-center gap-2">
      <PaginationButton
        disabled={disabled || page <= 1}
        href={hrefForPage?.(1)}
        label="首页"
        onClick={onPageChange ? () => go(1) : undefined}
      />
      <PaginationButton
        disabled={disabled || page <= 1}
        href={hrefForPage?.(Math.max(1, page - 1))}
        label="上一页"
        onClick={onPageChange ? () => go(Math.max(1, page - 1)) : undefined}
      />
      <PaginationButton
        disabled={
          disabled || (hasNext === undefined ? page >= totalPages : !hasNext)
        }
        href={hrefForPage?.(
          hasNext === undefined ? Math.min(totalPages, page + 1) : page + 1,
        )}
        label="下一页"
        onClick={
          onPageChange
            ? () =>
                go(
                  hasNext === undefined
                    ? Math.min(totalPages, page + 1)
                    : page + 1,
                )
            : undefined
        }
      />
      {hasNext === undefined ? (
        <PaginationButton
          disabled={disabled || page >= totalPages}
          href={hrefForPage?.(totalPages)}
          label="尾页"
          onClick={onPageChange ? () => go(totalPages) : undefined}
        />
      ) : null}
    </span>
  );
}

export function buildPaginationHref(
  basePath: string,
  page: number,
  query?: PaginationQuery,
): string {
  const search = new URLSearchParams();
  Object.entries(query ?? {}).forEach(([key, value]) => {
    if (value === undefined || value === "") return;
    search.set(key, String(value));
  });
  if (page > 1) search.set("page", String(page));
  const queryString = search.toString();
  return queryString ? `${basePath}?${queryString}` : basePath;
}

export function replacePaginationUrl(
  basePath: string,
  page: number,
  query?: PaginationQuery,
) {
  if (typeof window === "undefined") return;
  window.history.replaceState(
    null,
    "",
    buildPaginationHref(basePath, page, query),
  );
}

function PaginationStatusItems({ items }: { items: React.ReactNode }) {
  return (
    <div className="flex min-w-0 flex-1 items-center gap-[18px] overflow-hidden whitespace-nowrap text-xs font-bold text-slate-900">
      {items}
    </div>
  );
}

function PaginationButton({
  disabled,
  href,
  label,
  onClick,
}: PaginationButtonProps) {
  const className = disabled
    ? "pointer-events-none inline-flex min-h-7 items-center justify-center rounded border border-line px-2.5 py-1 text-slate-300"
    : "inline-flex min-h-7 items-center justify-center rounded border border-line px-2.5 py-1 text-ink hover:border-primary hover:text-primary";
  if (onClick) {
    return (
      <button
        className={className}
        disabled={disabled}
        onClick={onClick}
        type="button"
      >
        {label}
      </button>
    );
  }
  return (
    <a
      aria-disabled={disabled ? "true" : undefined}
      className={className}
      href={href ?? "#"}
    >
      {label}
    </a>
  );
}
