"use client";

import { Search, X } from "lucide-react";

import { Input } from "@/components/ui/input";

export function SearchInput({
  value,
  onChange,
  placeholder,
  name,
}: {
  value: string;
  onChange: (value: string) => void;
  placeholder: string;
  name?: string;
}) {
  return (
    <label className="relative inline-flex min-w-[280px] items-center">
      <Search className="pointer-events-none absolute left-3 h-4 w-4 text-ink-muted" />
      <Input
        className="pl-9 pr-8"
        name={name}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={placeholder}
      />
      {value ? (
        <button
          className="absolute right-2 text-ink-muted hover:text-ink"
          onClick={() => onChange("")}
          type="button"
          aria-label="清空搜索"
        >
          <X className="h-4 w-4" />
        </button>
      ) : null}
    </label>
  );
}
