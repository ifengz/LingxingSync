"use client";

import type { FormEvent, ReactNode } from "react";
import { usePathname, useRouter, useSearchParams } from "next/navigation";
import {
  Activity,
  ArrowLeftRight,
  BarChart3,
  CalendarClock,
  CalendarDays,
  Check,
  ChevronDown,
  ChevronRight,
  Database,
  HelpCircle,
  History,
  Loader2,
  PlayCircle,
  RefreshCw,
  Save,
} from "lucide-react";
import { useCallback, useEffect, useMemo, useRef, useState } from "react";
import { isProjectedActiveSyncRunStatus } from "@polabel2/sync-engine/src/sync-status";
import {
  DateRangePicker,
  type DateRangePresetKey,
  type DateRangeValue,
} from "@polabel2/ui";
import { toast } from "sonner";

import { PaginationStatusBar } from "@/components/global/pagination";
import { SearchInput } from "@/components/global/search-input";
import { SortSelect } from "@/components/global/sort-select";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { SelectNative } from "@/components/ui/select";
import {
  Tooltip,
  TooltipContent,
  TooltipProvider,
  TooltipTrigger,
} from "@/components/ui/tooltip";

import {
  listSyncOverviewRowsAction,
  listSyncRunPageAction,
  reportCancelUnavailable,
  reportEndpointLimitSaveUnavailable,
  reportRetryUnavailable,
  reportRuntimeConfigSaveUnavailable,
  reportScheduleSaveUnavailable,
  showSyncActionUnavailable,
} from "../ui-action-adapter";
import type {
  ChannelRow,
  DataSourceRow,
  LingxingEndpointLimitRow,
  SegmentRow,
  SyncOverviewSourceKey,
  SyncOverviewStoreRow,
  SyncRunRow,
  SyncScheduleRow,
} from "../ui-types";
import type { SyncRuntimeConfigRow } from "../runtime-config";
import type { SyncWorkerHealthSnapshot } from "../ui-types";
import { SyncEventsClient } from "../sync-events-client";
import { CoverageMatrixDialog } from "./coverage-matrix-dialog";

type ManualSyncNotice = { message: string; lines: string[] };
type SyncLoadNotice = { message: string; lines: string[] };
type SyncRunReadFailureDetails = { correlationId: string; digest?: string };
type SyncRunActionKind = "cancel" | "retry";

type WorkspaceSchedule = SyncScheduleRow & {
  dataSourceLabel: string;
  presetHref: string;
  strategyLabel: string;
};

type ScheduleDateWindowPreset =
  | "today"
  | "yesterday"
  | "last3"
  | "last7"
  | "last30";

type RunScope = Record<string, unknown> & {
  dataSourceId?: string;
  dataSourceLabel?: string;
  storeId?: string;
  storeIds?: string[];
  stores?: string | string[];
  channelType?: string;
  startDate?: string;
  endDate?: string;
  start?: string;
  end?: string;
  start_at?: string;
  end_at?: string;
  chunkLabel?: string;
  month?: string;
  windowLabel?: string;
  mode?: string;
  triggerSource?: string;
  triggeredBy?: unknown;
};

type SyncRunView = SyncRunRow & {
  parent_run_id?: string | null;
  updated_at?: string | null;
  target_scope_json: unknown;
};

type ParentRunGroup = {
  run: SyncRunView;
  scope: RunScope;
  children: SyncRunView[];
  segments: SegmentRow[];
  childStatusSummary: Array<{ status: string; count: number }>;
  rowCount: number;
  rowEvidence: RowEvidenceSummary;
  debugText: string;
  debugTone: ReasonTone;
};

type RowEvidenceSummary = {
  written: number;
  matched: number;
  staged: number;
  fetched: number;
  promoted: number;
  inserted: number;
  updated: number;
  unchanged: number;
  deleted: number;
  classified: number;
  generic: number;
  primary: number;
  atomCount: number;
  successAtoms: number;
  attemptCount: number;
  successAttempts: number;
  rowEvidenceAttempts: number;
  evidenceSegments: number;
  fetchedEvidenceSegments: number;
  writtenEvidenceSegments: number;
  changeSummarySegments: number;
  incompleteChangeSummarySegments: number;
  hasRowEvidence: boolean;
  fetchedKnown: boolean;
  writtenKnown: boolean;
  changeSummaryAvailable: boolean;
};

type ReasonTone = "danger" | "warning" | "info" | "neutral";

type StoreLabel = {
  id: string | null;
  name: string;
  text: string;
};

function wait(ms: number) {
  return new Promise((resolve) => setTimeout(resolve, ms));
}

function syncRunActionIdsFromForm(form: HTMLFormElement) {
  return Array.from(
    new Set(
      new FormData(form)
        .getAll("runId")
        .map((value) => String(value))
        .filter(Boolean),
    ),
  );
}

async function submitSyncRunActions(
  runIds: string[],
  action: SyncRunActionKind,
  failureMessage: string,
) {
  const runAction =
    action === "cancel"
      ? (formData: FormData) => reportCancelUnavailable(formData)
      : (formData: FormData) => reportRetryUnavailable(formData);
  const results = await Promise.allSettled(
    runIds.map((runId) => {
      const formData = new FormData();
      formData.set("runId", runId);
      return runAction(formData);
    }),
  );
  return results
    .map((result, index) =>
      result.status === "rejected"
        ? `${runIds[index]}: ${formatErrorMessage(result.reason, failureMessage)}`
        : null,
    )
    .filter((line): line is string => Boolean(line));
}

const SYNC_RUNS_REFRESH_TIMEOUT_MS = 8_000;

type StoreLabelMap = Map<string, StoreLabel>;

type ManualSyncSubmitInput = {
  selectedSyncTypes: string[];
  dataSourceId: string;
  channelType: string;
  selectedStores: string[];
  dateRange: DateRangeValue;
  fromParam?: string;
  profileParam?: string;
};

const SYNC_TYPE_LABEL: Record<string, string> = {
  "sync:stores": "集成连接 · 店铺目录",
  "sync:pairing": "产品总览 · 配对",
  "sync:products": "产品总览 · 主数据",
  "sync:fba-links": "FBA 链接",
  "sync:fba-listings": "FBA 链接 · Listing",
  "sync:fba-listing-cache": "FBA 链接 · Listing",
  "sync:sc-sales": "FBA 链接 · SC 销量",
  "sync:sc-ads": "FBA 链接 · SC 广告",
  "sync:sc-performance": "FBA 链接 · 表现快照",
  "sync:sc-returns": "FBA 链接 · SC 退货",
  "sync:sc-ads-hsa-creative-mapping": "FBA 链接 · HSA 映射",
  "sync:sc-ads-account": "FBA 链接 · 账户广告",
  "sync:sc-inventory": "FBA 链接 · SC 库存",
  "sync:vc-sales": "VC 链接 · 销量",
  "sync:vc-traffic": "VC 链接 · 访问量",
  "sync:vc-inventory": "VC 链接 · 库存",
  "sync:vc-realtime": "VC 链接 · 实时销量",
  "sync:vc-margin": "VC 链接 · 利润",
  "sync:vc-ads": "VC 链接 · 广告",
  "sync:vc-links": "VC 链接",
  "sync:operations-log-vc": "运营日志 · VC 数据",
  "sync:operations-log-sc": "运营日志 · SC 数据",
  "sync:po-orders": "PO 订单 · VC 订单",
  "sync:df-orders": "DF 订单 · VC 订单",
  "sync:vc-invoices": "VC 链接 · 发货单",
};

const SYNC_PRESETS = [
  {
    value: "sync:stores",
    label: "集成连接 · 店铺目录",
    category: "integration",
    needsStores: false,
    needsDates: false,
    defaultRange: "7d",
  },
  {
    value: "sync:products",
    label: "产品总览 · 主数据",
    category: "store",
    needsStores: false,
    needsDates: false,
    defaultRange: "7d",
  },
  {
    value: "sync:pairing",
    label: "产品总览 · 配对",
    category: "store",
    needsStores: false,
    needsDates: false,
    defaultRange: "7d",
  },
  {
    value: "sync:fba-listings",
    label: "FBA 链接 · Listing",
    category: "store",
    needsStores: true,
    needsDates: false,
    defaultRange: "7d",
  },
  {
    value: "sync:sc-sales",
    label: "FBA 链接 · SC 销量",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "7d",
  },
  {
    value: "sync:sc-returns",
    label: "FBA 链接 · SC 退货",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "7d",
  },
  {
    value: "sync:sc-ads",
    label: "FBA 链接 · SC 广告",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "7d",
  },
  {
    value: "sync:sc-performance",
    label: "FBA 链接 · 表现快照",
    category: "store",
    needsStores: true,
    needsDates: false,
    defaultRange: "1d",
  },
  {
    value: "sync:sc-ads-hsa-creative-mapping",
    label: "FBA 链接 · HSA 映射",
    category: "store",
    needsStores: true,
    needsDates: false,
    defaultRange: "7d",
  },
  {
    value: "sync:sc-ads-account",
    label: "FBA 链接 · 账户广告",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "7d",
  },
  {
    value: "sync:sc-inventory",
    label: "FBA 链接 · SC 库存",
    category: "store",
    needsStores: true,
    needsDates: false,
    defaultRange: "1d",
  },
  {
    value: "sync:vc-sales",
    label: "VC 链接 · 销量",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "30d",
  },
  {
    value: "sync:vc-traffic",
    label: "VC 链接 · 访问量",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "30d",
  },
  {
    value: "sync:vc-inventory",
    label: "VC 链接 · 库存",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "30d",
  },
  {
    value: "sync:vc-realtime",
    label: "VC 链接 · 实时销量",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "7d",
  },
  {
    value: "sync:vc-margin",
    label: "VC 链接 · 利润",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "30d",
  },
  {
    value: "sync:vc-ads",
    label: "VC 链接 · 广告",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "30d",
  },
  {
    value: "sync:po-orders",
    label: "PO 订单 · VC 订单",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "30d",
  },
  {
    value: "sync:df-orders",
    label: "DF 订单 · VC 订单",
    category: "store",
    needsStores: true,
    needsDates: true,
    defaultRange: "30d",
  },
  {
    value: "sync:vc-invoices",
    label: "VC 链接 · 发货单",
    category: "store",
    needsStores: true,
    needsDates: false,
    defaultRange: "30d",
  },
] as const;

const SYNC_TYPE_GROUPS = [
  { label: "集成连接", types: ["sync:stores"] },
  { label: "产品总览", types: ["sync:products", "sync:pairing"] },
  {
    label: "FBA 链接",
    types: [
      "sync:fba-listings",
      "sync:sc-sales",
      "sync:sc-returns",
      "sync:sc-ads",
      "sync:sc-performance",
      "sync:sc-ads-hsa-creative-mapping",
      "sync:sc-ads-account",
      "sync:sc-inventory",
    ],
  },
  {
    label: "VC 链接",
    types: [
      "sync:vc-sales",
      "sync:vc-traffic",
      "sync:vc-inventory",
      "sync:vc-realtime",
      "sync:vc-margin",
      "sync:vc-ads",
      "sync:vc-invoices",
    ],
  },
  { label: "PO 订单", types: ["sync:po-orders"] },
  { label: "DF 订单", types: ["sync:df-orders"] },
];

const SCHEDULE_FILTERS = [
  { value: "all", label: "全部" },
  { value: "fba", label: "FBA 链接" },
  { value: "vc", label: "VC 链接" },
  { value: "operations", label: "运营日志" },
  { value: "orders", label: "订单" },
  { value: "foundation", label: "产品基础" },
] as const;

const SCHEDULE_DATE_WINDOW_PRESETS: Array<{
  value: ScheduleDateWindowPreset;
  label: string;
}> = [
  { value: "today", label: "今天 1 天" },
  { value: "yesterday", label: "昨天 1 天" },
  { value: "last3", label: "近 3 天" },
  { value: "last7", label: "近 7 天" },
  { value: "last30", label: "近 30 天" },
];

const BUSINESS_SYNC_SCHEDULE_PROVIDERS = new Set(["lingxing"]);
const COMPOUND_PROFILE_SYNC_TYPES = new Set([
  "sync:fba-links",
  "sync:vc-links",
  "sync:operations-log-vc",
  "sync:operations-log-sc",
]);

type SyncTab = "overview" | "manual" | "runs" | "schedules" | "sources";

const SYNC_TABS: Array<{
  value: SyncTab;
  label: string;
  icon: ReactNode;
  count?: keyof SyncOverview;
}> = [
  { value: "overview", label: "概览", icon: <Activity className="h-4 w-4" /> },
  {
    value: "manual",
    label: "手动同步",
    icon: <PlayCircle className="h-4 w-4" />,
  },
  {
    value: "schedules",
    label: "预设计划",
    icon: <CalendarClock className="h-4 w-4" />,
    count: "schedules",
  },
  { value: "runs", label: "同步日志", icon: <History className="h-4 w-4" /> },
  { value: "sources", label: "数据源", icon: <Database className="h-4 w-4" /> },
];

type SyncOverview = {
  running: number;
  success: number;
  error: number;
  schedules: number;
};

const OVERVIEW_SOURCES: Array<{ key: SyncOverviewSourceKey; label: string }> = [
  { key: "self", label: "自营领星" },
  { key: "affiliate", label: "联营领星" },
  { key: "spotterio", label: "Spotter" },
];

export function SyncWorkspace({
  initialRuns,
  initialSegments,
  initialRunsLoaded,
  initialRunPage,
  initialRunPageSize,
  initialRunHasNext,
  initialDataSources,
  initialChannels,
  initialSchedules,
  initialOverviewRows,
  initialEndpointLimitRows,
  initialRuntimeConfigRows,
  initialWorkerHealth,
  canManageEndpointLimits,
  initialLoadErrors,
}: {
  initialRuns: SyncRunRow[];
  initialSegments: SegmentRow[];
  initialRunsLoaded?: boolean;
  initialRunPage?: number;
  initialRunPageSize?: number;
  initialRunHasNext?: boolean;
  initialDataSources: DataSourceRow[];
  initialChannels?: ChannelRow[];
  initialSchedules: SyncScheduleRow[];
  initialOverviewRows?: SyncOverviewStoreRow[];
  initialEndpointLimitRows?: LingxingEndpointLimitRow[];
  initialRuntimeConfigRows?: SyncRuntimeConfigRow[];
  initialWorkerHealth?: SyncWorkerHealthSnapshot | null;
  canManageEndpointLimits?: boolean;
  initialLoadErrors?: string[];
}) {
  const pathname = usePathname() ?? "/admin/sync";
  const searchParams = useSearchParams();
  const initialRunSearch = searchParams?.get("q") ?? "";
  const initialRunningOnly = searchParams?.get("runningOnly") === "1";
  const initialRunStatus = normalizeRunStatusFilter(
    searchParams?.get("runStatus") ?? null,
  );
  const initialRunType = normalizeRunTypeFilter(
    searchParams?.get("runType") ?? null,
  );
  const [runs, setRuns] = useState<SyncRunView[]>(coerceRuns(initialRuns));
  const [segments, setSegments] = useState(initialSegments);
  const [runsLoaded, setRunsLoaded] = useState(Boolean(initialRunsLoaded));
  const [runPage, setRunPage] = useState(initialRunPage ?? 1);
  const [runPageSize, setRunPageSize] = useState(initialRunPageSize ?? 50);
  const [runHasNext, setRunHasNext] = useState(Boolean(initialRunHasNext));
  const [overviewRows, setOverviewRows] = useState(() =>
    Array.isArray(initialOverviewRows) ? initialOverviewRows : [],
  );
  const [loadNotice, setLoadNotice] = useState<SyncLoadNotice | null>(() =>
    initialLoadErrors?.length
      ? { message: "同步数据读取失败", lines: initialLoadErrors }
      : null,
  );
  const [dataSources] = useState(initialDataSources);
  const [channels] = useState(initialChannels ?? []);
  const [search, setSearch] = useState(initialRunSearch);
  const [runningOnly, setRunningOnly] = useState(initialRunningOnly);
  const [statusFilter, setStatusFilter] = useState(initialRunStatus);
  const [typeFilter, setTypeFilter] = useState(initialRunType);
  const loadedRunQueryKeyRef = useRef(
    initialRunsLoaded
      ? buildRunLogQueryKey({
          page: initialRunPage ?? 1,
          pageSize: initialRunPageSize ?? 50,
          runningOnly: initialRunningOnly,
          search: initialRunSearch,
          status: initialRunStatus,
          type: initialRunType,
        })
      : "",
  );
  const [batchCancelling, setBatchCancelling] = useState(false);
  const [batchRetrying, setBatchRetrying] = useState(false);
  const [overviewSource, setOverviewSource] =
    useState<SyncOverviewSourceKey>("self");
  const [coverageRow, setCoverageRow] = useState<SyncOverviewStoreRow | null>(
    null,
  );
  const initialTab = searchParams ? searchParams.get("tab") : null;
  const [activeTab, setActiveTab] = useState<SyncTab>(() =>
    searchParams?.get("type") || searchParams?.get("from")
      ? "manual"
      : normalizeTab(initialTab),
  );

  const handleTabChange = useCallback(
    (tab: SyncTab) => {
      setActiveTab(tab);
      let next = new URLSearchParams();
      if (tab === "runs") {
        next = buildRunLogParams({
          page: runPage,
          pageSize: runPageSize,
          runningOnly,
          search,
          status: statusFilter,
          type: typeFilter,
        });
      } else if (tab === "overview") {
        // no params needed
      } else {
        next.set("tab", tab);
      }
      const query = next.toString();
      const nextUrl = query ? `${pathname}?${query}` : pathname;
      window.history.replaceState(null, "", nextUrl);
    },
    [
      pathname,
      runPage,
      runPageSize,
      runningOnly,
      search,
      statusFilter,
      typeFilter,
    ],
  );

  const [refreshing, setRefreshing] = useState(false);
  const [runsRefreshing, setRunsRefreshing] = useState(false);
  const [runsLoadFailed, setRunsLoadFailed] = useState(false);
  const runLogRequestSeqRef = useRef(0);

  const refreshRuns = useCallback(async () => {
    const requestSeq = runLogRequestSeqRef.current + 1;
    runLogRequestSeqRef.current = requestSeq;
    const requestedQuery = {
      page: runPage,
      pageSize: runPageSize,
      runningOnly,
      search,
      status: statusFilter,
      type: typeFilter,
    };
    try {
      const result = await listSyncRunPageAction(requestedQuery);
      if (runLogRequestSeqRef.current !== requestSeq) return;
      if (!result.ok) throw new Error(formatSyncRunReadFailure(result.error));
      setRuns(coerceRuns(result.data.runs));
      setSegments(result.data.segments);
      setRunHasNext(result.data.hasNext);
      setRunPage(result.data.page);
      setRunPageSize(result.data.pageSize);
      setRunsLoaded(true);
      setRunsLoadFailed(false);
      loadedRunQueryKeyRef.current = buildRunLogQueryKey({
        ...requestedQuery,
        page: result.data.page,
        pageSize: result.data.pageSize,
      });
      setLoadNotice(null);
    } catch (error) {
      if (runLogRequestSeqRef.current !== requestSeq) return;
      setRunsLoaded(true);
      setRunsLoadFailed(true);
      loadedRunQueryKeyRef.current = buildRunLogQueryKey(requestedQuery);
      setLoadNotice({
        message: "同步数据读取失败",
        lines: [formatErrorMessage(error, "同步日志读取失败")],
      });
    }
  }, [runPage, runPageSize, runningOnly, search, statusFilter, typeFilter]);

  const refresh = useCallback(async () => {
    setRefreshing(true);
    try {
      const [overviewResult] = await Promise.allSettled([
        listSyncOverviewRowsAction(),
        refreshRuns(),
      ]);
      if (overviewResult.status === "fulfilled") {
        setOverviewRows(overviewResult.value);
      } else {
        setLoadNotice({
          message: "同步数据读取失败",
          lines: [
            settledFailureLine(overviewResult, "覆盖矩阵读取失败") ??
              "覆盖矩阵读取失败",
          ],
        });
      }
    } finally {
      setRefreshing(false);
    }
  }, [refreshRuns]);

  const refreshLogRuns = useCallback(async () => {
    setRunsRefreshing(true);
    try {
      const refreshTask = Promise.all([refreshRuns(), wait(500)]);
      const timeoutTask = wait(SYNC_RUNS_REFRESH_TIMEOUT_MS).then(() => {
        throw new Error("同步日志刷新超时，请重试");
      });
      await Promise.race([refreshTask, timeoutTask]);
    } catch (error) {
      runLogRequestSeqRef.current += 1;
      setRunsLoaded(true);
      setRunsLoadFailed(true);
      loadedRunQueryKeyRef.current = buildRunLogQueryKey({
        page: runPage,
        pageSize: runPageSize,
        runningOnly,
        search,
        status: statusFilter,
        type: typeFilter,
      });
      setLoadNotice({
        message: "同步数据读取失败",
        lines: [formatErrorMessage(error, "同步日志刷新失败")],
      });
    } finally {
      setRunsRefreshing(false);
    }
  }, [
    refreshRuns,
    runPage,
    runPageSize,
    runningOnly,
    search,
    statusFilter,
    typeFilter,
  ]);

  const applySyncSnapshot = useCallback(
    (payload: {
      hasNext?: boolean;
      runs?: unknown[];
      segments?: unknown[];
    }) => {
      if (
        runPage !== 1 ||
        search ||
        statusFilter !== "all" ||
        typeFilter !== "all"
      )
        return;
      if (runningOnly) {
        void refreshRuns();
        return;
      }
      if (Array.isArray(payload.runs)) {
        const snapshotRuns = payload.runs;
        setRuns(coerceRuns(snapshotRuns));
        setRunHasNext(Boolean(payload.hasNext));
        setRunsLoaded(true);
        loadedRunQueryKeyRef.current = buildRunLogQueryKey({
          page: 1,
          pageSize: runPageSize,
          runningOnly: false,
          search: "",
          status: "all",
          type: "all",
        });
      }
      if (Array.isArray(payload.segments)) {
        setSegments(payload.segments as SegmentRow[]);
      }
    },
    [
      refreshRuns,
      runPage,
      runPageSize,
      runningOnly,
      search,
      statusFilter,
      typeFilter,
    ],
  );

  const handleSyncEventsError = useCallback(
    (message: string) => {
      runLogRequestSeqRef.current += 1;
      setRunsLoaded(true);
      setRunsLoadFailed(true);
      loadedRunQueryKeyRef.current = buildRunLogQueryKey({
        page: runPage,
        pageSize: runPageSize,
        runningOnly,
        search,
        status: statusFilter,
        type: typeFilter,
      });
      setLoadNotice({ message: "同步实时事件异常", lines: [message] });
    },
    [runPage, runPageSize, runningOnly, search, statusFilter, typeFilter],
  );

  useEffect(() => {
    if (activeTab !== "runs" || runsLoaded || runsRefreshing) return;
    void refreshLogRuns();
  }, [activeTab, refreshLogRuns, runsLoaded, runsRefreshing]);

  useEffect(() => {
    if (activeTab !== "runs" || !runsLoaded || runsRefreshing) return;
    const queryKey = buildRunLogQueryKey({
      page: runPage,
      pageSize: runPageSize,
      runningOnly,
      search,
      status: statusFilter,
      type: typeFilter,
    });
    if (loadedRunQueryKeyRef.current === queryKey) return;
    void refreshRuns();
  }, [
    activeTab,
    refreshRuns,
    runPage,
    runPageSize,
    runningOnly,
    runsLoaded,
    runsRefreshing,
    search,
    statusFilter,
    typeFilter,
  ]);

  useEffect(() => {
    if (activeTab !== "runs" || !runsLoaded) return;
    const query = buildRunLogParams({
      page: runPage,
      pageSize: runPageSize,
      runningOnly,
      search,
      status: statusFilter,
      type: typeFilter,
    }).toString();
    window.history.replaceState(null, "", `${pathname}?${query}`);
  }, [
    activeTab,
    pathname,
    runPage,
    runPageSize,
    runningOnly,
    runsLoaded,
    search,
    statusFilter,
    typeFilter,
  ]);

  const hydratedSchedules = useMemo(
    () =>
      initialSchedules
        .filter(
          (schedule) =>
            !["test:connection", "sync:stores", "refresh:erp-token"].includes(
              schedule.task_type,
            ),
        )
        .filter((schedule) =>
          supportsBusinessSyncSchedules(
            dataSources.find((item) => item.id === schedule.data_source_id),
          ),
        )
        // 联营 Pairing 的历史 disabled schedule 保留在 DB；同源 active products=0，未经用户授权不得删除或恢复入口。
        .filter(
          (schedule) =>
            schedule.task_type !== "sync:pairing" ||
            isDefaultSelfPairingSource(
              dataSources.find((item) => item.id === schedule.data_source_id),
            ),
        )
        .map((schedule) => {
          const ds = dataSources.find(
            (item) => item.id === schedule.data_source_id,
          );
          return {
            ...schedule,
            dataSourceLabel: ds?.label ?? schedule.data_source_id,
            presetHref: buildSyncCenterHref({
              type: schedule.task_type,
              dataSourceId: schedule.data_source_id,
              range: schedulePresetToManualRange(
                readScheduleDateWindowPreset(schedule),
              ),
            }),
            strategyLabel: describeScheduleStrategy(
              schedule.task_type,
              schedule.cron_expr,
              readScheduleDateWindowPreset(schedule),
            ),
          };
        }),
    [dataSources, initialSchedules],
  );

  const groups = useMemo(
    () => buildParentGroups(runs, segments),
    [runs, segments],
  );
  const storeLabels = useMemo(
    () => buildStoreLabelMap(overviewRows, channels),
    [channels, overviewRows],
  );
  const activeGroups = groups.filter((group) =>
    isActiveStatus(group.run.status),
  );
  // listSyncRunPageAction already applies the active filters before pagination.
  const filteredLogGroups = groups;
  const batchCancelRunIds = runActionIdsForGroups(filteredLogGroups, "cancel");
  const batchRetryRunIds = runActionIdsForGroups(filteredLogGroups, "retry");

  const overview = useMemo(
    () => ({
      running: activeGroups.length,
      success: groups.filter((group) => group.run.status === "success").length,
      error: groups.filter((group) =>
        ["error", "stale", "cancelled"].includes(group.run.status),
      ).length,
      schedules: hydratedSchedules.filter((schedule) => schedule.enabled)
        .length,
    }),
    [activeGroups.length, groups, hydratedSchedules],
  );
  const visibleOverviewRows = overviewRows.filter(
    (row) => row.source_key === overviewSource,
  );

  async function handleBatchRunAction(
    event: FormEvent<HTMLFormElement>,
    action: SyncRunActionKind,
  ) {
    event.preventDefault();
    const runIds = syncRunActionIdsFromForm(event.currentTarget);
    if (!runIds.length) return;
    const setBusy = action === "cancel" ? setBatchCancelling : setBatchRetrying;
    const successMessage =
      action === "cancel"
        ? `已提交 ${runIds.length} 个取消请求`
        : `已提交 ${runIds.length} 个重试请求`;
    const failureMessage =
      action === "cancel" ? "批量取消部分失败" : "批量重试部分失败";
    setBusy(true);
    try {
      const failures = await submitSyncRunActions(
        runIds,
        action,
        failureMessage,
      );
      await refreshRuns();
      if (failures.length) {
        setLoadNotice({ message: failureMessage, lines: failures });
        return;
      }
      toast.success(successMessage);
    } finally {
      setBusy(false);
    }
  }

  return (
    <section className="flex h-screen min-h-0 flex-col">
      <SyncLoadNoticeBanner
        notice={loadNotice}
        onClose={() => setLoadNotice(null)}
      />
      <header className="products-module-header">
        <div className="products-title-row">
          <div className="flex min-w-0 flex-1 items-center gap-4">
            <div className="products-title-lockup shrink-0">
              <span className="products-title-icon">
                <ArrowLeftRight />
              </span>
              <h1>同步中心</h1>
            </div>
            <TabStrip
              activeTab={activeTab}
              overview={overview}
              onChange={handleTabChange}
            />
          </div>
          <div className="flex shrink-0 items-center gap-2 text-sm text-ink-sub">
            <SyncEventsClient
              active={
                activeTab === "runs" && !runsRefreshing && !runsLoadFailed
              }
              onError={handleSyncEventsError}
              onSnapshot={applySyncSnapshot}
            />
          </div>
        </div>
      </header>

      <main className="min-h-0 flex-1 p-5">
        <div className="h-full min-h-0 overflow-hidden rounded-lg border border-line bg-white">
          {activeTab === "overview" ? (
            <section
              className="flex h-full min-h-0 flex-col"
              data-testid="sync-overview-panel"
            >
              <OverviewSourceTabs
                activeSource={overviewSource}
                onChange={setOverviewSource}
                rows={overviewRows}
              />
              <section className="flex h-full min-h-0 flex-col">
                <SectionHeader
                  icon={
                    overviewSource === "spotterio" ? (
                      <BarChart3 className="h-4 w-4" />
                    ) : (
                      <Database className="h-4 w-4" />
                    )
                  }
                  title={
                    overviewSource === "spotterio"
                      ? "Spotter 同步历史"
                      : "店铺同步历史"
                  }
                  description="按来源和店铺展示已同步的历史范围、事实表数量和最近同步时间。"
                  actions={
                    <Button
                      type="button"
                      variant="default"
                      size="sm"
                      onClick={() => void refresh()}
                      disabled={refreshing}
                    >
                      <RefreshCw
                        className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`}
                      />
                      {refreshing ? "刷新中…" : "刷新"}
                    </Button>
                  }
                />
                <OverviewTable
                  rows={visibleOverviewRows}
                  source={overviewSource}
                  onOpenCoverage={setCoverageRow}
                />
                <CoverageMatrixDialog
                  row={coverageRow}
                  open={Boolean(coverageRow)}
                  onOpenChange={(open) => {
                    if (!open) setCoverageRow(null);
                  }}
                />
              </section>
            </section>
          ) : null}

          {activeTab === "manual" ? (
            <TriggerSyncPanel
              dataSources={dataSources}
              channels={channels}
              searchParams={searchParams}
              refreshing={refreshing}
              onRefresh={refresh}
            />
          ) : null}

          {activeTab === "runs" ? (
            <section
              className="relative flex h-full min-h-0 flex-col"
              data-testid="history-runs-panel"
              aria-busy={runsRefreshing}
            >
              {runsRefreshing ? (
                <div
                  className="pointer-events-none absolute inset-0 z-20 flex items-start justify-center bg-white/45 pt-12 backdrop-blur-[1px]"
                  data-testid="sync-runs-panel-refreshing"
                >
                  <div className="inline-flex items-center gap-3 rounded-lg border border-line bg-white px-5 py-3 text-base font-extrabold text-slate-800 shadow-lg shadow-slate-900/10">
                    <RefreshCw className="h-5 w-5 animate-spin text-primary" />
                    主表刷新中...
                  </div>
                </div>
              ) : null}
              <SectionHeader
                icon={<History className="h-4 w-4" />}
                title="同步日志"
                description="按批次展示最近同步，可查看子任务、拉取明细与完成情况。batch 精确搜索实时；店铺与状态模糊搜索基于最近 200 条日志。颜色说明：红色=已经失败/数据不完整/异常终止；橙/黄色=无可同步内容/等待用户判断/子任务进行中/可重试但不一定坏；蓝色=正在处理；绿色=已完成；灰色=无需处理/已取消/无数据但正常。"
                controls={
                  <>
                    <SearchInput
                      value={search}
                      onChange={(value) => {
                        setRunPage(1);
                        setSearch(value);
                      }}
                      placeholder="搜索店铺 / 状态 / batch"
                    />
                    <SortSelect
                      value={statusFilter}
                      onChange={(value) => {
                        setRunPage(1);
                        setStatusFilter(value);
                      }}
                    >
                      <option value="all">全部状态</option>
                      <option value="active">进行中</option>
                      <option value="success">成功</option>
                      <option value="error">失败</option>
                      <option value="cancelled">已取消</option>
                    </SortSelect>
                    <SortSelect
                      value={typeFilter}
                      onChange={(value) => {
                        setRunPage(1);
                        setTypeFilter(value);
                      }}
                    >
                      <option value="all">全部类型</option>
                      {SYNC_PRESETS.map((preset) => (
                        <option key={preset.value} value={preset.value}>
                          {preset.label}
                        </option>
                      ))}
                    </SortSelect>
                    <label
                      className="inline-flex h-8 shrink-0 items-center gap-2 rounded-md px-1 text-sm font-extrabold text-slate-700"
                      title="只显示仍在 running 的父任务或子任务"
                    >
                      <input
                        aria-label="进行中任务"
                        checked={runningOnly}
                        className="h-4 w-4 rounded border-slate-300 text-primary focus:ring-primary/30"
                        onChange={(event) => {
                          setRunPage(1);
                          setRunningOnly(event.currentTarget.checked);
                        }}
                        type="checkbox"
                      />
                      <span className="whitespace-nowrap">进行中任务</span>
                    </label>
                  </>
                }
                actions={
                  <>
                    <form
                      data-testid="sync-runs-batch-cancel-form"
                      onSubmit={(event) =>
                        void handleBatchRunAction(event, "cancel")
                      }
                      className="inline-flex"
                    >
                      {batchCancelRunIds.map((runId) => (
                        <input
                          key={runId}
                          type="hidden"
                          name="runId"
                          value={runId}
                        />
                      ))}
                      <Button
                        type="submit"
                        variant="outline"
                        size="sm"
                        disabled={
                          batchCancelling || batchCancelRunIds.length === 0
                        }
                        title="取消当前筛选下正在执行的同步批次"
                        data-testid="sync-runs-batch-cancel-button"
                      >
                        {batchCancelling ? "取消中…" : "一键取消"}
                      </Button>
                    </form>
                    <form
                      data-testid="sync-runs-batch-retry-form"
                      onSubmit={(event) =>
                        void handleBatchRunAction(event, "retry")
                      }
                      className="inline-flex"
                    >
                      {batchRetryRunIds.map((runId) => (
                        <input
                          key={runId}
                          type="hidden"
                          name="runId"
                          value={runId}
                        />
                      ))}
                      <Button
                        type="submit"
                        variant="outline"
                        size="sm"
                        disabled={
                          batchRetrying || batchRetryRunIds.length === 0
                        }
                        title="重试当前筛选下失败、取消或过期的同步批次"
                        data-testid="sync-runs-batch-retry-button"
                      >
                        {batchRetrying ? "重试中…" : "一键重试"}
                      </Button>
                    </form>
                    <Button
                      type="button"
                      variant="outline"
                      size="sm"
                      className="min-w-[72px]"
                      onClick={() => void refreshLogRuns()}
                      disabled={runsRefreshing}
                      aria-busy={runsRefreshing}
                    >
                      <RefreshCw
                        className={`h-4 w-4 ${runsRefreshing ? "animate-spin" : ""}`}
                      />
                      刷新
                    </Button>
                    <span className="shrink-0 text-xs text-ink-sub">
                      第 {runPage} 页 · {runHasNext ? "还有下一页" : "已到底"}
                    </span>
                  </>
                }
              />
              <DenseRunTable
                groups={filteredLogGroups}
                storeLabels={storeLabels}
                hasNext={runHasNext}
                page={runPage}
                pageSize={runPageSize}
                onPageChange={setRunPage}
                onPageSizeChange={(size) => {
                  setRunPage(1);
                  setRunPageSize(size);
                }}
                onRefresh={refreshRuns}
                onLocalError={(message, error) =>
                  setLoadNotice({
                    message,
                    lines: [formatErrorMessage(error, message)],
                  })
                }
                runningOnly={runningOnly}
              />
            </section>
          ) : null}

          {activeTab === "schedules" ? (
            <SchedulePanel
              schedules={hydratedSchedules}
              channels={channels}
              searchParams={searchParams}
              pathname={pathname}
            />
          ) : null}

          {activeTab === "sources" ? (
            <DataSourcePanel
              dataSources={dataSources}
              workerHealth={initialWorkerHealth ?? null}
              runtimeConfigRows={initialRuntimeConfigRows ?? []}
              endpointLimitRows={initialEndpointLimitRows ?? []}
              canManageEndpointLimits={Boolean(canManageEndpointLimits)}
            />
          ) : null}
        </div>
      </main>
    </section>
  );
}

function TriggerSyncPanel({
  dataSources,
  channels,
  searchParams,
  refreshing,
  onRefresh,
}: {
  dataSources: DataSourceRow[];
  channels: ChannelRow[];
  searchParams: ReturnType<typeof useSearchParams>;
  refreshing: boolean;
  onRefresh: () => void;
}) {
  const pathname = usePathname() ?? "/admin/sync";
  const resolvedSearchParams = searchParams ?? new URLSearchParams();
  const initialSyncTypes = normalizeSyncTypes(resolvedSearchParams.get("type"));
  const initialType = initialSyncTypes[0] ?? "sync:sc-sales";
  const initialDataSourceId = resolveDataSourceId(
    dataSources,
    resolvedSearchParams.get("dataSourceId"),
  );
  const [selectedSyncTypes, setSelectedSyncTypes] = useState(initialSyncTypes);
  const [datePreset, setDatePreset] = useState<DateRangePresetKey>(
    toDateRangePreset(
      normalizeRangePreset(resolvedSearchParams.get("range"), initialType),
    ),
  );
  const [dateRange, setDateRange] = useState<DateRangeValue>(
    resolveDateRange(
      normalizeRangePreset(resolvedSearchParams.get("range"), initialType),
      resolvedSearchParams.get("start"),
      resolvedSearchParams.get("end"),
    ),
  );
  const [dataSourceId, setDataSourceId] = useState(initialDataSourceId);
  const [selectedStores, setSelectedStores] = useState(() =>
    parseStoreIds(
      resolvedSearchParams.get("stores") ??
        resolvedSearchParams.get("store") ??
        "",
    ),
  );
  const [storeSearch, setStoreSearch] = useState("");
  const [submitting, setSubmitting] = useState(false);
  const [manualSyncNotice, setManualSyncNotice] =
    useState<ManualSyncNotice | null>(null);
  const fromParam = resolvedSearchParams.get("from") ?? "";
  const profileParam = resolvedSearchParams.get("profile") ?? "";

  const selectedPresets = selectedSyncTypes
    .map((type) => SYNC_PRESETS.find((item) => item.value === type))
    .filter(Boolean) as Array<(typeof SYNC_PRESETS)[number]>;
  const primarySyncType = selectedSyncTypes[0] ?? "sync:sc-sales";
  const needsStores = selectedPresets.some((item) => item.needsStores);
  const needsDates = selectedPresets.some((item) => item.needsDates);
  const visibleChannels = useMemo(
    () =>
      channels.filter(
        (channel) =>
          Number(channel.enabled) === 1 &&
          channel.data_source_id === dataSourceId &&
          selectedSyncTypes.every((syncType) =>
            channelMatchesSyncType(syncType, channel.channel_type),
          ),
      ),
    [channels, dataSourceId, selectedSyncTypes],
  );
  const visibleStoreIds = useMemo(
    () => new Set(visibleChannels.map((channel) => channel.store_id)),
    [visibleChannels],
  );
  const selectedVisibleStores = useMemo(
    () => selectedStores.filter((storeId) => visibleStoreIds.has(storeId)),
    [selectedStores, visibleStoreIds],
  );
  const selectedStoreValue = selectedVisibleStores.join(",");

  // Sync form state to URL so page refresh / server action re-render preserves selections
  useEffect(() => {
    const next = new URLSearchParams();
    next.set("tab", "manual");
    next.set("type", selectedSyncTypes.join(","));
    next.set("dataSourceId", dataSourceId);
    if (selectedStoreValue) next.set("stores", selectedStoreValue);
    if (dateRange.start) next.set("start", dateRange.start);
    if (dateRange.end) next.set("end", dateRange.end);
    if (fromParam) next.set("from", fromParam);
    if (profileParam) next.set("profile", profileParam);
    const nextUrl = `${pathname}?${next.toString()}`;
    window.history.replaceState(null, "", nextUrl);
  }, [
    selectedSyncTypes,
    dataSourceId,
    selectedStoreValue,
    dateRange,
    pathname,
    fromParam,
    profileParam,
  ]);

  // Remove the old resync effect - URL sync handles state persistence now
  // (initialType/resolvedSearchParams already set correct initial state)

  const filteredChannels = useMemo(() => {
    if (!storeSearch.trim()) return visibleChannels;
    const q = storeSearch.trim().toLowerCase();
    return visibleChannels.filter((channel) =>
      [
        channel.store_name,
        channel.store_id,
        channel.channel_type,
        channel.country,
      ]
        .filter(Boolean)
        .join(" ")
        .toLowerCase()
        .includes(q),
    );
  }, [visibleChannels, storeSearch]);
  const selectedChannelTypes = Array.from(
    new Set(
      visibleChannels
        .filter((channel) => selectedVisibleStores.includes(channel.store_id))
        .map((channel) => channel.channel_type),
    ),
  );
  const channelTypeValue =
    selectedChannelTypes.length === 1
      ? (selectedChannelTypes[0] ?? inferChannelType(primarySyncType))
      : inferChannelType(primarySyncType);
  const submitSnapshot = {
    selectedSyncTypes,
    dataSourceId,
    channelType: channelTypeValue,
    selectedStores: selectedVisibleStores,
    dateRange,
    fromParam,
    profileParam,
  };

  async function handleSubmit(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    setSubmitting(true);
    try {
      const request = buildManualSyncFormData(submitSnapshot);
      const query = buildManualSyncUrlParams(submitSnapshot);
      window.history.replaceState(null, "", `${pathname}?${query.toString()}`);
      await showSyncActionUnavailable(request);
      setManualSyncNotice(null);
      toast.success("已提交同步任务，可在同步日志查看进度");
      await onRefresh();
    } catch (error) {
      setManualSyncNotice({
        message: "同步任务提交失败",
        lines: [error instanceof Error ? error.message : "同步任务提交失败"],
      });
    } finally {
      setSubmitting(false);
    }
  }

  useEffect(() => {
    setSelectedStores((current) =>
      current.filter((storeId) =>
        visibleChannels.some((channel) => channel.store_id === storeId),
      ),
    );
  }, [dataSourceId, visibleChannels]);

  return (
    <form
      onSubmit={(event) => void handleSubmit(event)}
      className="flex h-full min-h-0 flex-col"
      data-testid="sync-trigger-panel"
    >
      <ManualSyncNoticeBanner
        notice={manualSyncNotice}
        onClose={() => setManualSyncNotice(null)}
      />
      <input
        type="hidden"
        name="syncType"
        value={selectedSyncTypes.join(",")}
      />
      <input
        type="hidden"
        name="start"
        value={needsDates ? dateRange.start : ""}
      />
      <input type="hidden" name="end" value={needsDates ? dateRange.end : ""} />
      <input
        type="hidden"
        name="stores"
        value={needsStores ? selectedStoreValue : ""}
      />
      <input type="hidden" name="channelType" value={channelTypeValue} />
      {profileParam ? (
        <input type="hidden" name="profile" value={profileParam} />
      ) : null}

      <SectionHeader
        icon={<PlayCircle className="h-4 w-4" />}
        title="手动同步"
        description="所有业务页同步入口都收口到这里，支持预填参数继续发起。"
        actions={
          <>
            <Button
              type="button"
              variant="outline"
              size="sm"
              onClick={onRefresh}
              disabled={refreshing}
            >
              <RefreshCw
                className={`h-4 w-4 ${refreshing ? "animate-spin" : ""}`}
              />
              {refreshing ? "刷新中…" : "刷新"}
            </Button>
            <Button
              type="submit"
              variant="default"
              size="sm"
              disabled={submitting}
            >
              {submitting ? <Loader2 className="h-4 w-4 animate-spin" /> : null}
              {submitting ? "提交中…" : "提交同步"}
            </Button>
          </>
        }
      />

      {/* Sync type cards — grouped by module, one row per group */}
      <div
        className="border-b border-line px-5 py-4"
        data-testid="sync-type-card-grid"
      >
        <div className="grid gap-2.5">
          {SYNC_TYPE_GROUPS.map((group) => (
            <div key={group.label} className="flex items-center gap-2">
              <span className="w-16 shrink-0 text-xs font-bold text-ink-muted">
                {group.label}
              </span>
              <div className="flex flex-wrap gap-1.5">
                {group.types
                  .filter(
                    (typeValue) =>
                      typeValue !== "sync:pairing" ||
                      isDefaultSelfPairingSource(
                        dataSources.find(
                          (source) => source.id === dataSourceId,
                        ),
                      ),
                  )
                  .map((typeValue) => {
                    const p = SYNC_PRESETS.find(
                      (item) => item.value === typeValue,
                    )!;
                    const selected = selectedSyncTypes.includes(p.value);
                    return (
                      <button
                        key={p.value}
                        type="button"
                        data-testid={`sync-type-card-${p.value}`}
                        onClick={() =>
                          setSelectedSyncTypes((current) =>
                            toggleManualSyncTypeSelection(current, p.value),
                          )
                        }
                        className={`rounded-md border px-3 py-1.5 transition ${
                          selected
                            ? "border-primary bg-primary-light font-bold text-primary ring-1 ring-primary/30"
                            : "border-line bg-white text-slate-700 hover:border-primary/50 hover:bg-slate-50"
                        }`}
                        aria-pressed={selected}
                      >
                        <span className="inline-flex items-center gap-1.5 text-sm">
                          <span
                            className={`grid h-3.5 w-3.5 place-items-center rounded border ${selected ? "border-primary bg-primary text-white" : "border-line bg-white"}`}
                          >
                            {selected ? <Check className="h-3 w-3" /> : null}
                          </span>
                          {p.label.split(" · ")[1] ?? p.label}
                        </span>
                      </button>
                    );
                  })}
              </div>
            </div>
          ))}
        </div>
      </div>

      {/* Parameters row: data source + store search + date inline */}
      <div className="flex items-center gap-5 border-b border-line px-5 py-3">
        <label className="flex items-center gap-2 text-sm font-semibold text-ink-sub">
          数据源
          <SelectNative
            name="dataSourceId"
            value={dataSourceId}
            onChange={(event) => setDataSourceId(event.currentTarget.value)}
            className="w-44"
          >
            {dataSources.map((dataSource) => (
              <option key={dataSource.id} value={dataSource.id}>
                {dataSource.label}
              </option>
            ))}
          </SelectNative>
        </label>
        <SearchInput
          value={storeSearch}
          onChange={setStoreSearch}
          placeholder="搜索店铺名 / ID / 站点"
        />
        <div
          className={`flex items-center gap-2 transition-opacity${needsDates ? "" : " pointer-events-none opacity-40"}`}
        >
          <span className="text-sm font-semibold text-ink-sub">日期</span>
          <DateRangePicker
            value={dateRange}
            presetKey={datePreset}
            icon={<CalendarDays className="h-4 w-4" />}
            onChange={(next) => {
              setDateRange(next.value);
              setDatePreset(next.presetKey);
            }}
          />
          {!needsDates ? (
            <span className="text-xs text-ink-muted">不需要</span>
          ) : null}
        </div>
      </div>

      {/* Store selection — always visible, disabled overlay when not needed */}
      <div
        className={`relative min-h-0 flex-1 overflow-auto px-5 py-4${needsStores ? "" : " pointer-events-none select-none"}`}
      >
        {!needsStores ? (
          <div className="absolute inset-0 z-10 bg-white/60" />
        ) : null}
        <div className="mb-2 flex items-center justify-between">
          <label className="flex items-center gap-2 text-sm font-semibold text-ink-sub">
            <input
              type="checkbox"
              checked={
                visibleChannels.length > 0 &&
                selectedVisibleStores.length === visibleChannels.length
              }
              onChange={() =>
                setSelectedStores(
                  selectedVisibleStores.length === visibleChannels.length
                    ? []
                    : visibleChannels.map((c) => c.store_id),
                )
              }
              className="h-4 w-4 rounded border-line text-primary"
            />
            选择店铺
          </label>
          <span className="text-sm text-ink-muted">
            {needsStores
              ? `已选 ${selectedVisibleStores.length} / ${visibleChannels.length}`
              : "当前类型不需要选择店铺"}
          </span>
        </div>
        <div
          className="grid grid-cols-[repeat(auto-fill,minmax(240px,1fr))] gap-2"
          data-testid="sync-store-checkbox-list"
        >
          {filteredChannels.length ? (
            filteredChannels.map((channel) => {
              const checked = selectedVisibleStores.includes(channel.store_id);
              return (
                <label
                  key={`${channel.data_source_id}:${channel.store_id}:${channel.channel_type}`}
                  className={`flex cursor-pointer items-center gap-2.5 rounded-lg border px-3 py-2 text-sm transition ${checked ? "border-primary bg-primary-light text-slate-900" : "border-line bg-white text-slate-700 hover:border-primary/40 hover:bg-slate-50"}`}
                >
                  <input
                    type="checkbox"
                    checked={checked}
                    onChange={() =>
                      setSelectedStores((current) =>
                        toggleStoreSelection(current, channel.store_id),
                      )
                    }
                    className="h-4 w-4 rounded border-line text-primary"
                  />
                  <span className="min-w-0 flex-1">
                    <span className="block truncate font-semibold">
                      {channel.store_name ?? channel.store_id}
                    </span>
                    <span className="block truncate text-xs text-ink-muted">
                      {channel.store_id} ·{" "}
                      {channelTypeLabel(channel.channel_type)}
                      {channel.country ? ` · ${channel.country}` : ""}
                    </span>
                  </span>
                </label>
              );
            })
          ) : (
            <div className="col-span-full rounded-lg border border-dashed border-line bg-slate-50 px-4 py-6 text-center text-sm text-ink-muted">
              当前数据源暂无店铺目录，请先同步店铺目录。
            </div>
          )}
        </div>
      </div>
    </form>
  );
}

function ManualSyncNoticeBanner({
  notice,
  onClose,
}: {
  notice: ManualSyncNotice | null;
  onClose: () => void;
}) {
  if (!notice) return null;
  return (
    <div className="pointer-events-none fixed inset-x-0 top-[52px] z-50 flex justify-center px-6">
      <div
        data-testid="manual-sync-local-notice"
        className="pointer-events-auto flex w-fit min-w-[min(374px,calc(100vw-48px))] max-w-[min(760px,calc(100vw-48px))] items-start justify-between gap-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm font-semibold text-red-700 shadow-lg shadow-slate-900/10"
      >
        <div className="grid min-w-0 gap-1.5">
          <span className="text-sm font-extrabold leading-6">
            {notice.message}
          </span>
          <div className="grid gap-1 whitespace-normal break-words text-sm font-extrabold leading-6 [overflow-wrap:anywhere]">
            {notice.lines.map((line) => (
              <span key={line}>{line}</span>
            ))}
          </div>
        </div>
        <button
          type="button"
          className="shrink-0 text-lg font-bold leading-none opacity-70 hover:opacity-100"
          onClick={onClose}
        >
          ×
        </button>
      </div>
    </div>
  );
}

function SyncLoadNoticeBanner({
  notice,
  onClose,
}: {
  notice: SyncLoadNotice | null;
  onClose: () => void;
}) {
  if (!notice) return null;
  return (
    <div className="pointer-events-none fixed inset-x-0 top-[52px] z-50 flex justify-center px-6">
      <div
        data-testid="sync-load-local-notice"
        className="pointer-events-auto flex w-fit min-w-[min(374px,calc(100vw-48px))] max-w-[min(760px,calc(100vw-48px))] items-start justify-between gap-3 rounded-xl border border-red-200 bg-red-50 px-4 py-3 text-sm font-semibold text-red-700 shadow-lg shadow-slate-900/10"
      >
        <div className="grid min-w-0 gap-1.5">
          <span className="text-sm font-extrabold leading-6">
            {notice.message}
          </span>
          <div className="grid gap-1 whitespace-normal break-words text-sm font-extrabold leading-6 [overflow-wrap:anywhere]">
            {notice.lines.map((line) => (
              <span key={line}>{line}</span>
            ))}
          </div>
        </div>
        <button
          type="button"
          className="shrink-0 text-lg font-bold leading-none opacity-70 hover:opacity-100"
          onClick={onClose}
        >
          ×
        </button>
      </div>
    </div>
  );
}

function OverviewSourceTabs({
  activeSource,
  onChange,
  rows,
}: {
  activeSource: SyncOverviewSourceKey;
  onChange: (source: SyncOverviewSourceKey) => void;
  rows: SyncOverviewStoreRow[];
}) {
  return (
    <nav
      className="flex shrink-0 items-end gap-3 border-b border-line bg-slate-50/80 px-4 pt-4"
      data-testid="sync-overview-source-tabs"
      aria-label="概览来源"
    >
      {OVERVIEW_SOURCES.map((source) => {
        const selected = activeSource === source.key;
        const count = rows.filter(
          (row) => row.source_key === source.key,
        ).length;
        return (
          <button
            key={source.key}
            type="button"
            onClick={() => onChange(source.key)}
            className={`min-h-12 min-w-40 rounded-t-lg border px-6 text-sm font-extrabold transition ${
              selected
                ? "border-primary bg-white text-primary shadow-sm"
                : "border-line bg-slate-50 text-slate-600 hover:bg-white hover:text-primary"
            }`}
            aria-pressed={selected}
          >
            <span>{source.label}</span>
            {count ? (
              <span className="ml-2 text-xs font-semibold text-ink-sub">
                {count}
              </span>
            ) : null}
          </button>
        );
      })}
    </nav>
  );
}

function OverviewTable({
  rows,
  source,
  onOpenCoverage,
}: {
  rows: SyncOverviewStoreRow[];
  source: SyncOverviewSourceKey;
  onOpenCoverage: (row: SyncOverviewStoreRow) => void;
}) {
  const emptyText =
    source === "spotterio" ? "暂无 Spotter 同步历史" : "暂无店铺同步历史";
  return (
    <div className="min-h-0 flex-1 overflow-auto">
      <table
        className="min-w-[1080px] w-full text-sm"
        data-testid="sync-overview-table"
      >
        <thead className="sticky top-0 z-10 border-b border-line bg-slate-50 text-xs font-extrabold text-slate-600">
          <tr>
            <th className="px-4 py-3 text-center">店铺 id</th>
            <th className="px-4 py-3 text-center">店铺</th>
            <th className="px-4 py-3 text-center">ASIN</th>
            <th className="px-4 py-3 text-center">日期范围</th>
            <th className="px-4 py-3 text-center">主表</th>
            <th className="px-4 py-3 text-center">明细</th>
            <th className="px-4 py-3 text-center">广告</th>
            <th className="px-4 py-3 text-center">覆盖率(90d)</th>
            <th className="px-4 py-3 text-center">空洞天数</th>
            <th className="px-4 py-3 text-center">最新同步</th>
          </tr>
        </thead>
        <tbody className="divide-y divide-slate-100">
          {rows.length ? (
            rows.map((row) => (
              <tr
                key={`${row.source_key}:${row.data_source_id ?? "spotter"}:${row.store_key}`}
                className={`bg-white hover:bg-slate-50/70 ${row.source_key === "spotterio" ? "" : "cursor-pointer"}`}
                onClick={() => {
                  if (row.source_key !== "spotterio") onOpenCoverage(row);
                }}
              >
                <td className="px-4 py-2.5 text-center font-mono text-xs font-semibold text-slate-700">
                  {row.store_key}
                </td>
                <td className="px-4 py-2.5">
                  <div className="font-semibold text-primary">
                    {formatStoreLabel(row)}
                  </div>
                </td>
                <td className="px-4 py-2.5 text-center font-medium text-slate-900">
                  {row.asin_count}
                </td>
                <td className="px-4 py-2.5 text-center font-medium text-slate-600">
                  {formatOverviewDateRange(row.start_date, row.end_date)}
                </td>
                <td className="px-4 py-2.5 text-center text-slate-900">
                  {row.main_rows}
                </td>
                <td className="px-4 py-2.5 text-center text-slate-900">
                  {row.detail_rows}
                </td>
                <td className="px-4 py-2.5 text-center text-slate-900">
                  {row.ad_rows}
                </td>
                <td className="px-4 py-2.5 text-center font-semibold text-emerald-700">
                  {row.source_key === "spotterio"
                    ? "—"
                    : `${row.synced_days}/${row.total_days}`}
                </td>
                <td
                  className={`px-4 py-2.5 text-center font-semibold ${row.gap_days > 0 ? "text-danger" : "text-slate-400"}`}
                >
                  {row.source_key === "spotterio" || row.gap_days === 0
                    ? "—"
                    : `${row.gap_days} 天`}
                </td>
                <td className="px-4 py-2.5 text-center text-slate-600">
                  {formatDateOnlyString(row.latest_sync_at)}
                </td>
              </tr>
            ))
          ) : (
            <tr>
              <td
                colSpan={10}
                className="px-4 py-10 text-center text-sm text-slate-500"
              >
                {emptyText}
              </td>
            </tr>
          )}
        </tbody>
      </table>
    </div>
  );
}

function formatStoreLabel(row: SyncOverviewStoreRow) {
  if (row.source_key === "spotterio") return row.store_label;
  const prefix = row.channel_type === "sc" ? "SC" : "VC";
  return `(${prefix}) ${row.store_label}`;
}

function buildStoreLabelMap(
  rows: SyncOverviewStoreRow[],
  channels: ChannelRow[] = [],
): StoreLabelMap {
  const entries: Array<[string, StoreLabel]> = channels.flatMap((channel) => {
    const storeName = channel.store_name || channel.store_id;
    const label = {
      id: channel.store_id,
      name: storeName,
      text: `${storeName} · 店铺ID ${channel.store_id}`,
    };
    return [
      [
        storeLabelKey(
          channel.data_source_id,
          channel.store_id,
          channel.channel_type,
        ),
        label,
      ],
      [storeLabelKey(channel.data_source_id, channel.store_id), label],
      [storeLabelKey("", channel.store_id), label],
    ];
  });
  entries.push(
    ...rows.flatMap((row): Array<[string, StoreLabel]> => {
      if (!row.data_source_id || row.source_key === "spotterio") return [];
      const label = {
        id: row.store_key,
        name: row.store_label,
        text: `${row.store_label} · 店铺ID ${row.store_key}`,
      };
      return [
        [
          storeLabelKey(
            row.data_source_id,
            row.store_key,
            row.channel_type ?? undefined,
          ),
          label,
        ],
        [storeLabelKey(row.data_source_id, row.store_key), label],
        [storeLabelKey("", row.store_key), label],
      ];
    }),
  );
  return new Map(entries);
}

function storeLabelKey(
  dataSourceId: string,
  storeId: string,
  channelType?: string,
) {
  return `${dataSourceId}::${storeId}::${channelType ?? ""}`;
}

function formatOverviewDateRange(start: string | null, end: string | null) {
  if (start && end)
    return `${formatDateOnlyString(start)} ~ ${formatDateOnlyString(end)}`;
  if (start) return formatDateOnlyString(start);
  return "-";
}

function formatDateOnlyString(value: string | null) {
  if (!value) return "-";
  const raw = String(value);
  if (/^\d{4}-\d{2}-\d{2}/.test(raw)) return raw.slice(0, 10);
  const date = new Date(raw);
  if (Number.isNaN(date.getTime())) return raw;
  return formatDateOnly(date);
}

function TabStrip({
  activeTab,
  overview,
  onChange,
}: {
  activeTab: SyncTab;
  overview: SyncOverview;
  onChange: (tab: SyncTab) => void;
}) {
  return (
    <nav
      className="flex shrink-0 items-center gap-1.5 whitespace-nowrap"
      data-testid="sync-tab-strip"
      aria-label="同步中心视图"
    >
      {SYNC_TABS.map((tab) => {
        const selected = activeTab === tab.value;
        const count = tab.count ? overview[tab.count] : null;
        return (
          <button
            key={tab.value}
            type="button"
            onClick={() => onChange(tab.value)}
            className={`inline-flex min-h-8 items-center gap-2 rounded-md border px-3 py-1 text-sm font-bold transition ${
              selected
                ? "border-primary bg-primary text-white"
                : "border-transparent bg-white text-slate-600 hover:border-line hover:bg-slate-50 hover:text-slate-900"
            }`}
            aria-pressed={selected}
            data-testid={`sync-tab-${tab.value}`}
          >
            {tab.icon}
            <span>{tab.label}</span>
            {count !== null ? (
              <span
                className={`rounded-full px-1.5 py-0.5 text-[11px] ${selected ? "bg-white/20 text-white" : "bg-slate-100 text-slate-500"}`}
              >
                {count}
              </span>
            ) : null}
          </button>
        );
      })}
    </nav>
  );
}

function DenseRunTable({
  groups,
  storeLabels,
  hasNext,
  page,
  pageSize,
  onPageChange,
  onPageSizeChange,
  onRefresh,
  onLocalError,
  runningOnly,
}: {
  groups: ParentRunGroup[];
  storeLabels: StoreLabelMap;
  hasNext: boolean;
  page: number;
  pageSize: number;
  onPageChange: (page: number) => void;
  onPageSizeChange: (size: number) => void;
  onRefresh: () => Promise<void>;
  onLocalError: (message: string, error: unknown) => void;
  runningOnly: boolean;
}) {
  const [collapsedGroups, setCollapsedGroups] = useState<
    Record<string, boolean>
  >({});
  function toggleCompoundGroup(runId: string) {
    setCollapsedGroups((current) => ({
      ...current,
      [runId]: !(current[runId] ?? !runningOnly),
    }));
  }

  return (
    <div className="flex min-h-0 flex-1 flex-col">
      <div
        className="relative min-h-0 flex-1 overflow-auto"
        data-testid="sync-run-table-wrap"
      >
        <table
          className="min-w-[1560px] w-full table-fixed text-left text-xs"
          data-testid="sync-run-table"
        >
          <colgroup>
            <col style={{ width: "12%" }} />
            <col style={{ width: "7.3%" }} />
            <col style={{ width: "8.2%" }} />
            <col style={{ width: "14%" }} />
            <col style={{ width: "12%" }} />
            <col style={{ width: "9.3%" }} />
            <col style={{ width: "7.5%" }} />
            <col style={{ width: "15.2%" }} />
            <col style={{ width: "14.5%" }} />
          </colgroup>
          <thead className="sticky top-0 z-10 border-b border-line bg-slate-50 text-[11px] font-extrabold text-slate-600">
            <tr>
              <th className="px-3 py-2">时间</th>
              <th className="px-3 py-2">触发人</th>
              <th className="px-3 py-2">类型</th>
              <th className="px-3 py-2">店铺</th>
              <th className="px-3 py-2">窗口</th>
              <th className="px-3 py-2">执行进度</th>
              <th className="px-3 py-2 text-center">状态</th>
              <th className="px-3 py-2">完成情况</th>
              <th className="px-3 py-2">操作</th>
            </tr>
          </thead>
          <tbody className="divide-y divide-slate-100">
            {groups.length ? (
              groups.map((group) => (
                <RunTableRow
                  key={group.run.run_id}
                  group={group}
                  storeLabels={storeLabels}
                  collapsed={
                    group.children.length
                      ? (collapsedGroups[group.run.run_id] ?? !runningOnly)
                      : false
                  }
                  onToggle={() => toggleCompoundGroup(group.run.run_id)}
                  onRefresh={onRefresh}
                  onLocalError={onLocalError}
                  runningOnly={runningOnly}
                />
              ))
            ) : (
              <tr>
                <td
                  colSpan={9}
                  className="px-4 py-10 text-center text-sm text-slate-500"
                >
                  当前筛选下没有同步批次。
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
      <PaginationStatusBar
        testId="sync-run-pagination-status"
        leftItems={
          <span>
            本页数量：<strong>{groups.length}</strong>
          </span>
        }
        middleItems={
          <>
            <span>每页</span>
            <select
              className="h-7 rounded border border-line bg-white px-2 text-xs font-semibold text-slate-700"
              value={pageSize}
              onChange={(event) => onPageSizeChange(Number(event.target.value))}
              aria-label="同步日志每页数量"
            >
              <option value={30}>30</option>
              <option value={50}>50</option>
              <option value={100}>100</option>
            </select>
            <span>父任务</span>
          </>
        }
        onPageChange={onPageChange}
        page={page}
        total={0}
        totalPages={page}
        hasNext={hasNext}
      />
    </div>
  );
}

function RunTableRow({
  group,
  storeLabels,
  collapsed,
  onToggle,
  onRefresh,
  onLocalError,
  runningOnly,
}: {
  group: ParentRunGroup;
  storeLabels: StoreLabelMap;
  collapsed: boolean;
  onToggle: () => void;
  onRefresh: () => Promise<void>;
  onLocalError: (message: string, error: unknown) => void;
  runningOnly: boolean;
}) {
  const retryRunIds = runActionIdsForGroup(group, "retry");
  const cancelRunIds = runActionIdsForGroup(group, "cancel");
  const canRetry = retryRunIds.length > 0;
  const canCancel = cancelRunIds.length > 0;
  const atomCompat = isAtomCompatRun(group.run);
  const displayStatus = groupDisplayStatus(group);
  const leaseExpired = isExpiredActiveLease(group.run);
  const showRunningIndicator = displayStatus === "running" && !leaseExpired;
  const storeLabel = collectStoreLabel(group.scope, storeLabels);
  const syncTypeParts = splitSyncTypeLabel(
    SYNC_TYPE_LABEL[group.run.sync_type] ?? group.run.sync_type,
  );
  const triggerActor = triggerActorLabel(group.scope);
  const atomResubmitHref = atomCompat
    ? buildAtomResubmitHref(group.run, group.scope)
    : null;
  const visibleChildren = runningOnly
    ? group.children.filter((child) =>
        isProjectedActiveSyncRunStatus(child.status),
      )
    : group.children;
  const hasChildRuns = group.children.length > 0;
  const completionSummary = segmentSummary(group, displayStatus);
  const completionText = completionSummaryText(
    completionSummary,
    group.debugText,
  );
  const durationNowMs = Date.now();
  const durationText = formatGroupRunDuration(
    group,
    durationNowMs,
    displayStatus,
  );
  const [retrying, setRetrying] = useState(false);
  const [cancelling, setCancelling] = useState(false);

  async function handleRetry(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const runIds = syncRunActionIdsFromForm(event.currentTarget);
    if (!runIds.length) return;
    setRetrying(true);
    try {
      const failures = await submitSyncRunActions(
        runIds,
        "retry",
        "同步重试失败",
      );
      await onRefresh();
      if (failures.length) {
        onLocalError("同步重试失败", new Error(failures.join("\n")));
      }
    } catch (error) {
      onLocalError("同步重试失败", error);
    } finally {
      setRetrying(false);
    }
  }

  async function handleCancel(event: FormEvent<HTMLFormElement>) {
    event.preventDefault();
    const runIds = syncRunActionIdsFromForm(event.currentTarget);
    if (!runIds.length) return;
    setCancelling(true);
    try {
      const failures = await submitSyncRunActions(
        runIds,
        "cancel",
        "同步取消失败",
      );
      await onRefresh();
      if (failures.length) {
        onLocalError("同步取消失败", new Error(failures.join("\n")));
      }
    } catch (error) {
      onLocalError("同步取消失败", error);
    } finally {
      setCancelling(false);
    }
  }

  return (
    <>
      <tr
        className={`${hasChildRuns ? "bg-primary/5 shadow-[inset_0_-1px_0_rgba(37,99,235,0.14)]" : "bg-white"} hover:bg-slate-100 ${showRunningIndicator ? "sync-run-row-active" : ""} ${hasChildRuns && !collapsed && visibleChildren.length > 0 ? "sync-run-parent-expanded" : ""}`}
      >
        <td className="sync-run-hcell px-3 py-2 align-middle">
          <div className="flex items-center gap-2">
            {hasChildRuns ? (
              <button
                type="button"
                className="inline-flex h-6 w-6 shrink-0 items-center justify-center rounded-md border border-primary/30 bg-white text-primary shadow-sm hover:bg-primary/10 focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
                aria-expanded={!collapsed}
                aria-label={
                  collapsed
                    ? `展开 ${visibleChildren.length} 个子任务`
                    : `收起 ${visibleChildren.length} 个子任务`
                }
                title={
                  collapsed
                    ? `展开 ${visibleChildren.length} 个子任务`
                    : `收起 ${visibleChildren.length} 个子任务`
                }
                data-testid="sync-run-toggle-children"
                onClick={onToggle}
              >
                {collapsed ? (
                  <ChevronRight className="h-4 w-4" />
                ) : (
                  <ChevronDown className="h-4 w-4" />
                )}
              </button>
            ) : null}
            <div className="min-w-0">
              <div className="font-semibold text-slate-900">
                {formatDateTime(group.run.created_at)}
              </div>
              <div
                className="mt-1 truncate font-mono text-[11px] text-ink-sub"
                title={group.run.run_id}
              >
                {group.run.run_id}
              </div>
            </div>
          </div>
        </td>
        <td className="px-3 py-2 align-middle">
          <div
            className="truncate font-semibold text-slate-900"
            title={triggerActor}
          >
            {triggerActor}
          </div>
        </td>
        <td className="px-3 py-2 align-middle">
          {hasChildRuns ? (
            <div className="mb-1 inline-flex rounded bg-primary/10 px-1.5 py-0.5 text-[10px] font-extrabold text-primary">
              组合父任务
            </div>
          ) : null}
          <div className="sync-type-primary whitespace-nowrap font-semibold text-slate-900">
            {syncTypeParts.primary}
          </div>
          {syncTypeParts.secondary ? (
            <div className="sync-type-secondary mt-1 whitespace-nowrap text-[11px] font-medium text-ink-sub">
              {syncTypeParts.secondary}
            </div>
          ) : null}
        </td>
        <td className="px-3 py-2 align-middle">
          <div
            className="truncate font-semibold text-slate-900"
            title={storeLabel.text}
          >
            {storeLabel.name}
          </div>
          {storeLabel.id ? (
            <div className="mt-1 truncate text-[11px] font-medium text-ink-sub">
              店铺ID {storeLabel.id}
            </div>
          ) : null}
        </td>
        <td className="px-3 py-2 align-middle">
          <div
            className="truncate font-medium text-slate-800"
            title={scopeLabel(group.scope)}
          >
            {scopeLabel(group.scope)}
          </div>
          <div className="mt-1 text-[11px] text-ink-sub">
            {group.scope.channelType
              ? `通道 ${group.scope.channelType}`
              : "通道按任务定义"}
          </div>
        </td>
        <td className="px-3 py-2 align-middle">
          <div className="font-semibold text-slate-900">
            {visibleChildren.length
              ? `${visibleChildren.length}个子任务`
              : standaloneTaskLabel(group.run)}
          </div>
          <div
            className="mt-1 truncate text-[11px] text-ink-sub"
            title={durationText}
          >
            {durationText}
          </div>
        </td>
        <td className="px-3 py-2 text-center align-middle">
          <StatusBadge
            value={leaseExpired ? "租约已过期" : displayStatus}
            tone={leaseExpired ? "danger" : undefined}
            loading={showRunningIndicator}
          />
          <div className="mt-1 text-[11px] text-ink-sub">
            {formatDateTime(group.run.ended_at)}
          </div>
        </td>
        <td className="px-3 py-2 align-middle">
          <div
            className={`line-clamp-2 font-medium ${completionTextClass(displayStatus, group.children)}`}
          >
            {completionText}
          </div>
        </td>
        <td className="px-3 py-2 align-middle">
          <div className="flex min-w-max items-center justify-start gap-1 whitespace-nowrap">
            {canRetry ? (
              <form
                onSubmit={(event) => void handleRetry(event)}
                className="inline-flex shrink-0"
              >
                {retryRunIds.map((runId) => (
                  <input key={runId} type="hidden" name="runId" value={runId} />
                ))}
                <Button
                  variant="outline"
                  size="sm"
                  type="submit"
                  disabled={retrying}
                  data-testid="sync-run-retry-button"
                >
                  {retrying ? "重试中…" : "整批重试"}
                </Button>
              </form>
            ) : null}
            {canCancel ? (
              <form
                onSubmit={(event) => void handleCancel(event)}
                className="inline-flex shrink-0"
              >
                {cancelRunIds.map((runId) => (
                  <input key={runId} type="hidden" name="runId" value={runId} />
                ))}
                <Button
                  variant="outline"
                  size="sm"
                  type="submit"
                  disabled={cancelling}
                  data-testid="sync-run-cancel-button"
                >
                  {cancelling ? "取消中…" : "取消"}
                </Button>
              </form>
            ) : null}
            {atomResubmitHref ? (
              <a
                href={atomResubmitHref}
                className="inline-flex min-h-[28px] shrink-0 items-center whitespace-nowrap rounded-md border border-line px-2 text-xs font-semibold text-slate-700 hover:bg-slate-50"
                title="返回手动同步调整范围后重新提交"
                data-testid="sync-run-atom-resubmit-link"
              >
                手动同步
              </a>
            ) : null}
          </div>
        </td>
      </tr>
      {hasChildRuns && !collapsed
        ? visibleChildren.map((child, index) => (
            <RunTableChildRow
              key={child.run_id}
              run={child}
              index={index}
              isLast={index === visibleChildren.length - 1}
              parentRunId={group.run.run_id}
              segments={group.segments.filter(
                (segment) => segment.run_id === child.run_id,
              )}
              storeLabels={storeLabels}
              nowMs={durationNowMs}
            />
          ))
        : null}
    </>
  );
}

function RunTableChildRow({
  run,
  index,
  isLast,
  parentRunId,
  segments,
  storeLabels,
  nowMs,
}: {
  run: SyncRunView;
  index: number;
  isLast: boolean;
  parentRunId: string;
  segments: SegmentRow[];
  storeLabels: StoreLabelMap;
  nowMs: number;
}) {
  const scope = parseScope(run.target_scope_json);
  const storeLabel = collectStoreLabel(scope, storeLabels);
  const syncTypeParts = splitSyncTypeLabel(
    SYNC_TYPE_LABEL[run.sync_type] ?? run.sync_type,
  );
  const debugMessage = buildDebugMessage(run, [], segments);
  const completionSummary = childRunSummary(run, segments);
  const completionText = completionSummaryText(
    completionSummary,
    debugMessage.debugText,
  );
  const leaseExpired = isExpiredActiveLease(run);
  const showRunningIndicator = run.status === "running" && !leaseExpired;

  return (
    <tr
      className="bg-slate-50/80 text-slate-700 hover:bg-primary/5"
      data-testid="sync-run-child-row"
    >
      <td
        className={`px-3 py-2 align-middle sync-hierarchy-child${isLast ? " sync-hierarchy-child-last" : ""}`}
      >
        <div className="relative z-[1] flex items-center pl-7 font-semibold text-slate-800">
          <span className="rounded bg-white px-1.5 py-0.5 text-[11px] font-extrabold text-slate-600 shadow-sm">{`子任务 ${index + 1}`}</span>
        </div>
        <div
          className="relative z-[1] mt-1 truncate pl-7 font-mono text-[11px] text-ink-sub"
          title={run.run_id}
        >
          {run.run_id}
        </div>
      </td>
      <td className="px-3 py-2 align-middle">
        <div
          className="truncate text-[11px] font-medium text-ink-sub"
          title={parentRunId}
        >{`父任务 ${parentRunId}`}</div>
      </td>
      <td className="px-3 py-2 align-middle">
        <div className="sync-type-primary whitespace-nowrap font-semibold text-slate-800">
          {syncTypeParts.primary}
        </div>
        {syncTypeParts.secondary ? (
          <div className="sync-type-secondary mt-1 whitespace-nowrap text-[11px] font-medium text-ink-sub">
            {syncTypeParts.secondary}
          </div>
        ) : null}
      </td>
      <td className="px-3 py-2 align-middle">
        <div
          className="truncate font-medium text-slate-800"
          title={storeLabel.text}
        >
          {storeLabel.name}
        </div>
        {storeLabel.id ? (
          <div className="mt-1 truncate text-[11px] text-ink-sub">
            店铺ID {storeLabel.id}
          </div>
        ) : null}
      </td>
      <td className="px-3 py-2 align-middle">
        <div
          className="truncate font-medium text-slate-800"
          title={scopeLabel(scope)}
        >
          {scopeLabel(scope)}
        </div>
        <div className="mt-1 text-[11px] text-ink-sub">
          {scope.channelType ? `通道 ${scope.channelType}` : "继承父任务范围"}
        </div>
      </td>
      <td className="px-3 py-2 align-middle">
        <div className="font-semibold text-slate-800">
          {childTaskUnitLabel(run)}
        </div>
        <div
          className="mt-1 truncate text-[11px] text-ink-sub"
          title={formatRunDuration(run, nowMs)}
        >
          {formatRunDuration(run, nowMs)}
        </div>
      </td>
      <td className="px-3 py-2 text-center align-middle">
        <StatusBadge
          value={leaseExpired ? "租约已过期" : run.status}
          tone={leaseExpired ? "danger" : undefined}
          loading={showRunningIndicator}
        />
        <div className="mt-1 text-[11px] text-ink-sub">
          {formatDateTime(run.ended_at)}
        </div>
      </td>
      <td className="px-3 py-2 align-middle">
        <div
          className={`line-clamp-2 font-medium ${completionTextClass(run.status)}`}
        >
          {completionText}
        </div>
      </td>
      <td className="px-3 py-2 align-middle text-left text-[11px] font-medium text-ink-sub">
        随父任务
      </td>
    </tr>
  );
}

function SchedulePanel({
  schedules,
  channels,
  searchParams,
  pathname,
}: {
  schedules: WorkspaceSchedule[];
  channels: ChannelRow[];
  searchParams: ReturnType<typeof useSearchParams>;
  pathname: string;
}) {
  const [filter, setFilter] = useState<string>(() =>
    normalizeScheduleFilter(searchParams?.get("schedule")),
  );
  const filtered =
    filter === "all"
      ? schedules
      : schedules.filter((s) => scheduleMatchesFilter(s.task_type, filter));

  function handleFilterChange(nextFilter: string) {
    const normalized = normalizeScheduleFilter(nextFilter);
    setFilter(normalized);
    const next = new URLSearchParams(searchParams?.toString() ?? "");
    next.set("tab", "schedules");
    if (normalized === "all") next.delete("schedule");
    else next.set("schedule", normalized);
    const query = next.toString();
    window.history.replaceState(
      null,
      "",
      query ? `${pathname}?${query}` : pathname,
    );
  }

  return (
    <section
      className="flex h-full min-h-0 flex-col"
      data-testid="sync-schedule-panel"
    >
      <SectionHeader
        icon={<CalendarClock className="h-4 w-4" />}
        title="预设计划"
        controls={
          <>
            <SchedulePanelHelp />
            <nav className="flex items-center gap-1">
              {SCHEDULE_FILTERS.map((f) => {
                const selected = filter === f.value;
                return (
                  <button
                    key={f.value}
                    type="button"
                    onClick={() => handleFilterChange(f.value)}
                    className={`inline-flex items-center rounded-md border px-2.5 py-1 text-xs font-bold transition ${selected ? "border-primary bg-primary text-white" : "border-transparent text-slate-600 hover:border-line hover:bg-slate-50 hover:text-slate-900"}`}
                  >
                    {f.label}
                  </button>
                );
              })}
            </nav>
          </>
        }
      />
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="min-w-[1120px] w-full table-fixed text-sm">
          <colgroup>
            <col className="w-[180px]" />
            <col className="w-[210px]" />
            <col className="w-[150px]" />
            <col className="w-[180px]" />
            <col />
            <col className="w-[90px]" />
            <col className="w-[150px]" />
          </colgroup>
          <thead className="sticky top-0 z-10 bg-slate-50 text-left text-xs font-extrabold uppercase tracking-wide text-slate-500 shadow-[0_1px_0_#e2e8f0]">
            <tr>
              <th className="px-4 py-2.5">任务</th>
              <th className="px-4 py-2.5">页面用途</th>
              <th className="px-4 py-2.5">时间策略</th>
              <th className="px-4 py-2.5">数据源</th>
              <th className="px-4 py-2.5">CRON</th>
              <th className="px-4 py-2.5">状态</th>
              <th className="py-2.5 pl-2 pr-4">操作</th>
            </tr>
          </thead>
          <tbody>
            {filtered.length ? (
              filtered.map((schedule) => (
                <ScheduleEditRow
                  key={schedule.id}
                  schedule={schedule}
                  channels={channels}
                />
              ))
            ) : (
              <tr>
                <td
                  colSpan={7}
                  className="px-4 py-8 text-center text-sm text-slate-500"
                >
                  当前没有预设计划。
                </td>
              </tr>
            )}
          </tbody>
        </table>
      </div>
    </section>
  );
}

function SchedulePanelHelp() {
  return (
    <TooltipProvider delayDuration={120}>
      <Tooltip>
        <TooltipTrigger asChild>
          <button
            aria-label="预设计划使用说明"
            className="inline-flex h-5 w-5 shrink-0 items-center justify-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
            type="button"
          >
            <HelpCircle className="h-3.5 w-3.5" />
          </button>
        </TooltipTrigger>
        <TooltipContent
          data-testid="schedule-panel-help-tooltip"
          side="bottom"
          align="start"
          sideOffset={8}
          className="z-50 grid w-[min(560px,calc(100vw-48px))] max-w-[calc(100vw-48px)] gap-2 whitespace-normal break-words rounded-lg border border-slate-200 bg-white px-4 py-3 text-[13px] font-semibold leading-6 text-slate-600 shadow-[0_18px_42px_rgba(15,23,42,0.16)]"
        >
          <div className="grid min-w-0 grid-cols-[18px_minmax(0,1fr)] gap-3">
            <CalendarDays className="mt-1 h-4 w-4 text-slate-500" />
            <span className="min-w-0 break-words [overflow-wrap:anywhere]">
              <strong className="font-extrabold text-slate-800">
                CRON 填法：
              </strong>
              按 5 段填写：分钟 小时 日期 月份 星期；不填秒。
            </span>
          </div>
          <div className="grid min-w-0 grid-cols-[18px_minmax(0,1fr)] gap-3">
            <HelpCircle className="mt-1 h-4 w-4 text-slate-500" />
            <span className="min-w-0 break-words [overflow-wrap:anywhere]">
              <strong className="font-extrabold text-slate-800">
                每个 *：
              </strong>
              第 1 段分钟 * = 每分钟；第 2 段小时 * = 每小时；第 3 段日期 * =
              每天；第 4 段月份 * = 每月；第 5 段星期 * = 不限定星期。
            </span>
          </div>
          <div className="grid min-w-0 grid-cols-[18px_minmax(0,1fr)] gap-3">
            <ArrowLeftRight className="mt-1 h-4 w-4 text-slate-500" />
            <span className="min-w-0 break-words [overflow-wrap:anywhere]">
              <strong className="font-extrabold text-slate-800">
                常用符号：
              </strong>
              `*` 表示该段全部；`,` 表示多个值，例如小时 `3,15` = 3 点和 15 点。
            </span>
          </div>
          <div className="grid min-w-0 grid-cols-[18px_minmax(0,1fr)] gap-3">
            <Database className="mt-1 h-4 w-4 text-slate-500" />
            <span className="min-w-0 break-words [overflow-wrap:anywhere]">
              <strong className="font-extrabold text-slate-800">例子：</strong>
              `0 3,15 * * *` = 每天 3:00、15:00；`15 2,14 * * *` = 每天
              2:15、14:15；`0 2 * * 1` = 每周一 2:00；`30 8 1 * *` = 每月 1 号
              8:30。
            </span>
          </div>
          <div className="grid min-w-0 grid-cols-[18px_minmax(0,1fr)] gap-3">
            <Save className="mt-1 h-4 w-4 text-slate-500" />
            <span className="min-w-0 break-words [overflow-wrap:anywhere]">
              <strong className="font-extrabold text-slate-800">
                保存方式：
              </strong>
              改完
              CRON、时间策略或启停状态后，点本行保存才生效；右侧中文说明会显示当前
              CRON 含义。
            </span>
          </div>
        </TooltipContent>
      </Tooltip>
    </TooltipProvider>
  );
}

function ScheduleEditRow({
  schedule,
  channels,
}: {
  schedule: WorkspaceSchedule;
  channels: ChannelRow[];
}) {
  const router = useRouter();
  const dateWindowTask = scheduleUsesDateWindow(schedule.task_type);
  const savedDateWindowPreset = readScheduleDateWindowPreset(schedule);
  const [cronExpr, setCronExpr] = useState(schedule.cron_expr ?? "");
  const [enabled, setEnabled] = useState(Boolean(schedule.enabled));
  const [dateWindowPreset, setDateWindowPreset] =
    useState<ScheduleDateWindowPreset>(savedDateWindowPreset);
  const [saveFeedback, setSaveFeedback] = useState<"idle" | "saving" | "saved">(
    "idle",
  );
  const [running, setRunning] = useState(false);
  const saving = saveFeedback === "saving";
  const saved = saveFeedback === "saved";
  const isDirty =
    cronExpr.trim() !== (schedule.cron_expr ?? "").trim() ||
    enabled !== Boolean(schedule.enabled) ||
    (dateWindowTask && dateWindowPreset !== savedDateWindowPreset);
  const runNowStoreIds = useMemo(
    () =>
      channels
        .filter(
          (channel) =>
            channel.data_source_id === schedule.data_source_id &&
            channelMatchesScheduleTask(
              schedule.task_type,
              channel.channel_type,
            ),
        )
        .map((channel) => channel.store_id),
    [channels, schedule.data_source_id, schedule.task_type],
  );

  async function handleSave() {
    setSaveFeedback("saving");
    try {
      const result = await reportScheduleSaveUnavailable({
        dataSourceId: schedule.data_source_id,
        taskType: schedule.task_type,
        cronExpr,
        enabled,
        schedulePayload: dateWindowTask ? { dateWindowPreset } : {},
      });
      if (!result.ok) {
        toast.error(result.error ?? "保存失败");
        if (result.dbChanged) router.refresh();
        setSaveFeedback("idle");
        return;
      }
      setSaveFeedback("saved");
      router.refresh();
      setTimeout(() => setSaveFeedback("idle"), 550);
    } catch (error) {
      toast.error(formatErrorMessage(error, "保存失败"));
      setSaveFeedback("idle");
    }
  }

  async function runNow() {
    setRunning(true);
    try {
      const request = new FormData();
      request.set("syncType", schedule.task_type);
      request.set("dataSourceId", schedule.data_source_id);
      request.set("clientRequestId", crypto.randomUUID());
      request.set("channelType", inferChannelType(schedule.task_type));
      if (requiresScheduleStores(schedule.task_type))
        request.set("stores", runNowStoreIds.join(","));
      if (dateWindowTask) {
        request.set("dateWindowPreset", dateWindowPreset);
      }
      await showSyncActionUnavailable(request);
      toast.success("已提交同步任务，可在同步日志查看进度");
      router.refresh();
    } catch (error) {
      toast.error(formatErrorMessage(error, "同步任务提交失败"));
    } finally {
      setRunning(false);
    }
  }

  return (
    <tr
      className={`border-t border-slate-100 transition-colors ${isDirty ? "bg-amber-50/70 shadow-[inset_3px_0_0_#f59e0b]" : ""}`}
      data-sync-task={schedule.task_type}
      data-run-now-stores={runNowStoreIds.join(",")}
      data-schedule-window={dateWindowTask ? dateWindowPreset : "snapshot"}
      data-schedule-strategy={describeScheduleStrategy(
        schedule.task_type,
        cronExpr,
        dateWindowTask ? dateWindowPreset : undefined,
      )}
    >
      <td
        className="px-4 py-2 font-semibold text-slate-900"
        title={describePreset(schedule.task_type)}
      >
        {SYNC_TYPE_LABEL[schedule.task_type] ?? schedule.task_type}
      </td>
      <td className="px-4 py-2 text-xs font-semibold text-slate-600">
        {schedulePageUsage(schedule.task_type)}
      </td>
      <td className="px-4 py-2 text-xs text-ink-sub">
        {dateWindowTask ? (
          <SelectNative
            value={dateWindowPreset}
            onChange={(e) =>
              setDateWindowPreset(e.target.value as ScheduleDateWindowPreset)
            }
            className="h-8 w-full min-w-0 rounded-md border border-line bg-white px-2 text-xs font-semibold text-slate-700"
          >
            {SCHEDULE_DATE_WINDOW_PRESETS.map((item) => (
              <option key={item.value} value={item.value}>
                {item.label}
              </option>
            ))}
          </SelectNative>
        ) : (
          scheduleTimeStrategy(schedule.task_type)
        )}
      </td>
      <td className="px-4 py-2 text-ink-sub">{schedule.dataSourceLabel}</td>
      <td className="px-4 py-2">
        <div className="flex items-center gap-2">
          <Input
            value={cronExpr}
            onChange={(e) => setCronExpr(e.target.value)}
            className="w-40 font-mono text-sm"
          />
          <span className="text-xs text-slate-500">
            {cronToHuman(cronExpr)}
          </span>
        </div>
      </td>
      <td className="px-4 py-2">
        <label className="flex items-center gap-2 text-xs font-semibold text-slate-600">
          <input
            type="checkbox"
            checked={enabled}
            onChange={(e) => setEnabled(e.target.checked)}
          />
          {enabled ? "启用" : "停用"}
        </label>
      </td>
      <td className="py-2 pl-2 pr-4">
        <div className="flex items-center gap-2">
          <Button
            variant={isDirty ? "default" : "outline"}
            size="sm"
            className="relative"
            aria-label={saving ? "保存中" : saved ? "已保存" : "保存"}
            onClick={() => void handleSave()}
            disabled={saveFeedback !== "idle" || running}
          >
            <span className={saveFeedback === "idle" ? undefined : "invisible"}>
              保存
            </span>
            {saving ? (
              <span className="absolute left-1/2 top-1/2 flex h-3.5 w-3.5 -translate-x-1/2 -translate-y-1/2 items-center justify-center">
                <Loader2 className="h-3.5 w-3.5 animate-spin" />
              </span>
            ) : null}
            {saved ? (
              <span className="absolute left-1/2 top-1/2 flex h-3.5 w-3.5 -translate-x-1/2 -translate-y-1/2 items-center justify-center">
                <Check className="h-3.5 w-3.5" />
              </span>
            ) : null}
          </Button>
          <Button
            variant="ghost"
            size="sm"
            onClick={() => void runNow()}
            disabled={saving || running}
          >
            <PlayCircle className="h-3.5 w-3.5" />
            {running ? "执行中…" : "立即执行"}
          </Button>
        </div>
      </td>
    </tr>
  );
}

function DataSourcePanel({
  dataSources,
  workerHealth,
  runtimeConfigRows,
  endpointLimitRows,
  canManageEndpointLimits,
}: {
  dataSources: DataSourceRow[];
  workerHealth: SyncWorkerHealthSnapshot | null;
  runtimeConfigRows: SyncRuntimeConfigRow[];
  endpointLimitRows: LingxingEndpointLimitRow[];
  canManageEndpointLimits: boolean;
}) {
  return (
    <section
      className="flex h-full min-h-0 flex-col"
      data-testid="sync-data-source-panel"
    >
      <SectionHeader
        icon={<Database className="h-4 w-4" />}
        title="数据源"
        description="同步中心只展示连接摘要；凭证维护仍在 API 配置页。"
      />
      <div className="min-h-0 flex-1 overflow-auto">
        <table className="min-w-full text-sm">
          <thead className="bg-slate-50 text-left text-xs font-extrabold uppercase tracking-wide text-slate-500">
            <tr>
              <th className="px-4 py-3">名称</th>
              <th className="px-4 py-3">Provider</th>
              <th className="px-4 py-3">账号</th>
              <th className="px-4 py-3">状态</th>
              <th className="px-4 py-3">更新时间</th>
            </tr>
          </thead>
          <tbody>
            {dataSources.length ? (
              dataSources.map((source) => (
                <tr key={source.id} className="border-t border-slate-100">
                  <td className="px-4 py-3">
                    <div className="font-semibold text-slate-900">
                      {source.label}
                    </div>
                    <div className="font-mono text-xs text-ink-sub">
                      {source.id}
                    </div>
                  </td>
                  <td className="px-4 py-3 text-ink-sub">{source.provider}</td>
                  <td className="px-4 py-3 text-ink-sub">
                    {source.account_slug || "—"}
                  </td>
                  <td className="px-4 py-3">
                    <StatusBadge
                      value={source.enabled ? "启用" : "停用"}
                      tone={source.enabled ? "success" : "neutral"}
                    />
                  </td>
                  <td className="px-4 py-3 text-ink-sub">
                    {formatDateTime(source.updated_at)}
                  </td>
                </tr>
              ))
            ) : (
              <tr>
                <td
                  colSpan={5}
                  className="px-4 py-8 text-center text-sm text-slate-500"
                >
                  暂无数据源。
                </td>
              </tr>
            )}
          </tbody>
        </table>
        <WorkerNetworkPanel health={workerHealth} />
        {canManageEndpointLimits && runtimeConfigRows.length ? (
          <RuntimeConfigPanel rows={runtimeConfigRows} />
        ) : null}
        {canManageEndpointLimits && endpointLimitRows.length ? (
          <EndpointLimitPanel rows={endpointLimitRows} />
        ) : null}
      </div>
    </section>
  );
}

function WorkerNetworkPanel({
  health,
}: {
  health: SyncWorkerHealthSnapshot | null;
}) {
  const statusLabel = health?.ok ? "在线" : "不可用";
  const statusTone = health?.ok ? "success" : "danger";
  const egressIp = health?.egressIp ?? "未获取";
  const checkedAt = health?.egressIpCheckedAt
    ? formatDateTime(health.egressIpCheckedAt)
    : "—";
  const queue = health?.queue ?? "—";
  const uptime =
    health?.uptime !== null && health?.uptime !== undefined
      ? formatDurationSeconds(health.uptime)
      : "—";
  const error =
    health?.egressIpError ??
    health?.error ??
    (health && !health.egressIpCheckedAt ? "探测中" : null);

  return (
    <section
      className="border-t border-line px-4 py-4"
      data-testid="sync-worker-network"
    >
      <div className="mb-3 flex items-center gap-2 text-sm font-extrabold text-slate-900">
        <span>同步 Worker 出口</span>
        <StatusBadge value={statusLabel} tone={statusTone} />
      </div>
      <div className="overflow-hidden rounded-md border border-line">
        <table className="min-w-full text-xs">
          <thead className="bg-slate-50 text-left font-bold text-slate-500">
            <tr>
              <th className="px-3 py-2">出口 IP</th>
              <th className="px-3 py-2">检查时间</th>
              <th className="px-3 py-2">队列</th>
              <th className="px-3 py-2">Worker 运行</th>
              <th className="px-3 py-2">状态</th>
            </tr>
          </thead>
          <tbody>
            <tr className="border-t border-slate-100">
              <td className="px-3 py-2 font-mono text-sm font-bold text-slate-900">
                {egressIp}
              </td>
              <td className="px-3 py-2 text-ink-sub">{checkedAt}</td>
              <td className="px-3 py-2 font-mono text-slate-900">{queue}</td>
              <td className="px-3 py-2 text-ink-sub">{uptime}</td>
              <td className="px-3 py-2 text-ink-sub">{error ?? "正常"}</td>
            </tr>
          </tbody>
        </table>
      </div>
    </section>
  );
}

function RuntimeConfigPanel({ rows }: { rows: SyncRuntimeConfigRow[] }) {
  return (
    <section
      className="border-t border-line px-4 py-4"
      data-testid="sync-runtime-config"
    >
      <div className="mb-3 flex items-center gap-2 text-sm font-extrabold text-slate-900">
        <span>运行时配置（需重启生效）</span>
      </div>
      <div className="overflow-hidden rounded-md border border-line">
        <table className="min-w-full text-xs">
          <thead className="bg-slate-50 text-left font-bold text-slate-500">
            <tr>
              <th className="px-3 py-2">参数</th>
              <th className="px-3 py-2">当前有效值</th>
              <th className="px-3 py-2">修改为</th>
              <th className="px-3 py-2">来源</th>
              <th className="px-3 py-2">范围</th>
              <th className="px-3 py-2">生产候选值</th>
              <th className="px-3 py-2">生效边界</th>
              <th className="px-3 py-2" />
            </tr>
          </thead>
          <tbody>
            {rows.map((row) => (
              <RuntimeConfigEditRow key={row.key} row={row} />
            ))}
          </tbody>
        </table>
      </div>
      <div className="mt-2 text-xs text-amber-700">
        修改后的生效边界以各行"生效边界"列为准。清空输入并保存可恢复默认值。
      </div>
    </section>
  );
}

function RuntimeConfigEditRow({ row }: { row: SyncRuntimeConfigRow }) {
  const router = useRouter();
  const [inputValue, setInputValue] = useState(row.configuredValue ?? "");
  const [saving, setSaving] = useState(false);
  const [saved, setSaved] = useState(false);

  const save = async () => {
    setSaving(true);
    try {
      const result = await reportRuntimeConfigSaveUnavailable({
        key: row.key,
        value: inputValue.trim(),
      });
      if (!result.ok) {
        toast.error(result.error ?? "保存失败");
        return;
      }
      setSaved(true);
      setTimeout(() => setSaved(false), 550);
      router.refresh();
    } finally {
      setSaving(false);
    }
  };

  const isDirty = inputValue.trim() !== (row.configuredValue ?? "");

  return (
    <tr
      className={`border-t border-slate-100${isDirty ? " bg-amber-50/60" : ""}`}
    >
      <td className="px-3 py-2">
        <div className="font-semibold text-slate-900">{row.label}</div>
        <div className="font-mono text-[11px] text-ink-sub">{row.key}</div>
      </td>
      <td className="px-3 py-2">
        <div className="flex items-center gap-2">
          <span className="font-mono text-sm font-bold text-slate-900">
            {row.effectiveValue}
          </span>
          {row.warning ? (
            <StatusBadge value="低于候选值" tone="warning" />
          ) : null}
        </div>
      </td>
      <td className="px-3 py-2">
        <Input
          value={inputValue}
          onChange={(e) => {
            setInputValue(e.target.value);
            setSaved(false);
          }}
          placeholder={String(row.effectiveValue)}
          className="h-8 w-36 font-mono text-xs"
          aria-label={`修改 ${row.label}`}
        />
      </td>
      <td className="px-3 py-2 text-ink-sub">{row.sourceLabel}</td>
      <td className="px-3 py-2 text-ink-sub">{row.rangeLabel}</td>
      <td className="px-3 py-2 font-mono text-slate-900">
        {row.recommendedValue}
      </td>
      <td className="px-3 py-2 text-ink-sub">{row.reloadLabel}</td>
      <td className="px-3 py-2">
        <Button
          type="button"
          size="sm"
          variant={isDirty ? "default" : "outline"}
          disabled={saving}
          onClick={() => void save()}
          aria-label={`保存 ${row.label}`}
        >
          {saving ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : saved ? (
            <Check className="h-4 w-4" />
          ) : (
            <Save className="h-4 w-4" />
          )}
          保存
        </Button>
      </td>
    </tr>
  );
}

function EndpointLimitPanel({ rows }: { rows: LingxingEndpointLimitRow[] }) {
  const grouped = useMemo(() => {
    const map = new Map<string, LingxingEndpointLimitRow[]>();
    for (const row of rows)
      map.set(row.dataSourceId, [...(map.get(row.dataSourceId) ?? []), row]);
    return [...map.entries()];
  }, [rows]);
  return (
    <section
      className="border-t border-line px-4 py-4"
      data-testid="lingxing-endpoint-limiter"
    >
      <div className="mb-3 flex items-center gap-2 text-sm font-extrabold text-slate-900">
        <span>接口限流（桶令牌数）</span>
      </div>
      <div className="space-y-4">
        {grouped.map(([dataSourceId, sourceRows]) => {
          const first = sourceRows[0]!;
          return (
            <div key={dataSourceId} className="rounded-md border border-line">
              <div className="flex items-center justify-between gap-3 border-b border-line bg-slate-50 px-3 py-2">
                <div>
                  <div className="text-sm font-bold text-slate-900">
                    {first.dataSourceName}
                  </div>
                  <div className="font-mono text-xs text-ink-sub">
                    {first.accountSlug || "未标记"} · app ****
                    {first.appIdLast4 || "----"}
                  </div>
                </div>
              </div>
              <table className="min-w-full text-xs">
                <thead className="text-left font-bold text-slate-500">
                  <tr>
                    <th className="px-3 py-2">接口</th>
                    <th className="px-3 py-2">Endpoint</th>
                    <th className="px-3 py-2">默认桶令牌数</th>
                    <th className="px-3 py-2">当前桶令牌数</th>
                    <th className="px-3 py-2">覆盖桶令牌数</th>
                    <th className="px-3 py-2">操作</th>
                  </tr>
                </thead>
                <tbody>
                  {sourceRows.map((row) => (
                    <EndpointLimitRow
                      key={`${row.dataSourceId}:${row.endpointUrl}`}
                      row={row}
                    />
                  ))}
                </tbody>
              </table>
            </div>
          );
        })}
      </div>
    </section>
  );
}

function EndpointLimitRow({ row }: { row: LingxingEndpointLimitRow }) {
  const router = useRouter();
  const [capacity, setCapacity] = useState(
    String(row.overrideCapacity ?? row.effectiveCapacity),
  );
  const [saving, setSaving] = useState(false);
  const save = async () => {
    setSaving(true);
    try {
      const result = await reportEndpointLimitSaveUnavailable({
        dataSourceId: row.dataSourceId,
        endpointUrl: row.endpointUrl,
        capacity: Number(capacity),
      });
      if (!result.ok) {
        toast.error(result.error ?? "保存失败");
        return;
      }
      toast.success("已保存");
      router.refresh();
    } finally {
      setSaving(false);
    }
  };
  return (
    <tr className="border-t border-slate-100">
      <td className="px-3 py-2 font-semibold text-slate-900">
        {row.label}
        <div className="text-[11px] font-normal text-ink-sub">
          {row.methods.join("/")}
        </div>
      </td>
      <td className="px-3 py-2 font-mono text-[11px] text-ink-sub">
        {row.endpointUrl}
      </td>
      <td className="px-3 py-2">{row.defaultCapacity}</td>
      <td className="px-3 py-2">{row.effectiveCapacity}</td>
      <td className="px-3 py-2">
        <Input
          type="number"
          min={1}
          max={row.editable ? 10 : row.effectiveCapacity}
          value={capacity}
          disabled={!row.editable}
          onChange={(event) => setCapacity(event.target.value)}
          className="h-8 w-20 text-xs"
          aria-label={`${row.label} 覆盖容量`}
        />
        {row.fixedReason ? (
          <div className="mt-1 text-[11px] text-ink-sub">{row.fixedReason}</div>
        ) : null}
      </td>
      <td className="px-3 py-2">
        <Button
          type="button"
          size="sm"
          variant="outline"
          disabled={!row.editable || saving}
          onClick={() => void save()}
        >
          {saving ? (
            <Loader2 className="h-4 w-4 animate-spin" />
          ) : (
            <Save className="h-4 w-4" />
          )}
          保存
        </Button>
      </td>
    </tr>
  );
}

function SectionHeader({
  icon,
  title,
  description,
  controls,
  actions,
}: {
  icon: ReactNode;
  title: string;
  description?: string;
  controls?: ReactNode;
  actions?: ReactNode;
}) {
  return (
    <div className="flex h-11 items-center border-b border-line px-5">
      <div className="flex w-full items-center justify-between gap-4">
        <div className="flex min-w-0 items-center gap-3">
          <div className="flex items-center gap-2 text-sm font-extrabold text-slate-900">
            <span className="text-primary">{icon}</span>
            <span>{title}</span>
            {description ? (
              <TitleHelp title={title} description={description} />
            ) : null}
          </div>
          {controls ? (
            <div
              data-testid="section-header-controls"
              className="sync-section-header-controls flex min-w-0 items-center gap-2 overflow-x-auto whitespace-nowrap"
            >
              {controls}
            </div>
          ) : null}
        </div>
        {actions ? (
          <div
            data-testid="section-header-actions"
            className="sync-section-header-actions flex min-w-0 shrink-0 items-center justify-end gap-2 overflow-x-auto whitespace-nowrap"
          >
            {actions}
          </div>
        ) : null}
      </div>
    </div>
  );
}

function TitleHelp({
  title,
  description,
}: {
  title: string;
  description: string;
}) {
  return (
    <span className="group/help relative inline-flex">
      <button
        type="button"
        className="inline-flex h-5 w-5 items-center justify-center rounded-full text-slate-400 transition hover:bg-slate-100 hover:text-primary focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-primary/30"
        aria-label={`${title}说明`}
        aria-describedby={`${title}-help`}
      >
        <HelpCircle className="h-3.5 w-3.5" />
      </button>
      <span
        id={`${title}-help`}
        data-testid="title-help-tooltip"
        role="tooltip"
        className="absolute left-0 top-full z-[80] mt-2 hidden w-[min(320px,calc(100vw-48px))] whitespace-normal break-words rounded-md border border-line bg-slate-900 px-3 py-2 text-xs font-medium leading-5 text-white shadow-lg group-hover/help:block group-focus-within/help:block"
      >
        {description}
      </span>
    </span>
  );
}

function buildParentGroups(
  runs: SyncRunView[],
  segments: SegmentRow[],
): ParentRunGroup[] {
  const childrenByParent = new Map<string, SyncRunView[]>();
  runs.forEach((run) => {
    if (!run.parent_run_id) return;
    childrenByParent.set(run.parent_run_id, [
      ...(childrenByParent.get(run.parent_run_id) ?? []),
      run,
    ]);
  });

  const parents = runs.filter((run) => !run.parent_run_id);
  return parents
    .map((run) => {
      const children = (childrenByParent.get(run.run_id) ?? []).sort(
        (left, right) => {
          const leftScope = parseScope(left.target_scope_json);
          const rightScope = parseScope(right.target_scope_json);
          return String(
            leftScope.chunkLabel ?? leftScope.startDate ?? left.created_at,
          ).localeCompare(
            String(
              rightScope.chunkLabel ?? rightScope.startDate ?? right.created_at,
            ),
          );
        },
      );
      const relatedRunIds = new Set([
        run.run_id,
        ...children.map((child) => child.run_id),
      ]);
      const relatedSegments = segments.filter((segment) =>
        relatedRunIds.has(segment.run_id),
      );
      const rowEvidence = summarizeRowEvidence(relatedSegments);
      return {
        run,
        scope: parseScope(run.target_scope_json),
        children,
        segments: relatedSegments,
        childStatusSummary: summarizeStatuses(
          children.map((child) => child.status),
        ),
        rowEvidence,
        rowCount: rowEvidence.primary,
        ...buildDebugMessage(run, children, relatedSegments),
      };
    })
    .sort(
      (left, right) =>
        new Date(right.run.created_at).getTime() -
        new Date(left.run.created_at).getTime(),
    );
}

function coerceRuns(rows: unknown[]): SyncRunView[] {
  return rows
    .filter((row): row is Record<string, unknown> =>
      Boolean(row && typeof row === "object"),
    )
    .map((row) => ({
      ...(row as unknown as SyncRunView),
      parent_run_id:
        row.parent_run_id == null ? null : String(row.parent_run_id),
      updated_at: row.updated_at == null ? null : String(row.updated_at),
      lease_until: row.lease_until == null ? null : String(row.lease_until),
      heartbeat_at: row.heartbeat_at == null ? null : String(row.heartbeat_at),
    }));
}

function parseScope(value: unknown): RunScope {
  return value && typeof value === "object" ? (value as RunScope) : {};
}

function triggerActorLabel(scope: RunScope): string {
  if (scope.triggerSource === "cron" || scope.triggerSource === "scheduled") {
    return "定时";
  }
  if (scope.triggerSource === "system_test") {
    return "系统测试";
  }
  const actor = triggerActorName(scope.triggeredBy);
  if (actor) return actor;
  if (requiresTriggerActor(scope.triggerSource)) return "缺触发人";
  return "-";
}

function requiresTriggerActor(source: unknown): boolean {
  return (
    source === "manual" ||
    source === "admin_manual" ||
    source === "page_manual" ||
    source === "retry"
  );
}

function triggerActorName(value: unknown): string | null {
  if (typeof value === "string") return value.trim() || null;
  if (!value || typeof value !== "object") return null;
  const record = value as Record<string, unknown>;
  const username = firstNonEmpty(record.name, record.username);
  if (username) return String(username);
  const userId = firstNonEmpty(record.userId);
  if (userId) return `ID ${String(userId)}`;
  return null;
}

function summarizeStatuses(statuses: string[]) {
  return Array.from(
    statuses
      .reduce(
        (map, status) => map.set(status, (map.get(status) ?? 0) + 1),
        new Map<string, number>(),
      )
      .entries(),
  ).map(([status, count]) => ({ status, count }));
}

function scopeLabel(scope: RunScope) {
  const scoped = scope as Record<string, unknown>;
  const start = firstNonEmpty(scope.startDate, scoped.start, scoped.start_at);
  const end = firstNonEmpty(scope.endDate, scoped.end, scoped.end_at);
  const chunk = firstNonEmpty(
    scope.chunkLabel,
    scoped.month,
    scoped.windowLabel,
  );
  if (chunk) return String(chunk);
  if (start && end) return `${start} ~ ${end}`;
  if (start) return String(start);
  return scope.mode ? `模式 ${scope.mode}` : "全量 / 即时";
}

function collectStoreLabel(
  scope: RunScope,
  storeLabels?: StoreLabelMap,
): StoreLabel {
  const raw = scope.storeIds ?? scope.stores ?? scope.storeId;
  if (Array.isArray(raw)) {
    const labels = raw.map((item) =>
      resolveStoreLabel(scope, String(item), storeLabels),
    );
    const ids = labels
      .map((label) => label.id)
      .filter((id): id is string => Boolean(id));
    return {
      id: ids.length ? ids.join(", ") : null,
      name: labels.map((label) => label.name).join(", "),
      text: labels.map((label) => label.text).join(", "),
    };
  }
  if (raw !== undefined && raw !== null)
    return resolveStoreLabel(scope, String(raw), storeLabels);
  return { id: null, name: "全部", text: "全部" };
}

function resolveStoreLabel(
  scope: RunScope,
  storeId: string,
  storeLabels?: StoreLabelMap,
): StoreLabel {
  const dataSourceId =
    typeof scope.dataSourceId === "string" ? scope.dataSourceId : "";
  const channelType =
    typeof scope.channelType === "string" ? scope.channelType : "";
  return (
    storeLabels?.get(storeLabelKey(dataSourceId, storeId, channelType)) ??
    storeLabels?.get(storeLabelKey(dataSourceId, storeId)) ??
    storeLabels?.get(storeLabelKey("", storeId)) ?? {
      id: null,
      name: storeId,
      text: storeId,
    }
  );
}

function extractRowCount(value: unknown) {
  return summarizeEvidenceValue(value).primary;
}

function summarizeRowEvidence(segments: SegmentRow[]) {
  return segments.reduce((summary, segment) => {
    const evidence = summarizeEvidenceValue(segment.response_evidence_json);
    summary.written += evidence.written;
    summary.matched += evidence.matched;
    summary.staged += evidence.staged;
    summary.fetched += evidence.fetched;
    summary.promoted += evidence.promoted;
    summary.inserted += evidence.inserted;
    summary.updated += evidence.updated;
    summary.unchanged += evidence.unchanged;
    summary.deleted += evidence.deleted;
    summary.classified += evidence.classified;
    summary.generic += evidence.generic;
    summary.primary += evidence.primary;
    summary.atomCount += evidence.atomCount;
    summary.successAtoms += evidence.successAtoms;
    summary.attemptCount += evidence.attemptCount;
    summary.successAttempts += evidence.successAttempts;
    summary.rowEvidenceAttempts += evidence.rowEvidenceAttempts;
    summary.evidenceSegments += evidence.hasRowEvidence ? 1 : 0;
    summary.fetchedEvidenceSegments += evidence.fetchedKnown ? 1 : 0;
    summary.writtenEvidenceSegments += evidence.writtenKnown ? 1 : 0;
    summary.changeSummarySegments += evidence.changeSummaryAvailable ? 1 : 0;
    summary.incompleteChangeSummarySegments +=
      evidence.hasRowEvidence && !evidence.changeSummaryAvailable ? 1 : 0;
    return summary;
  }, emptyEvidenceSummary());
}

function summarizeEvidenceValue(value: unknown) {
  const row = parseEvidenceRecord(value);
  if (!row) return emptyEvidenceSummary();
  const writtenValue = firstEvidenceNumber(row.written_rows, row.writtenRows);
  const fetchedValue = firstEvidenceNumber(row.fetched_rows, row.fetchedRows);
  const insertedValue = firstEvidenceNumber(row.inserted_rows);
  const updatedValue = firstEvidenceNumber(row.updated_rows);
  const unchangedValue = firstEvidenceNumber(row.unchanged_rows);
  const deletedValue = firstEvidenceNumber(row.deleted_rows);
  const classifiedValue = firstEvidenceNumber(row.classified_rows);
  const written = writtenValue ?? 0;
  const matched = firstPositiveNumber(row.matched_rows, row.matchedRows);
  const staged = firstPositiveNumber(row.staged_rows, row.stagedRows);
  const fetched = fetchedValue ?? 0;
  const promoted = firstPositiveNumber(row.promoted_rows, row.promotedRows);
  const generic = firstPositiveNumber(row.rowCount, row.count, row.rows);
  const changeSummaryAvailable =
    row.change_summary_available === true &&
    fetchedValue !== null &&
    insertedValue !== null &&
    updatedValue !== null &&
    unchangedValue !== null &&
    deletedValue !== null &&
    classifiedValue !== null &&
    classifiedValue ===
      insertedValue + updatedValue + unchangedValue + deletedValue;
  const rowEvidenceAttempts = nonNegativeInt(
    firstNonEmpty(row.row_evidence_attempts, row.rowEvidenceAttempts),
  );
  const hasRowEvidence =
    changeSummaryAvailable ||
    fetchedValue !== null ||
    writtenValue !== null ||
    matched > 0 ||
    staged > 0 ||
    promoted > 0 ||
    generic > 0 ||
    rowEvidenceAttempts > 0;
  return {
    written,
    matched,
    staged,
    fetched,
    promoted,
    inserted: insertedValue ?? 0,
    updated: updatedValue ?? 0,
    unchanged: unchangedValue ?? 0,
    deleted: deletedValue ?? 0,
    classified: classifiedValue ?? 0,
    generic,
    primary: firstPositiveNumber(generic, written, matched, staged, fetched),
    atomCount: nonNegativeInt(firstNonEmpty(row.atom_count, row.atomCount)),
    successAtoms: nonNegativeInt(
      firstNonEmpty(row.success_atoms, row.successAtoms),
    ),
    attemptCount: nonNegativeInt(
      firstNonEmpty(row.attempt_count, row.attemptCount),
    ),
    successAttempts: nonNegativeInt(
      firstNonEmpty(row.success_attempts, row.successAttempts),
    ),
    rowEvidenceAttempts,
    hasRowEvidence,
    fetchedKnown: fetchedValue !== null,
    writtenKnown: writtenValue !== null,
    changeSummaryAvailable,
    evidenceSegments: 0,
    fetchedEvidenceSegments: 0,
    writtenEvidenceSegments: 0,
    changeSummarySegments: 0,
    incompleteChangeSummarySegments: 0,
  };
}

function emptyEvidenceSummary(): RowEvidenceSummary {
  return {
    written: 0,
    matched: 0,
    staged: 0,
    fetched: 0,
    promoted: 0,
    inserted: 0,
    updated: 0,
    unchanged: 0,
    deleted: 0,
    classified: 0,
    generic: 0,
    primary: 0,
    atomCount: 0,
    successAtoms: 0,
    attemptCount: 0,
    successAttempts: 0,
    rowEvidenceAttempts: 0,
    evidenceSegments: 0,
    fetchedEvidenceSegments: 0,
    writtenEvidenceSegments: 0,
    changeSummarySegments: 0,
    incompleteChangeSummarySegments: 0,
    hasRowEvidence: false,
    fetchedKnown: false,
    writtenKnown: false,
    changeSummaryAvailable: false,
  };
}

function firstEvidenceNumber(...values: unknown[]) {
  for (const value of values) {
    if (value === undefined || value === null || value === "") continue;
    const numberValue = Number(value);
    if (Number.isSafeInteger(numberValue) && numberValue >= 0)
      return numberValue;
  }
  return null;
}

function firstPositiveNumber(...values: unknown[]) {
  for (const value of values) {
    const numberValue = Number(value ?? 0);
    if (Number.isFinite(numberValue) && numberValue > 0) return numberValue;
  }
  return 0;
}

function parseEvidenceRecord(value: unknown): Record<string, unknown> | null {
  if (value && typeof value === "object" && !Array.isArray(value))
    return value as Record<string, unknown>;
  if (typeof value !== "string" || !value.trim()) return null;
  try {
    const parsed = JSON.parse(value) as unknown;
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : null;
  } catch {
    return null;
  }
}

function segmentSummary(
  group: ParentRunGroup,
  status = groupDisplayStatus(group),
) {
  if (
    group.children.length &&
    isAtomCompatRun(group.run) &&
    COMPOUND_PROFILE_SYNC_TYPES.has(group.run.sync_type)
  )
    return childProgressSummary(group);
  if (isActiveStatus(status)) return activeStatusSummary(status);
  if (status === "success") {
    if (!group.segments.length) return "同步完成";
    return completedEvidenceSummary(group.rowEvidence);
  }
  if (status === "error" || status === "failed") return "同步失败";
  if (status === "cancelled" || status === "stale") return "已取消";
  return "同步中";
}

function childRunSummary(run: SyncRunView, segments: SegmentRow[]) {
  const rowEvidence = summarizeRowEvidence(segments);
  if (isActiveStatus(run.status)) return activeStatusSummary(run.status);
  if (run.status === "success") return completedEvidenceSummary(rowEvidence);
  if (run.status === "error" || run.status === "failed") return "同步失败";
  if (run.status === "cancelled" || run.status === "stale") return "已取消";
  return "同步中";
}

function activeStatusSummary(status: string) {
  const labels: Record<string, string> = {
    queued: "等待执行",
    pending: "等待规划",
    resource_ready: "等待调度",
    waiting_resource: "等待资源",
    admitted: "等待执行",
    running: "同步中",
    retry_wait: "等待重试",
    cooldown: "冷却中",
    paused: "已暂停",
  };
  return labels[status] ?? "同步中";
}

function completionSummaryText(summary: string, _debugText: string) {
  return summary.trim();
}

function completedEvidenceSummary(evidence: RowEvidenceSummary) {
  return evidenceSummaryText(evidence);
}

function evidenceSummaryText(evidence: RowEvidenceSummary) {
  if (hasCompleteChangeSummary(evidence)) return changeSummaryText(evidence);
  if (evidence.evidenceSegments) return legacyEvidenceSummaryText(evidence);
  return "同步完成";
}

function hasCompleteChangeSummary(evidence: RowEvidenceSummary) {
  return (
    evidence.evidenceSegments > 0 &&
    evidence.changeSummarySegments === evidence.evidenceSegments &&
    evidence.incompleteChangeSummarySegments === 0 &&
    evidence.fetchedEvidenceSegments === evidence.evidenceSegments &&
    evidence.classified ===
      evidence.inserted +
        evidence.updated +
        evidence.unchanged +
        evidence.deleted &&
    evidence.classified <= evidence.fetched &&
    evidence.deleted === 0
  );
}

function changeSummaryText(evidence: RowEvidenceSummary) {
  if (evidence.fetched === 0) return "上游返回 0 条";
  if (evidence.classified === 0) return "未匹配当前范围";
  if (evidence.inserted === 0 && evidence.updated === 0)
    return `已同步 ${evidence.classified} 条（无变化）`;
  return `已同步 ${evidence.classified} 条（${evidence.inserted} 新增 · ${evidence.updated} 更新 · ${evidence.unchanged} 无变化）`;
}

function legacyEvidenceSummaryText(evidence: RowEvidenceSummary) {
  return `已同步 ${legacySyncedRows(evidence)} 条`;
}

function legacySyncedRows(evidence: RowEvidenceSummary) {
  if (hasCompleteWrittenEvidence(evidence)) return evidence.written;
  if (hasCompleteFetchedEvidence(evidence)) return evidence.fetched;
  return firstPositiveNumber(
    evidence.generic,
    evidence.matched,
    evidence.staged,
    evidence.promoted,
  );
}

function hasCompleteFetchedEvidence(evidence: RowEvidenceSummary) {
  return (
    evidence.evidenceSegments > 0 &&
    evidence.fetchedEvidenceSegments === evidence.evidenceSegments
  );
}

function hasCompleteWrittenEvidence(evidence: RowEvidenceSummary) {
  return (
    evidence.evidenceSegments > 0 &&
    evidence.writtenEvidenceSegments === evidence.evidenceSegments
  );
}

function childProgressSummary(group: ParentRunGroup) {
  const total = group.children.length;
  const success = group.children.filter(
    (child) => child.status === "success",
  ).length;
  const failed = group.children.filter(
    (child) =>
      child.status === "error" ||
      child.status === "failed" ||
      child.status === "stale",
  ).length;
  return failed
    ? `${success}/${total} 已完成 · ${failed} 失败`
    : `${success}/${total} 已完成`;
}

function completionTextClass(status: string, children: SyncRunView[] = []) {
  const completedChildren = children.filter(
    (child) => child.status === "success",
  ).length;
  const failedChildren = children.filter(
    (child) =>
      child.status === "error" ||
      child.status === "failed" ||
      child.status === "stale",
  ).length;
  if (completedChildren > 0 && failedChildren > 0) return "text-amber-700";
  const failed = status === "error" || status === "failed";
  const childFailed = failedChildren > 0;
  return failed || childFailed ? "text-danger" : "text-slate-700";
}

function atomCoverage(scope: RunScope) {
  const raw = (scope as Record<string, unknown>).atomCoverage;
  if (!raw || typeof raw !== "object") return null;
  const coverage = raw as Record<string, unknown>;
  const total = nonNegativeInt(coverage.total);
  if (total <= 0) return null;
  return {
    total,
    success: nonNegativeInt(coverage.success),
    pending: nonNegativeInt(coverage.pending),
    resource_ready: nonNegativeInt(coverage.resource_ready),
    admitted: nonNegativeInt(coverage.admitted),
    running: nonNegativeInt(coverage.running),
    waiting_resource: nonNegativeInt(coverage.waiting_resource),
    retry_wait: nonNegativeInt(coverage.retry_wait),
    error: nonNegativeInt(coverage.error),
  };
}

function nonNegativeInt(value: unknown) {
  const numberValue = Number(value ?? 0);
  if (!Number.isFinite(numberValue) || numberValue < 0) return 0;
  return Math.floor(numberValue);
}

function buildDebugText(
  run: SyncRunView,
  children: SyncRunView[],
  segments: SegmentRow[],
) {
  return buildDebugMessage(run, children, segments).debugText;
}

function buildDebugMessage(
  run: SyncRunView,
  children: SyncRunView[],
  segments: SegmentRow[],
): { debugText: string; debugTone: ReasonTone } {
  const syncTypeByRunId = new Map<string, string>([
    [run.run_id, run.sync_type],
    ...children.map((child): [string, string] => [
      child.run_id,
      child.sync_type,
    ]),
  ]);
  const rawParts = [
    { reason: run.reason_code, syncType: run.sync_type },
    { reason: run.cancelled_reason, syncType: run.sync_type },
    ...children.flatMap((child) => [
      { reason: child.reason_code, syncType: child.sync_type },
      { reason: child.cancelled_reason, syncType: child.sync_type },
    ]),
    ...segments.map((segment) => ({
      reason: segmentEvidenceText(segment.response_evidence_json),
      syncType: syncTypeByRunId.get(segment.run_id) ?? run.sync_type,
    })),
  ].filter((item): item is { reason: string; syncType: string } =>
    Boolean(item.reason && item.reason.trim()),
  );
  const uniqueParts: Array<{ reason: string; syncType: string }> = [];
  for (const part of rawParts) {
    const normalized = normalizeFailureReason(part.reason);
    if (!normalized) continue;
    for (const reasonPart of splitKnownReasonParts(normalized)) {
      if (isAggregateReason(reasonPart)) continue;
      const existingIndex = uniqueParts.findIndex(
        (item) =>
          item.syncType === part.syncType &&
          isEquivalentFailureReason(item.reason, reasonPart),
      );
      if (existingIndex === -1) {
        uniqueParts.push({ reason: reasonPart, syncType: part.syncType });
      } else {
        uniqueParts[existingIndex] = {
          reason: chooseMoreCompleteFailureReason(
            uniqueParts[existingIndex]!.reason,
            reasonPart,
          ),
          syncType: part.syncType,
        };
      }
    }
  }
  const displayReasons = uniqueParts.map((part) =>
    formatDisplayReason(part.reason, part.syncType),
  );
  return {
    debugText: displayReasons.map((reason) => reason.text).join(" · "),
    debugTone: strongestReasonTone(displayReasons.map((reason) => reason.tone)),
  };
}

function isAggregateReason(reason: string) {
  return [
    "children_partial_error",
    "children_cancelled",
    "children_success",
    "completed",
    "started",
  ].includes(reason);
}

function normalizeFailureReason(reason: string) {
  return reason
    .replace(/\s+request_id=[^\s·]+/gi, "")
    .replace(/\s+request_?i?d?$/gi, "")
    .replace(/\s*·\s*HTTP\s+\d{3}\b/g, "")
    .replace(/\s*·\s*(lingxing|openapi|upstream)\s*$/gi, "")
    .trim();
}

function isEquivalentFailureReason(left: string, right: string) {
  if (left === right) return true;
  const leftLingxing = parseLingxingFailure(left);
  const rightLingxing = parseLingxingFailure(right);
  if (!leftLingxing || !rightLingxing) return false;
  if (leftLingxing.code !== rightLingxing.code) return false;
  if (!leftLingxing.message || !rightLingxing.message) return true;
  return (
    leftLingxing.message.startsWith(rightLingxing.message) ||
    rightLingxing.message.startsWith(leftLingxing.message)
  );
}

function chooseMoreCompleteFailureReason(left: string, right: string) {
  return right.length > left.length ? right : left;
}

function parseLingxingFailure(reason: string) {
  const match = reason.match(
    /^Lingxing API error code=(\d+)(?:\s+message=([^·]+))?/i,
  );
  if (!match) return null;
  return { code: match[1], message: (match[2] ?? "").trim() };
}

function splitKnownReasonParts(reason: string) {
  const parts = reason
    .split(/\s*·\s*/)
    .map((part) => part.trim())
    .filter(Boolean);
  if (parts.length <= 1) return [reason];
  return parts.every(isKnownReasonCode) ? parts : [reason];
}

function isKnownReasonCode(reason: string) {
  return [
    "no_business_atoms_planned",
    "atom_coverage_missing",
    "atom_failed",
    "atom_incomplete",
    "compound_incomplete",
    "planner_not_closed",
    "user_cancelled",
    "cancelled",
  ].includes(reason);
}

function formatDisplayReason(
  reason: string,
  syncType = "",
): { text: string; tone: ReasonTone } {
  const lower = reason.toLowerCase();
  if (
    lower.includes("deadlock found when trying to get lock") ||
    lower.includes("lock wait timeout exceeded")
  ) {
    return { text: "数据库忙，写入冲突，稍后重试", tone: "danger" };
  }
  if (
    /\bhttp\s+\d{3}\b/i.test(reason) ||
    lower.includes("openapi.lingxing.com") ||
    /^erp_\d+/i.test(reason)
  ) {
    return { text: reason, tone: "danger" };
  }
  if (reason === "no_business_atoms_planned") {
    return syncType === "sync:vc-ads"
      ? { text: "广告请求未提交：未生成可执行广告请求", tone: "warning" }
      : { text: "没有可同步内容", tone: "warning" };
  }
  if (reason === "atom_coverage_missing")
    return { text: "同步任务不完整", tone: "danger" };
  if (reason === "atom_failed")
    return { text: "部分子任务失败", tone: "danger" };
  if (reason === "permanent_target_blocked")
    return { text: "永久目标已阻断：需人工确认后强制重试", tone: "danger" };
  if (reason === "atom_incomplete")
    return { text: "子任务进行中", tone: "warning" };
  if (reason === "compound_incomplete")
    return { text: "组合任务未完成", tone: "warning" };
  if (reason === "planner_not_closed")
    return { text: "任务准备中", tone: "info" };
  if (reason === "user_cancelled")
    return { text: "用户已取消", tone: "neutral" };
  if (reason === "cancelled") return { text: "已取消", tone: "neutral" };
  return { text: reason, tone: defaultReasonTone(reason) };
}

function defaultReasonTone(reason: string): ReasonTone {
  const lower = reason.toLowerCase();
  if (
    [
      "error",
      "failed",
      "fail",
      "exception",
      "upstream_5xx",
      "permanent_error",
    ].some((token) => lower.includes(token))
  )
    return "danger";
  if (["no_", "empty"].some((token) => lower.includes(token))) return "warning";
  return "neutral";
}

function strongestReasonTone(tones: ReasonTone[]): ReasonTone {
  if (tones.includes("danger")) return "danger";
  if (tones.includes("warning")) return "warning";
  if (tones.includes("info")) return "info";
  return "neutral";
}

function reasonTextClass(tone: ReasonTone) {
  if (tone === "danger") return "text-danger";
  if (tone === "warning") return "text-amber-700";
  if (tone === "info") return "text-primary";
  return "text-slate-600";
}

function splitSyncTypeLabel(label: string) {
  const [primary, ...rest] = label.split(/\s*·\s*/).filter(Boolean);
  return {
    primary: primary ?? label,
    secondary: rest.join(" "),
  };
}

function schedulePageUsage(syncType: string) {
  if (
    [
      "sync:sc-sales",
      "sync:sc-ads",
      "sync:sc-performance",
      "sync:sc-returns",
      "sync:sc-inventory",
    ].includes(syncType)
  )
    return "FBA 链接 / 运营日志";
  if (
    [
      "sync:vc-sales",
      "sync:vc-traffic",
      "sync:vc-inventory",
      "sync:vc-ads",
    ].includes(syncType)
  )
    return "VC 链接 / 运营日志";
  if (
    [
      "sync:fba-listings",
      "sync:sc-ads-account",
      "sync:sc-ads-hsa-creative-mapping",
    ].includes(syncType)
  )
    return "FBA 链接";
  if (
    ["sync:vc-realtime", "sync:vc-margin", "sync:vc-invoices"].includes(
      syncType,
    )
  )
    return "VC 链接";
  if (syncType === "sync:po-orders") return "订单";
  if (syncType === "sync:df-orders") return "订单";
  if (["sync:products", "sync:pairing"].includes(syncType)) return "产品基础";
  return "同步中心";
}

function scheduleTimeStrategy(syncType: string) {
  if (
    ["sync:fba-listings", "sync:sc-inventory", "sync:sc-performance"].includes(
      syncType,
    )
  )
    return "当前快照";
  if (
    [
      "sync:products",
      "sync:pairing",
      "sync:sc-ads-hsa-creative-mapping",
    ].includes(syncType)
  )
    return "全量/基础";
  if (syncType === "sync:vc-invoices") return "当前状态";
  return "日期窗口";
}

function scheduleUsesDateWindow(syncType: string) {
  return scheduleTimeStrategy(syncType) === "日期窗口";
}

function readScheduleDateWindowPreset(
  schedule: Pick<SyncScheduleRow, "task_type" | "schedule_payload_json">,
): ScheduleDateWindowPreset {
  if (!scheduleUsesDateWindow(schedule.task_type)) return "yesterday";
  const payload = parseSchedulePayload(schedule.schedule_payload_json);
  const value =
    payload && typeof payload === "object" && "dateWindowPreset" in payload
      ? String(
          (payload as { dateWindowPreset?: unknown }).dateWindowPreset ?? "",
        )
      : "";
  return isScheduleDateWindowPreset(value) ? value : "yesterday";
}

function parseSchedulePayload(value: unknown): Record<string, unknown> {
  if (value && typeof value === "object" && !Array.isArray(value))
    return value as Record<string, unknown>;
  if (typeof value !== "string" || !value.trim()) return {};
  try {
    const parsed = JSON.parse(value) as unknown;
    return parsed && typeof parsed === "object" && !Array.isArray(parsed)
      ? (parsed as Record<string, unknown>)
      : {};
  } catch {
    return {};
  }
}

function isScheduleDateWindowPreset(
  value: string,
): value is ScheduleDateWindowPreset {
  return (
    value === "today" ||
    value === "yesterday" ||
    value === "last3" ||
    value === "last7" ||
    value === "last30"
  );
}

function scheduleDateWindowLabel(value: ScheduleDateWindowPreset | undefined) {
  return (
    SCHEDULE_DATE_WINDOW_PRESETS.find((item) => item.value === value)?.label ??
    "昨天 1 天"
  );
}

function schedulePresetToManualRange(value: ScheduleDateWindowPreset) {
  if (value === "last30") return "30d";
  return "7d";
}

function scheduleMatchesFilter(syncType: string, filter: string) {
  const usage = schedulePageUsage(syncType);
  if (filter === "fba") return usage.includes("FBA 链接");
  if (filter === "vc") return usage.includes("VC 链接");
  if (filter === "operations") return usage.includes("运营日志");
  if (filter === "orders") return usage === "订单";
  if (filter === "foundation") return usage === "产品基础";
  return true;
}

function normalizeScheduleFilter(value: string | null | undefined) {
  const filters: ReadonlySet<string> = new Set(
    SCHEDULE_FILTERS.map((filter) => filter.value),
  );
  return value && filters.has(value) ? value : "all";
}

function segmentEvidenceText(value: unknown) {
  const evidence = parseEvidenceRecord(value);
  if (!evidence) return "";
  const statusCode = Number(evidence.status_code ?? evidence.statusCode ?? 0);
  const explicitError = firstNonEmpty(
    evidence.error_message,
    evidence.error,
    evidence.reason,
  );
  const messageValue =
    statusCode >= 400
      ? firstNonEmpty(explicitError, evidence.message)
      : explicitError;
  const message = messageValue ? String(messageValue) : "";
  if (!message && statusCode < 400) return "";
  const statusText =
    statusCode >= 400 && !message.includes(`HTTP ${statusCode}`)
      ? `HTTP ${statusCode}`
      : null;
  const upstream = firstNonEmpty(evidence.upstream);
  const upstreamText =
    upstream && !message.includes(String(upstream)) ? String(upstream) : null;
  return [message, statusText, upstreamText].filter(Boolean).join(" · ");
}

function taskUnitCode(run: SyncRunView) {
  const scope = parseScope(run.target_scope_json);
  return String(
    firstNonEmpty(scope.chunkLabel, scope.month, run.run_id.slice(-6)) ??
      run.run_id,
  );
}

function standaloneTaskLabel(run: SyncRunView) {
  return `独立任务 ${taskUnitCode(run)}`;
}

function childTaskUnitLabel(run: SyncRunView) {
  return `子任务单元 ${taskUnitCode(run)}`;
}

function isActiveStatus(status: string) {
  return [
    "queued",
    "pending",
    "resource_ready",
    "waiting_resource",
    "admitted",
    "running",
    "retry_wait",
    "cooldown",
    "paused",
  ].includes(status);
}

function groupDisplayStatus(group: Pick<ParentRunGroup, "children" | "run">) {
  const statuses = [
    group.run.status,
    ...group.children.map((child) => child.status),
  ];
  return (
    [
      "running",
      "admitted",
      "resource_ready",
      "waiting_resource",
      "retry_wait",
      "pending",
      "queued",
    ].find((status) => statuses.includes(status)) ?? group.run.status
  );
}

function canCancelGroup(group: ParentRunGroup) {
  return runActionIdsForGroup(group, "cancel").length > 0;
}

function canRetryGroup(group: ParentRunGroup) {
  return runActionIdsForGroup(group, "retry").length > 0;
}

function runActionIdsForGroups(
  groups: ParentRunGroup[],
  action: SyncRunActionKind,
) {
  return Array.from(
    new Set(groups.flatMap((group) => runActionIdsForGroup(group, action))),
  );
}

function runActionIdsForGroup(
  group: ParentRunGroup,
  action: SyncRunActionKind,
) {
  if (action === "cancel") {
    if (isActiveStatus(group.run.status)) return [group.run.run_id];
    return group.children
      .filter((child) => isActiveStatus(child.status))
      .map((child) => child.run_id);
  }
  const retryableChildren = group.children
    .filter((child) => isRetryableStatus(child.status))
    .map((child) => child.run_id);
  if (isAtomCompatRun(group.run) && retryableChildren.length)
    return retryableChildren;
  if (isRetryableStatus(group.run.status)) return [group.run.run_id];
  return retryableChildren;
}

function isRetryableStatus(status: string) {
  return ["error", "cancelled", "stale"].includes(status);
}

function isAtomCompatRun(run: SyncRunView) {
  return parseScope(run.target_scope_json).executionTruth === "atom";
}

function isCompoundProfileGroup(group: ParentRunGroup) {
  return (
    group.children.length > 0 &&
    isAtomCompatRun(group.run) &&
    COMPOUND_PROFILE_SYNC_TYPES.has(group.run.sync_type)
  );
}

function buildAtomResubmitHref(run: SyncRunView, scope: RunScope) {
  const storeIds = Array.isArray(scope.storeIds)
    ? scope.storeIds.join(",")
    : typeof scope.storeId === "string"
      ? scope.storeId
      : undefined;
  return buildSyncCenterHref({
    type: run.sync_type,
    dataSourceId:
      typeof scope.dataSourceId === "string" ? scope.dataSourceId : undefined,
    stores: storeIds,
    start:
      typeof scope.startDate === "string"
        ? scope.startDate
        : typeof scope.start === "string"
          ? scope.start
          : undefined,
    end:
      typeof scope.endDate === "string"
        ? scope.endDate
        : typeof scope.end === "string"
          ? scope.end
          : undefined,
    from: "sync-log",
  });
}

function isExpiredActiveLease(run: SyncRunView) {
  if (!isActiveStatus(run.status) || !run.lease_until) return false;
  const expiresAt = new Date(run.lease_until).getTime();
  return Number.isFinite(expiresAt) && expiresAt < Date.now();
}

function buildSyncCenterHref({
  type,
  dataSourceId,
  store,
  stores,
  range,
  start,
  end,
  from,
}: {
  type: string;
  dataSourceId?: string;
  store?: string;
  stores?: string;
  range?: string;
  start?: string;
  end?: string;
  from?: string;
}) {
  const params = new URLSearchParams();
  params.set("type", type);
  if (dataSourceId) params.set("dataSourceId", dataSourceId);
  if (store) params.set("store", store);
  if (stores) params.set("stores", stores);
  if (range) params.set("range", range);
  if (start) params.set("start", start);
  if (end) params.set("end", end);
  if (from) params.set("from", from);
  return `/admin/sync?${params.toString()}`;
}

function describePreset(syncType: string) {
  if (syncType === "sync:stores") return "拉取集成连接页的店铺目录与通道信息";
  if (syncType === "sync:pairing")
    return "按配置页已勾选店铺执行，刷新产品总览页的 ASIN-listing 配对数据";
  if (syncType === "sync:products") return "刷新产品总览页的产品主数据";
  if (syncType === "sync:fba-listings") return "刷新 FBA 链接页的 Listing 缓存";
  if (syncType === "sync:sc-sales") return "补充 FBA 链接页的 SC 销量事实表";
  if (syncType === "sync:sc-returns") return "补充 FBA 链接页的 SC 退货事实表";
  if (syncType === "sync:sc-ads") return "补充 FBA 链接页的 SC 广告事实表";
  if (syncType === "sync:sc-performance") return "补充运营日志的产品表现事实表";
  if (syncType === "sync:sc-ads-hsa-creative-mapping")
    return "同步 HSA 创意到 ASIN 映射，供广告归因使用";
  if (syncType === "sync:sc-ads-account")
    return "补充 FBA 链接页的账户级广告汇总";
  if (syncType === "sync:sc-inventory")
    return "补充 FBA 链接页的 SC 库存快照事实表";
  if (syncType === "sync:vc-sales") return "补充 VC 链接页的销量事实表";
  if (syncType === "sync:vc-traffic")
    return "补充 VC 链接页和运营日志的访问量事实表";
  if (syncType === "sync:vc-inventory") return "补充 VC 链接页的库存事实表";
  if (syncType === "sync:vc-realtime")
    return "补充 VC 链接页的小时级实时销量事实表";
  if (syncType === "sync:vc-margin") return "补充 VC 链接页的利润率事实表";
  if (syncType === "sync:vc-ads") return "补充 VC 链接页的广告事实表";
  if (syncType === "sync:po-orders") return "同步 VC PO 上游只读订单事实表";
  if (syncType === "sync:df-orders") return "同步 VC DF 上游只读订单事实表";
  if (syncType === "sync:vc-invoices") return "同步 VC 发货单上游只读事实表";
  throw new Error(`未登记同步类型描述: ${syncType}`);
}

function describeScheduleStrategy(
  taskType: string,
  cronExpr: string | null,
  dateWindowPreset?: ScheduleDateWindowPreset,
) {
  const human = cronExpr ? cronToHuman(cronExpr) : "未设置";
  const windowLabel = scheduleDateWindowLabel(dateWindowPreset);
  if (taskType === "sync:fba-listings")
    return `FBA Listing 缓存 · 当前 Listing · ${human}`;
  if (taskType === "sync:sc-sales")
    return `SC 销量窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:sc-returns")
    return `SC 退货窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:sc-ads")
    return `SC 广告窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:sc-performance")
    return `SC 表现快照 · 当前表现 · ${human}`;
  if (taskType === "sync:sc-ads-hsa-creative-mapping")
    return `每日 HSA 映射 · ${human}`;
  if (taskType === "sync:sc-ads-account")
    return `账户广告窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:sc-inventory")
    return `SC 库存快照 · 当前库存 · ${human}`;
  if (taskType === "sync:vc-sales")
    return `VC 销量窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:vc-traffic")
    return `VC 访问量窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:vc-inventory")
    return `VC 库存窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:vc-realtime")
    return `VC 实时销量窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:vc-margin")
    return `VC 利润率窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:vc-ads")
    return `VC 广告窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:po-orders")
    return `VC PO 只读订单窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:df-orders")
    return `VC DF 只读订单窗口 · ${windowLabel} · ${human}`;
  if (taskType === "sync:vc-invoices") return `VC 发货单窗口 · ${human}`;
  if (taskType === "sync:products") return `产品主数据 · ${human}`;
  if (taskType === "sync:pairing") return `产品配对 · ${human}`;
  if (taskType === "sync:stores") return `店铺目录 · ${human}`;
  throw new Error(`未登记同步计划策略: ${taskType}`);
}

function cronToHuman(expr: string): string {
  const parts = expr.trim().split(/\s+/);
  if (parts.length !== 5) return expr;
  const min = parts[0]!;
  const hour = parts[1]!;
  const dom = parts[2]!;
  const mon = parts[3]!;
  const dow = parts[4]!;
  if (
    min.startsWith("*/") &&
    hour === "*" &&
    dom === "*" &&
    mon === "*" &&
    dow === "*"
  ) {
    return `每 ${min.slice(2)} 分钟`;
  }
  if (
    min.startsWith("*/") &&
    hour.startsWith("*/") &&
    dom === "*" &&
    mon === "*" &&
    dow === "*"
  ) {
    const minStep = Number(min.slice(2));
    const hourStep = Number(hour.slice(2));
    if (minStep > 0 && minStep < 60 && hourStep > 0 && hourStep <= 12) {
      const minutes = Array.from({ length: Math.floor(60 / minStep) }, (_, i) =>
        String(i * minStep).padStart(2, "0"),
      );
      return `每 ${hourStep} 小时，第 ${minutes.join("、")} 分`;
    }
  }
  if (
    min.startsWith("*/") &&
    hour !== "*" &&
    !hour.startsWith("*/") &&
    dom === "*" &&
    mon === "*" &&
    dow === "*"
  ) {
    const step = Number(min.slice(2));
    if (step > 0 && step < 60) {
      const hours = hour.split(",");
      const minutes = Array.from(
        { length: Math.floor(60 / step) },
        (_, i) => i * step,
      );
      const times = hours.flatMap((h) =>
        minutes.map((m) => `${h}:${String(m).padStart(2, "0")}`),
      );
      return `每天 ${times.join("、")}`;
    }
  }
  if (
    min === "0" &&
    hour.startsWith("*/") &&
    dom === "*" &&
    mon === "*" &&
    dow === "*"
  ) {
    return `每 ${hour.slice(2)} 小时`;
  }
  if (
    /^\d+$/.test(min) &&
    hour !== "*" &&
    hour.includes(",") &&
    dom === "*" &&
    mon === "*" &&
    dow === "*"
  ) {
    return `每天 ${hour
      .split(",")
      .map((h) => `${h}:${min.padStart(2, "0")}`)
      .join("、")}`;
  }
  if (
    /^\d+$/.test(min) &&
    /^\d+$/.test(hour) &&
    dom === "*" &&
    mon === "*" &&
    dow === "*"
  ) {
    return `每天 ${hour}:${min.padStart(2, "0")}`;
  }
  if (
    /^\d+$/.test(min) &&
    /^\d+$/.test(hour) &&
    dom === "*" &&
    mon === "*" &&
    dow !== "*"
  ) {
    return `每周${dow === "1-5" ? "一至五" : dow} ${hour}:${min.padStart(2, "0")}`;
  }
  return expr;
}

function buildManualSyncUrlParams(input: ManualSyncSubmitInput) {
  const next = new URLSearchParams();
  next.set("tab", "manual");
  next.set("type", input.selectedSyncTypes.join(","));
  next.set("dataSourceId", input.dataSourceId);
  if (input.selectedStores.length)
    next.set("stores", input.selectedStores.join(","));
  if (input.dateRange.start) next.set("start", input.dateRange.start);
  if (input.dateRange.end) next.set("end", input.dateRange.end);
  if (input.fromParam) next.set("from", input.fromParam);
  if (input.profileParam) next.set("profile", input.profileParam);
  return next;
}

function buildManualSyncFormData(input: ManualSyncSubmitInput) {
  const request = new FormData();
  request.set("syncType", input.selectedSyncTypes.join(","));
  request.set("dataSourceId", input.dataSourceId);
  request.set("stores", input.selectedStores.join(","));
  request.set("channelType", input.channelType);
  request.set("start", input.dateRange.start);
  request.set("end", input.dateRange.end);
  if (input.fromParam) request.set("from", input.fromParam);
  if (input.profileParam) request.set("profile", input.profileParam);
  return request;
}

export function describeManualSyncSubmitForTest(input: ManualSyncSubmitInput) {
  const form = buildManualSyncFormData(input);
  return {
    formEntries: Object.fromEntries(form.entries()),
    url: buildManualSyncUrlParams(input).toString(),
  };
}

function normalizeSyncType(value: string | null) {
  return (
    SYNC_PRESETS.find((item) => item.value === value)?.value ?? "sync:sc-sales"
  );
}

function normalizeSyncTypes(value: string | null) {
  const normalized = Array.from(
    new Set(
      String(value ?? "")
        .split(",")
        .map((item) => normalizeSyncType(item.trim())),
    ),
  );
  const selectedGroup = SYNC_TYPE_GROUPS.find((group) =>
    normalized.some((syncType) => group.types.includes(syncType)),
  );
  if (!selectedGroup) return ["sync:sc-sales"];
  const sameGroup = normalized.filter((syncType) =>
    selectedGroup.types.includes(syncType),
  );
  return sameGroup.length ? sameGroup : ["sync:sc-sales"];
}

function toggleManualSyncTypeSelection(current: string[], syncType: string) {
  const selectedGroup = SYNC_TYPE_GROUPS.find((group) =>
    group.types.includes(syncType),
  );
  if (!selectedGroup) return [normalizeSyncType(syncType)];
  const currentInGroup = current.every((item) =>
    selectedGroup.types.includes(item),
  );
  if (!currentInGroup) return [syncType];
  if (!current.includes(syncType)) return [...current, syncType];
  const next = current.filter((item) => item !== syncType);
  return next.length ? next : [syncType];
}

function normalizeRangePreset(value: string | null, syncType: string) {
  if (["7d", "30d", "month", "custom"].includes(String(value)))
    return String(value);
  return defaultScheduleRange(syncType);
}

function toDateRangePreset(value: string): DateRangePresetKey {
  if (value === "month") return "month_current";
  if (value === "30d") return "30d";
  if (value === "custom") return "custom";
  return "7d";
}

function defaultScheduleRange(syncType: string) {
  return (
    SYNC_PRESETS.find((item) => item.value === syncType)?.defaultRange ?? "7d"
  );
}

function resolveDataSourceId(
  dataSources: DataSourceRow[],
  requested: string | null,
) {
  if (requested && dataSources.some((item) => item.id === requested))
    return requested;
  return dataSources[0]?.id ?? "";
}

function normalizeTab(value: string | null): SyncTab {
  return SYNC_TABS.some((tab) => tab.value === value)
    ? (value as SyncTab)
    : "overview";
}

function normalizeRunStatusFilter(value: string | null): string {
  return ["active", "cancelled", "error", "success"].includes(String(value))
    ? String(value)
    : "all";
}

function normalizeRunTypeFilter(value: string | null): string {
  if (!value) return "all";
  return SYNC_PRESETS.some((preset) => preset.value === value) ? value : "all";
}

function buildRunLogParams(input: {
  page: number;
  pageSize: number;
  runningOnly: boolean;
  search: string;
  status: string;
  type: string;
}) {
  const params = new URLSearchParams();
  params.set("tab", "runs");
  if (input.search.trim()) params.set("q", input.search.trim());
  if (input.runningOnly) params.set("runningOnly", "1");
  if (input.status !== "all") params.set("runStatus", input.status);
  if (input.type !== "all") params.set("runType", input.type);
  if (input.page > 1) params.set("page", String(input.page));
  if (input.pageSize !== 50) params.set("pageSize", String(input.pageSize));
  return params;
}

function buildRunLogQueryKey(input: {
  page: number;
  pageSize: number;
  runningOnly: boolean;
  search: string;
  status: string;
  type: string;
}) {
  return buildRunLogParams(input).toString();
}

function inferChannelType(syncType: string) {
  if (
    syncType.startsWith("sync:vc") ||
    syncType === "sync:po-orders" ||
    syncType === "sync:df-orders"
  )
    return "vc";
  if (syncType === "sync:fba-listings") return "sc";
  if (
    syncType.startsWith("sync:sc") ||
    syncType === "sync:sc-performance" ||
    syncType === "sync:sc-ads-hsa-creative-mapping"
  )
    return "sc";
  return "";
}

function channelMatchesSyncType(syncType: string, channelType: string) {
  if (
    syncType.startsWith("sync:vc") ||
    syncType === "sync:po-orders" ||
    syncType === "sync:df-orders"
  )
    return channelType === "vc";
  if (syncType === "sync:fba-listings") return channelType === "sc";
  if (syncType === "sync:sc-inventory") return channelType === "sc";
  if (
    syncType.startsWith("sync:sc") ||
    syncType === "sync:sc-performance" ||
    syncType === "sync:sc-ads-hsa-creative-mapping"
  )
    return channelType === "sc";
  return true;
}

function requiresScheduleStores(syncType: string) {
  return (
    syncType === "sync:fba-listings" ||
    syncType.startsWith("sync:vc") ||
    syncType === "sync:po-orders" ||
    syncType === "sync:df-orders" ||
    syncType.startsWith("sync:sc") ||
    syncType === "sync:sc-performance" ||
    syncType === "sync:sc-ads-hsa-creative-mapping"
  );
}

function channelMatchesScheduleTask(syncType: string, channelType: string) {
  if (
    syncType.startsWith("sync:vc") ||
    syncType === "sync:po-orders" ||
    syncType === "sync:df-orders"
  )
    return channelType === "vc";
  if (syncType === "sync:fba-listings") return channelType === "sc";
  if (syncType === "sync:sc-inventory") return channelType === "sc";
  if (
    syncType.startsWith("sync:sc") ||
    syncType === "sync:sc-performance" ||
    syncType === "sync:sc-ads-hsa-creative-mapping"
  )
    return channelType === "sc";
  return true;
}

function parseStoreIds(value: string) {
  return value
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);
}

function toggleStoreSelection(current: string[], storeId: string) {
  return current.includes(storeId)
    ? current.filter((item) => item !== storeId)
    : [...current, storeId];
}

function channelTypeLabel(value: string) {
  if (value === "sc") return "SC";
  if (value === "vc") return "VC";
  return value || "自动";
}

function resolveDateRange(
  range: string,
  startOverride: string | null,
  endOverride: string | null,
) {
  if (range === "custom" && startOverride && endOverride)
    return { start: startOverride, end: endOverride };

  const today = new Date();
  const end = formatDateOnly(today);
  if (range === "month") {
    const firstDay = new Date(today.getFullYear(), today.getMonth(), 1);
    return { start: formatDateOnly(firstDay), end };
  }
  const days = range === "30d" ? 29 : 6;
  const start = new Date(today);
  start.setDate(today.getDate() - days);
  return {
    start: startOverride ?? formatDateOnly(start),
    end: endOverride ?? end,
  };
}

function formatDateOnly(value: Date) {
  return `${value.getFullYear()}-${String(value.getMonth() + 1).padStart(2, "0")}-${String(value.getDate()).padStart(2, "0")}`;
}

function formatDateTime(value: string | null | undefined) {
  if (!value) return "—";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return value;
  return new Intl.DateTimeFormat("zh-CN", {
    timeZone: "Asia/Shanghai",
    month: "2-digit",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(date);
}

function formatDurationSeconds(value: number) {
  const seconds = Math.max(0, Math.floor(value));
  const minutes = Math.floor(seconds / 60);
  const hours = Math.floor(minutes / 60);
  if (hours) return `${hours}时${minutes % 60}分`;
  if (minutes) {
    const remainder = seconds % 60;
    return remainder ? `${minutes}分${remainder}秒` : `${minutes}分`;
  }
  return `${seconds}秒`;
}

function formatDurationMilliseconds(value: unknown) {
  if (value === null || value === undefined || value === "") return null;
  const milliseconds = Number(value);
  if (!Number.isFinite(milliseconds) || milliseconds < 0) return null;
  if (milliseconds > 0 && milliseconds < 1000) return "<1秒";
  return formatDurationSeconds(Math.floor(milliseconds / 1000));
}

function formatGroupRunDuration(
  group: Pick<ParentRunGroup, "children" | "run">,
  nowMs: number,
  status = groupDisplayStatus(group),
) {
  if (!group.children.length)
    return formatRunDuration(group.run, nowMs, status);
  if (group.children.some((child) => child.duration_error))
    return "耗时证据异常";
  const starts = group.children
    .map((child) => dateMs(child.started_at || child.created_at))
    .filter(isFiniteNumber);
  const hasActiveWindow = isActiveStatus(status);
  const ends = hasActiveWindow
    ? [nowMs]
    : group.children
        .map((child) => dateMs(child.ended_at || child.updated_at))
        .filter(isFiniteNumber);
  if (starts.length && ends.length) {
    const elapsedMs = Math.max(...ends) - Math.min(...starts);
    if (elapsedMs < 0) return "耗时证据异常";
    const elapsedText =
      hasActiveWindow && elapsedMs === 0
        ? "<1秒"
        : formatDurationMilliseconds(elapsedMs);
    const durationText = formatStatusDuration(status, elapsedText);
    return status === "success" ? `${durationText} · 已完成` : durationText;
  }
  const durations = group.children
    .filter(
      (child) => child.duration_ms !== null && child.duration_ms !== undefined,
    )
    .map((child) => Number(child.duration_ms))
    .filter((value) => Number.isFinite(value) && value >= 0);
  const evidenceDuration = durations.length
    ? formatDurationMilliseconds(Math.max(...durations))
    : null;
  if (evidenceDuration) {
    const durationText = formatStatusDuration(status, evidenceDuration);
    return status === "success" ? `${durationText} · 已完成` : durationText;
  }
  return formatRunDuration(group.run, nowMs, status);
}

function formatRunDuration(
  run: Pick<
    SyncRunView,
    | "created_at"
    | "started_at"
    | "ended_at"
    | "updated_at"
    | "status"
    | "duration_ms"
    | "duration_error"
  >,
  nowMs: number,
  status = run.status,
) {
  if (run.duration_error) return "耗时证据异常";
  const evidenceDuration = isActiveStatus(status)
    ? null
    : formatDurationMilliseconds(run.duration_ms);
  if (evidenceDuration) {
    const durationText = formatStatusDuration(status, evidenceDuration);
    return status === "success" ? `${durationText} · 已完成` : durationText;
  }
  const startValue = run.started_at || run.created_at;
  if (!startValue) return formatStatusDuration(status, "—");
  const start = new Date(startValue);
  if (Number.isNaN(start.getTime())) return formatStatusDuration(status, "—");
  const endMs = isActiveStatus(status)
    ? nowMs
    : dateMs(run.ended_at || run.updated_at);
  if (!Number.isFinite(endMs)) return formatStatusDuration(status, "—");
  const elapsedMs = endMs - start.getTime();
  if (elapsedMs < 0) return "耗时证据异常";
  const elapsedText =
    isActiveStatus(status) && elapsedMs === 0
      ? "<1秒"
      : formatDurationMilliseconds(elapsedMs);
  const durationText = formatStatusDuration(status, elapsedText);
  return status === "success" ? `${durationText} · 已完成` : durationText;
}

function formatStatusDuration(status: string, duration: string | null) {
  const labels: Record<string, string> = {
    queued: "等待执行",
    pending: "等待规划",
    resource_ready: "等待调度",
    waiting_resource: "等待资源",
    admitted: "等待执行",
    retry_wait: "等待重试",
  };
  return `${labels[status] ?? "已执行"} ${duration ?? "—"}`;
}

function dateMs(value: string | null | undefined) {
  if (!value) return Number.NaN;
  return new Date(value).getTime();
}

function isFiniteNumber(value: number) {
  return Number.isFinite(value);
}

function supportsBusinessSyncSchedules(source: DataSourceRow | undefined) {
  return Boolean(
    source && BUSINESS_SYNC_SCHEDULE_PROVIDERS.has(source.provider),
  );
}

function isDefaultSelfPairingSource(source: DataSourceRow | undefined) {
  return Boolean(
    source &&
      source.provider === "lingxing" &&
      source.account_slug === "self" &&
      Number(source.is_default) === 1,
  );
}

function firstNonEmpty(...values: unknown[]) {
  return values.find(
    (value) =>
      value !== undefined && value !== null && String(value).trim() !== "",
  );
}

function formatErrorMessage(error: unknown, fallback: string) {
  return error instanceof Error ? error.message : String(error || fallback);
}

function formatSyncRunReadFailure(failure: SyncRunReadFailureDetails) {
  return [
    `关联标识：${failure.correlationId}`,
    ...(failure.digest ? [`Next digest：${failure.digest}`] : []),
  ].join(" · ");
}

function settledFailureLine(
  result: PromiseSettledResult<unknown>,
  label: string,
) {
  if (result.status === "fulfilled") return null;
  const message = formatErrorMessage(result.reason, label);
  return message === label ? label : `${label}: ${message}`;
}

function StatusBadge({
  value,
  tone,
  loading = false,
}: {
  value: string;
  tone?: "success" | "warning" | "danger" | "info" | "neutral";
  loading?: boolean;
}) {
  const resolvedTone = tone ?? resolveTone(value);
  return (
    <span className={`status-badge badge-${resolvedTone}`}>
      {loading ? (
        <RefreshCw
          className="sync-run-status-icon h-3 w-3 animate-spin"
          data-testid="sync-run-loading"
        />
      ) : null}
      {formatStatusBadgeValue(value)}
    </span>
  );
}

function formatStatusBadgeValue(value: string) {
  const labels: Record<string, string> = {
    success: "已完成",
    error: "同步失败",
    failed: "同步失败",
    cancelled: "已取消",
    stale: "已过期",
    queued: "等待执行",
    pending: "等待规划",
    resource_ready: "等待调度",
    waiting_resource: "等待资源",
    admitted: "等待执行",
    running: "执行中",
    retry_wait: "等待重试",
    cooldown: "冷却中",
    paused: "已暂停",
  };
  return labels[value.toLowerCase()] ?? value;
}

function resolveTone(
  value: string,
): "success" | "warning" | "danger" | "info" | "neutral" {
  const lower = value.toLowerCase();
  if (["success", "enabled"].some((token) => lower.includes(token)))
    return "success";
  if (["error", "stale", "fail"].some((token) => lower.includes(token)))
    return "danger";
  if (["running", "admitted", "sync"].some((token) => lower.includes(token)))
    return "info";
  if (
    [
      "queued",
      "pending",
      "resource_ready",
      "waiting_resource",
      "retry_wait",
      "cooldown",
      "paused",
      "warning",
    ].some((token) => lower.includes(token))
  )
    return "warning";
  return "neutral";
}
