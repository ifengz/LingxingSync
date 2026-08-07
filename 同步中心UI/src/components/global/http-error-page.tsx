"use client";

import * as React from "react";
import { Home } from "lucide-react";

import { HTTP_ERROR_PAGE_BYPASS_APP_SHELL } from "./http-error-shell";

type ErrorStatus = {
  code: string;
  title: string;
  summary: string;
  detail: string;
};

const WHEEL_ITEM_HEIGHT = 48;
const WHEEL_LOOP_REPEATS = 3;
const WHEEL_CENTER_REPEAT = 1;

export const HTTP_ERROR_STATUSES: ErrorStatus[] = [
  {
    code: "400",
    title: "请求格式不对",
    summary: "检查当前链接、筛选条件或填写内容，改正后再提交。",
    detail: "提交给服务器的信息不完整或格式不对，页面没法按这次请求继续处理。",
  },
  {
    code: "401",
    title: "登录状态失效",
    summary: "重新登录账号后再打开这个页面，必要时刷新浏览器。",
    detail: "当前登录凭证已经过期，服务器不能确认你是谁，所以先拦下了请求。",
  },
  {
    code: "403",
    title: "没有访问权限",
    summary: "确认账号是否有这个页面或操作权限，再让管理员补充权限。",
    detail: "账号已经登录，但没有被允许访问当前页面或执行这个操作。",
  },
  {
    code: "404",
    title: "页面不存在",
    summary: "回到首页重新进入，或检查地址是不是复制少了字符。",
    detail: "当前地址没有对应页面，可能是旧入口、错误链接或地址不完整。",
  },
  {
    code: "409",
    title: "数据状态冲突",
    summary: "刷新页面拿到最新数据后，再重新执行刚才的操作。",
    detail: "你看到的数据和服务器里的最新状态不一致，这次操作暂时不能继续。",
  },
  {
    code: "410",
    title: "资源已移除",
    summary: "从当前系统入口重新生成或重新进入，不要继续使用旧链接。",
    detail: "这个资源以前存在，但现在已经被移除或不再开放访问。",
  },
  {
    code: "413",
    title: "请求内容过大",
    summary: "缩小上传文件、减少导入行数，或分成多次提交。",
    detail: "本次提交的内容超过系统允许大小，服务器为了稳定性拒绝继续处理。",
  },
  {
    code: "422",
    title: "内容校验未通过",
    summary: "按页面提示修正字段内容，确认必填项和范围后再提交。",
    detail: "请求格式没问题，但里面的业务内容不符合规则，所以没有保存。",
  },
  {
    code: "423",
    title: "资源处理中",
    summary: "等当前同步、导入或后台任务结束后，再重新操作。",
    detail: "相关资源正在被后台任务占用，系统暂时不能同时处理这次请求。",
  },
  {
    code: "425",
    title: "请求过早",
    summary: "稍等几秒后再重试，不要连续快速点击同一个操作。",
    detail: "服务器还没有准备好处理当前请求，太早提交可能会造成重复或异常。",
  },
  {
    code: "429",
    title: "请求过于频繁",
    summary: "暂停刷新或重复点击，等限流恢复后再继续操作。",
    detail: "短时间内请求太多，系统为了保护服务稳定，临时限制了访问。",
  },
  {
    code: "500",
    title: "服务内部错误",
    summary: "记录页面、时间和操作步骤，复现时交给技术人员查看日志。",
    detail: "服务器处理时遇到了没有预料到的问题，这通常不是你当前操作造成的。",
  },
  {
    code: "502",
    title: "网关收到异常响应",
    summary: "先稍后重试；持续出现时检查后端服务和反向代理配置。",
    detail: "网关联系到了后端服务，但后端返回的内容不正常，页面无法继续展示。",
  },
  {
    code: "503",
    title: "服务暂时不可用",
    summary: "稍后重试；如果一直出现，优先确认服务是否在重启或过载。",
    detail: "服务可能正在重启、维护或负载过高，所以暂时不能响应这个页面。",
  },
  {
    code: "504",
    title: "网关等待超时",
    summary: "保留当前操作和时间，重点排查慢查询、慢接口或后端卡住的位置。",
    detail: "网关等后端响应等太久了，通常说明某个处理步骤耗时过长。",
  },
  {
    code: "ERR",
    title: "未知错误",
    summary: "先重试一次；如果还能复现，保留页面地址和操作路径。",
    detail: "这次异常没有带标准状态码，系统只能把它归到未知错误。",
  },
];

const FALLBACK_STATUS: ErrorStatus = HTTP_ERROR_STATUSES[
  HTTP_ERROR_STATUSES.length - 1
] ?? {
  code: "ERR",
  title: "未知错误",
  summary: "页面遇到未归类异常。",
  detail: "错误没有携带标准状态码。",
};
const LOOPED_ERROR_STATUSES = Array.from(
  { length: WHEEL_LOOP_REPEATS },
  (_, repeatIndex) =>
    HTTP_ERROR_STATUSES.map((status) => ({ status, repeatIndex })),
).flat();

function selectStatusCode(error?: Error | null, fallbackCode?: string) {
  if (fallbackCode) return fallbackCode;
  const message = error?.message ?? "";
  const match = message.match(
    /\b(400|401|403|404|409|410|413|422|423|425|429|500|502|503|504)\b/,
  );
  return match?.[1] ?? "500";
}

export function HttpErrorPage({
  error,
  reset,
  code,
}: {
  error?: Error | null;
  reset?: () => void;
  code?: string;
}) {
  const requestedInitialCode = selectStatusCode(error, code);
  const initialStatus =
    HTTP_ERROR_STATUSES.find(
      (status) => status.code === requestedInitialCode,
    ) ?? FALLBACK_STATUS;
  const initialCode = initialStatus.code;
  const [activeCode, setActiveCode] = React.useState(initialCode);
  const [hintCode, setHintCode] = React.useState<string | null>(null);
  const wheelRef = React.useRef<HTMLDivElement | null>(null);
  const scrollFrameRef = React.useRef<number | null>(null);
  const activeStatus =
    HTTP_ERROR_STATUSES.find((status) => status.code === activeCode) ??
    FALLBACK_STATUS;
  const hintStatus =
    HTTP_ERROR_STATUSES.find((status) => status.code === hintCode) ?? null;
  const hasMovedFromOriginal = activeStatus.code !== initialCode;

  React.useEffect(() => {
    setActiveCode(initialCode);
    centerWheelCode(initialCode);
  }, [initialCode]);

  React.useEffect(() => {
    return () => {
      if (scrollFrameRef.current)
        window.cancelAnimationFrame(scrollFrameRef.current);
    };
  }, []);

  function selectCode(nextCode: string) {
    setActiveCode(nextCode);
    centerWheelCode(nextCode);
  }

  function centerWheelCode(nextCode: string) {
    const wheel = wheelRef.current;
    const selected = wheel?.querySelector<HTMLElement>(
      `[data-error-code="${nextCode}"][data-wheel-repeat="${WHEEL_CENTER_REPEAT}"]`,
    );
    if (!wheel || !selected) return;
    wheel.scrollLeft = 0;
    wheel.scrollTop =
      selected.offsetTop - (wheel.clientHeight - selected.offsetHeight) / 2;
  }

  function handleWheelScroll() {
    if (scrollFrameRef.current)
      window.cancelAnimationFrame(scrollFrameRef.current);
    scrollFrameRef.current = window.requestAnimationFrame(() => {
      scrollFrameRef.current = null;
      const wheel = wheelRef.current;
      if (!wheel) return;
      wheel.scrollLeft = 0;
      const centeredLoopIndex = Math.round(
        (wheel.scrollTop + (wheel.clientHeight - WHEEL_ITEM_HEIGHT) / 2) /
          WHEEL_ITEM_HEIGHT,
      );
      const loopLength = HTTP_ERROR_STATUSES.length;
      const normalizedIndex =
        ((centeredLoopIndex % loopLength) + loopLength) % loopLength;
      const nextCode = HTTP_ERROR_STATUSES[normalizedIndex]?.code;
      if (nextCode) setActiveCode(nextCode);
      const minMiddleIndex = loopLength;
      const maxMiddleIndex = loopLength * 2 - 1;
      if (
        centeredLoopIndex < minMiddleIndex ||
        centeredLoopIndex > maxMiddleIndex
      ) {
        wheel.scrollTop +=
          (WHEEL_CENTER_REPEAT * loopLength -
            Math.floor(centeredLoopIndex / loopLength) * loopLength) *
          WHEEL_ITEM_HEIGHT;
      }
    });
  }

  return (
    <main
      className="http-error-page"
      data-current-code={activeStatus.code}
      aria-labelledby="http-error-title"
    >
      <section
        className="http-error-stage"
        aria-label={`${activeStatus.code} ${activeStatus.title}: ${activeStatus.summary}`}
        aria-live="polite"
        data-http-error-page
      >
        <div
          ref={wheelRef}
          className="http-error-wheel"
          onScroll={handleWheelScroll}
          role="listbox"
          aria-label="错误码"
        >
          {LOOPED_ERROR_STATUSES.map(({ status, repeatIndex }) => (
            <button
              aria-label={`${status.code} ${status.title}`}
              aria-selected={status.code === activeStatus.code}
              className={`http-error-code${status.code === activeStatus.code ? " is-active" : ""}${status.code === initialCode ? " is-current" : ""}`}
              data-error-code={status.code}
              data-wheel-repeat={repeatIndex}
              key={`${repeatIndex}-${status.code}`}
              onBlur={() => setHintCode(null)}
              onClick={() => selectCode(status.code)}
              onFocus={() => setHintCode(status.code)}
              onMouseEnter={() => setHintCode(status.code)}
              onMouseLeave={() => setHintCode(null)}
              role="option"
              tabIndex={repeatIndex === WHEEL_CENTER_REPEAT ? 0 : -1}
              type="button"
            >
              {status.code}
            </button>
          ))}
        </div>
        <div className="http-error-divider" />
        <div className="http-error-copy">
          <div className="http-error-title-row">
            <p id="http-error-title">{activeStatus.title}</p>
            <a className="http-error-home" href="/" aria-label="返回首页">
              <Home
                aria-hidden="true"
                focusable="false"
                size={16}
                strokeWidth={1.8}
              />
            </a>
          </div>
          {reset ? (
            <button className="http-error-reset" onClick={reset} type="button">
              重试
            </button>
          ) : null}
        </div>
        <button
          aria-label={`回到当前错误 ${initialCode}`}
          className={`http-error-current-anchor${hasMovedFromOriginal ? " is-visible" : ""}`}
          onClick={() => selectCode(initialCode)}
          type="button"
        >
          {initialCode}
        </button>
        <div
          className={`http-error-detail${hintStatus ? " is-visible" : ""}`}
          aria-hidden={!hintStatus}
        >
          <p>{hintStatus ? `${hintStatus.code}: ${hintStatus.detail}` : ""}</p>
          <p>{hintStatus ? `建议：${hintStatus.summary}` : ""}</p>
        </div>
      </section>
    </main>
  );
}

(
  HttpErrorPage as typeof HttpErrorPage & {
    [HTTP_ERROR_PAGE_BYPASS_APP_SHELL]?: boolean;
  }
)[HTTP_ERROR_PAGE_BYPASS_APP_SHELL] = true;
