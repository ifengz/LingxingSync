"use client";

import { useEffect, useRef } from "react";

type SyncSnapshotPayload = {
  hasNext?: boolean;
  runs?: unknown[];
  segments?: unknown[];
  emittedAt?: string;
};

export function SyncEventsClient({
  active,
  onError,
  onSnapshot,
}: {
  active: boolean;
  onError?: (message: string) => void;
  onSnapshot?: (payload: SyncSnapshotPayload) => void;
}) {
  const lastSnapshotSignatureRef = useRef("");

  useEffect(() => {
    if (!active) return undefined;
    const source = new EventSource("/api/sync-events");
    const handleSnapshot = (event: MessageEvent<string>) => {
      try {
        const payload = JSON.parse(event.data) as SyncSnapshotPayload;
        const signature = snapshotSignature(payload);
        if (signature === lastSnapshotSignatureRef.current) return;
        lastSnapshotSignatureRef.current = signature;
        onSnapshot?.(payload);
      } catch {
        onError?.("同步实时快照解析失败，请手动刷新同步日志");
      }
    };
    source.addEventListener("sync_snapshot", handleSnapshot as EventListener);
    return () => {
      source.removeEventListener(
        "sync_snapshot",
        handleSnapshot as EventListener,
      );
      source.close();
    };
  }, [active, onError, onSnapshot]);

  return null;
}

function snapshotSignature(payload: SyncSnapshotPayload) {
  return JSON.stringify({
    hasNext: Boolean(payload.hasNext),
    runs: stableRows(payload.runs),
    segments: stableRows(payload.segments),
  });
}

function stableRows(rows: unknown[] | undefined) {
  return Array.isArray(rows)
    ? rows
        .map(stableRow)
        .sort((left, right) =>
          JSON.stringify(left).localeCompare(JSON.stringify(right)),
        )
    : [];
}

function stableRow(row: unknown) {
  if (!row || typeof row !== "object" || Array.isArray(row)) return row;
  const {
    updated_at: _updatedAt,
    heartbeat_at: _heartbeatAt,
    lease_until: _leaseUntil,
    emittedAt: _emittedAt,
    ...rest
  } = row as Record<string, unknown>;
  return rest;
}
