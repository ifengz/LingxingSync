"use client";

import type { ReactNode } from "react";

import { SelectNative } from "@/components/ui/select";

export function SortSelect<T extends string>({
  value,
  onChange,
  children,
  name,
}: {
  value: T;
  onChange: (value: T) => void;
  children: ReactNode;
  name?: string;
}) {
  return (
    <SelectNative
      className="w-[180px]"
      name={name}
      value={value}
      onChange={(event) => onChange(event.target.value as T)}
    >
      {children}
    </SelectNative>
  );
}
