"use client";

import { HttpErrorPage } from "@/components/global/http-error-page";

export default function ErrorBoundary({
  error,
  reset,
}: {
  error: Error;
  reset: () => void;
}) {
  return <HttpErrorPage error={error} reset={reset} />;
}
