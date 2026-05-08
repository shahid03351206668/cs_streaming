"use client";

import { useCallback, useEffect, useRef, useState } from "react";
import { PlusIcon, SearchIcon, XIcon } from "lucide-react";

import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Badge } from "@/components/ui/badge";
import type { FilterDef } from "./types";

interface ListViewToolbarProps {
  search: string;
  onSearchChange: (value: string) => void;
  debounceMs?: number;

  filters: FilterDef[];
  activeFilters: Record<string, string>;
  onFilterChange: (key: string, value: string) => void;
  onClearFilters: () => void;

  onCreateClick?: () => void;
  createLabel?: string;

  searchable?: boolean;
  searchPlaceholder?: string;

  /** Total active filter count badge */
  activeFilterCount: number;
}

export function ListViewToolbar({
  search,
  onSearchChange,
  debounceMs = 400,
  filters,
  activeFilters,
  onFilterChange,
  onClearFilters,
  onCreateClick,
  createLabel = "Create",
  searchable = true,
  searchPlaceholder = "Search…",
  activeFilterCount,
}: ListViewToolbarProps) {
  // Local state so the input feels instant while the actual query debounces.
  const [localSearch, setLocalSearch] = useState(search);
  const debounceTimer = useRef<ReturnType<typeof setTimeout> | null>(null);

  // Keep local value in sync when parent resets (e.g. on filter clear).
  useEffect(() => {
    setLocalSearch(search);
  }, [search]);

  const handleSearchChange = useCallback(
    (e: React.ChangeEvent<HTMLInputElement>) => {
      const value = e.target.value;
      setLocalSearch(value);

      if (debounceTimer.current) clearTimeout(debounceTimer.current);
      debounceTimer.current = setTimeout(() => {
        onSearchChange(value);
      }, debounceMs);
    },
    [onSearchChange, debounceMs]
  );

  const clearSearch = useCallback(() => {
    setLocalSearch("");
    onSearchChange("");
  }, [onSearchChange]);

  return (
    <div className="flex flex-col gap-3 sm:flex-row sm:items-center sm:justify-between py-4">
      {/* Left side: search + filters */}
      <div className="flex flex-1 flex-wrap items-center gap-2">
        {searchable && (
          <div className="relative w-full sm:w-72">
            <SearchIcon className="absolute left-2.5 top-2.5 h-4 w-4 text-muted-foreground" />
            <Input
              placeholder={searchPlaceholder}
              value={localSearch}
              onChange={handleSearchChange}
              className="pl-8 pr-8"
            />
            {localSearch && (
              <button
                onClick={clearSearch}
                className="absolute right-2.5 top-2.5 text-muted-foreground hover:text-foreground"
                aria-label="Clear search"
              >
                <XIcon className="h-4 w-4" />
              </button>
            )}
          </div>
        )}

        {/* Filter selects */}
        {filters.map((filter) => (
          <Select
            key={filter.key}
            value={activeFilters[filter.key] ?? ""}
            onValueChange={(value) => onFilterChange(filter.key, value)}
          >
            <SelectTrigger className="w-40">
              <SelectValue placeholder={filter.placeholder ?? filter.label} />
            </SelectTrigger>
            <SelectContent>
              {/* Empty string = "All" / clear the filter */}
              <SelectItem value="">All {filter.label}</SelectItem>
              {filter.options.map((opt) => (
                <SelectItem key={opt.value} value={opt.value}>
                  {opt.label}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
        ))}

        {/* Clear all filters badge */}
        {activeFilterCount > 0 && (
          <Button
            variant="ghost"
            size="sm"
            onClick={onClearFilters}
            className="h-8 gap-1 text-muted-foreground"
          >
            <XIcon className="h-3.5 w-3.5" />
            Clear
            <Badge variant="secondary" className="ml-1 rounded-full px-1.5">
              {activeFilterCount}
            </Badge>
          </Button>
        )}
      </div>

      {/* Right side: create button */}
      {onCreateClick && (
        <Button onClick={onCreateClick} size="sm" className="shrink-0">
          <PlusIcon className="mr-2 h-4 w-4" />
          {createLabel}
        </Button>
      )}
    </div>
  );
}
