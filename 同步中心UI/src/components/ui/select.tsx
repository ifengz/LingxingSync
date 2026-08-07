"use client";

import * as React from "react";

import { cn } from "@/lib/utils";

export const SelectNative = React.forwardRef<
  HTMLSelectElement,
  React.SelectHTMLAttributes<HTMLSelectElement>
>(({ className, ...props }, ref) => (
  <select
    className={cn(
      "min-h-9 rounded-md border border-line bg-white px-3 py-2 text-sm text-ink outline-none focus:border-primary focus:ring-4 focus:ring-primary/10",
      className,
    )}
    ref={ref}
    {...props}
  />
));
SelectNative.displayName = "SelectNative";
