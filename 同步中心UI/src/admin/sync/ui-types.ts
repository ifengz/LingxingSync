export type DataSourceRow = {
  id: string;
  provider: string;
  label: string;
  account_slug: string | null;
  enabled: number;
  deletable: number;
  is_default: number;
  updated_at: string;
};
export type LingxingEndpointLimitRow = {
  dataSourceId: string;
  dataSourceName: string;
  accountSlug: string | null;
  appIdLast4: string | null;
  endpointUrl: string;
  label: string;
  methods: string[];
  defaultCapacity: number;
  effectiveCapacity: number;
  overrideCapacity: number | null;
  editable: boolean;
  fixedReason: string | null;
  registered: boolean;
};
export type ChannelRow = {
  id: number;
  data_source_id: string;
  store_id: string;
  store_name: string | null;
  country: string | null;
  channel_type: string;
  enabled: number;
  profile_id?: string | null;
};
export type SyncOverviewSourceKey = "self" | "affiliate" | "spotterio";
export type SyncOverviewStoreRow = {
  source_key: SyncOverviewSourceKey;
  data_source_id: string | null;
  data_source_label: string;
  store_key: string;
  store_label: string;
  channel_type: string | null;
  asin_count: number;
  start_date: string | null;
  end_date: string | null;
  main_rows: number;
  detail_rows: number;
  ad_rows: number;
  latest_sync_at: string | null;
  synced_days: number;
  total_days: number;
  gap_days: number;
};
export type CoverageDimension =
  | "sales"
  | "ads"
  | "inventory"
  | "performance"
  | "traffic"
  | "margin";
export type StoreCoverageDetail = {
  data_source_id: string;
  store_id: string;
  channel_type: string;
  start_date: string;
  end_date: string;
  dates: Record<CoverageDimension, string[]>;
  failed: Array<{
    sync_type: string;
    start_date: string | null;
    end_date: string | null;
  }>;
  successDates?: Array<{
    sync_type: string;
    start_date: string | null;
    end_date: string | null;
  }>;
  unavailable: CoverageDimension[];
};
export type SyncRunRow = {
  run_id: string;
  parent_run_id: string | null;
  sync_type: string;
  target_scope_json: unknown;
  status: string;
  reason_code: string | null;
  cancelled_reason: string | null;
  chunk_label: string | null;
  retry_of_run_id: string | null;
  summary_json?: unknown;
  created_at: string;
  started_at: string | null;
  ended_at: string | null;
  duration_ms?: number | null;
  duration_error?: string | null;
  updated_at?: string | null;
  lease_until?: string | null;
  heartbeat_at?: string | null;
};
export type SegmentRow = {
  run_id: string;
  stage_name: string;
  segment_key: string;
  status: string;
  response_evidence_json: unknown;
};
export type SyncScheduleRow = {
  id: number;
  data_source_id: string;
  task_type: string;
  cron_expr: string | null;
  schedule_payload_json?: unknown;
  enabled: number;
  last_run_at: string | null;
  next_run_at: string | null;
};
export type SyncWorkerHealthSnapshot = {
  egressIp: string | null;
  egressIpCheckedAt: string | null;
  egressIpError: string | null;
  error: string | null;
  ok: boolean;
  queue: string | null;
  uptime: number | null;
};
