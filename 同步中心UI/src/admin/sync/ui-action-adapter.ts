/**
 * UI-only boundary for the extracted Sync Center. Integrate these functions
 * with a host application's own API layer only after an explicit decision.
 */
const UI_ONLY_MESSAGE =
  "此副本只保留同步中心前端；实际同步、任务控制和配置保存已被移除。";

type UiActionResult = { ok: false; error: string; dbChanged?: false };

function unavailable(): never {
  throw new Error(UI_ONLY_MESSAGE);
}

export async function showSyncActionUnavailable(
  _formData: FormData,
): Promise<void> {
  unavailable();
}

export async function reportCancelUnavailable(
  _formData: FormData,
): Promise<void> {
  unavailable();
}

export async function reportRetryUnavailable(
  _formData: FormData,
): Promise<void> {
  unavailable();
}

export async function listSyncOverviewRowsAction(): Promise<never> {
  unavailable();
}

export async function listSyncRunPageAction(
  _input: unknown,
): Promise<{ ok: false; error: string }> {
  return { ok: false, error: UI_ONLY_MESSAGE };
}

export async function listStoreCoverageDetailAction(
  _input: unknown,
): Promise<never> {
  unavailable();
}

export async function reportScheduleSaveUnavailable(
  _input: unknown,
): Promise<UiActionResult> {
  return { ok: false, error: UI_ONLY_MESSAGE, dbChanged: false };
}

export async function reportRuntimeConfigSaveUnavailable(
  _input: unknown,
): Promise<UiActionResult> {
  return { ok: false, error: UI_ONLY_MESSAGE };
}

export async function reportEndpointLimitSaveUnavailable(
  _input: unknown,
): Promise<UiActionResult> {
  return { ok: false, error: UI_ONLY_MESSAGE };
}
