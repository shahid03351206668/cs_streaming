"use client";

import { useCallback, useMemo, useState } from "react";
import { useQuery, keepPreviousData } from "@tanstack/react-query";
import {
  ChevronLeftIcon,
  ChevronRightIcon,
  ChevronsLeftIcon,
  ChevronsRightIcon,
  AlertCircleIcon,
} from "lucide-react";

import { Button } from "@/components/ui/button";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert";

import { ListViewTable } from "./ListViewTable";
import { ListViewToolbar } from "./ListViewToolbar";
import type {
  ListViewParams,
  ListViewProps,
  SortDirection,
  SortState,
} from "./types";

const DEFAULT_PAGE_SIZE_OPTIONS = [10, 25, 50, 100];

export function ListView<TRow extends { id: string | number }>({
  title,
  description,
  columns,
  queryKey,
  queryFn,
  actions = [],
  filters = [],
  pageSizeOptions = DEFAULT_PAGE_SIZE_OPTIONS,
  defaultPageSize = 10,
  defaultSort,
  onCreateClick,
  createLabel = "Create",
  rowClassName,
  searchable = true,
  searchPlaceholder,
}: ListViewProps<TRow>) {
  // ── State ──────────────────────────────────────────────────────────────────
  const [page, setPage] = useState(1);
  const [limit, setLimit] = useState(defaultPageSize);
  const [search, setSearch] = useState("");
  const [sort, setSort] = useState<SortState>(
    defaultSort ?? { column: "", direction: "asc" }
  );
  const [activeFilters, setActiveFilters] = useState<Record<string, string>>({});

  // ── Derived params (stable reference for React Query key) ─────────────────
  const params = useMemo<ListViewParams>(
    () => ({
      page,
      limit,
      search,
      sortColumn: sort.column,
      sortDirection: sort.direction,
      filters: activeFilters,
    }),
    [page, limit, search, sort, activeFilters]
  );

  // ── Query ─────────────────────────────────────────────────────────────────
  const { data, isFetching, isError, error, refetch } = useQuery({
    queryKey: [...queryKey, params],
    queryFn: () => queryFn(params),
    placeholderData: keepPreviousData, // show old data while new page loads
    staleTime: 30_000,                 // 30s — reduce unnecessary refetches
  });

  const rows = data?.data ?? [];
  const total = data?.total ?? 0;
  const totalPages = data?.totalPages ?? 1;

  // ── Handlers ──────────────────────────────────────────────────────────────
  const handleSearchChange = useCallback((value: string) => {
    setSearch(value);
    setPage(1); // reset to page 1 on new search
  }, []);

  const handleSortChange = useCallback((column: string) => {
    setSort((prev) => ({
      column,
      direction:
        prev.column === column && prev.direction === "asc" ? "desc" : "asc",
    }));
    setPage(1);
  }, []);

  const handleFilterChange = useCallback((key: string, value: string) => {
    setActiveFilters((prev) => {
      const next = { ...prev };
      if (value === "") {
        delete next[key];
      } else {
        next[key] = value;
      }
      return next;
    });
    setPage(1);
  }, []);

  const handleClearFilters = useCallback(() => {
    setActiveFilters({});
    setSearch("");
    setPage(1);
  }, []);

  const handleLimitChange = useCallback((value: string) => {
    setLimit(Number(value));
    setPage(1);
  }, []);

  const activeFilterCount =
    Object.keys(activeFilters).length + (search ? 1 : 0);

  // ── Render ────────────────────────────────────────────────────────────────
  return (
    <div className="space-y-1">
      {/* Header */}
      {(title || description) && (
        <div className="mb-4">
          {title && (
            <h2 className="text-2xl font-bold tracking-tight">{title}</h2>
          )}
          {description && (
            <p className="text-muted-foreground mt-1">{description}</p>
          )}
        </div>
      )}

      {/* Error banner */}
      {isError && (
        <Alert variant="destructive" className="mb-4">
          <AlertCircleIcon className="h-4 w-4" />
          <AlertTitle>Failed to load data</AlertTitle>
          <AlertDescription className="flex items-center gap-2">
            {error instanceof Error ? error.message : "An unknown error occurred."}
            <Button
              variant="outline"
              size="sm"
              className="ml-auto h-7"
              onClick={() => refetch()}
            >
              Retry
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {/* Toolbar */}
      <ListViewToolbar
        search={search}
        onSearchChange={handleSearchChange}
        filters={filters}
        activeFilters={activeFilters}
        onFilterChange={handleFilterChange}
        onClearFilters={handleClearFilters}
        onCreateClick={onCreateClick}
        createLabel={createLabel}
        searchable={searchable}
        searchPlaceholder={searchPlaceholder}
        activeFilterCount={activeFilterCount}
      />

      {/* Table */}
      <ListViewTable<TRow>
        columns={columns}
        rows={rows}
        actions={actions}
        sort={sort}
        onSortChange={handleSortChange}
        isLoading={isFetching && rows.length === 0}
        rowClassName={rowClassName}
      />

      {/* Footer: pagination */}
      <div className="flex flex-col-reverse gap-3 sm:flex-row sm:items-center sm:justify-between py-4">
        {/* Total count */}
        <p className="text-sm text-muted-foreground">
          {total === 0
            ? "No results"
            : `Showing ${(page - 1) * limit + 1}–${Math.min(page * limit, total)} of ${total} result${total !== 1 ? "s" : ""}`}
        </p>

        <div className="flex items-center gap-4">
          {/* Rows per page */}
          <div className="flex items-center gap-2">
            <span className="text-sm text-muted-foreground whitespace-nowrap">
              Rows per page
            </span>
            <Select value={String(limit)} onValueChange={handleLimitChange}>
              <SelectTrigger className="h-8 w-16">
                <SelectValue />
              </SelectTrigger>
              <SelectContent side="top">
                {pageSizeOptions.map((size) => (
                  <SelectItem key={size} value={String(size)}>
                    {size}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>
          </div>

          {/* Page indicator */}
          <span className="text-sm text-muted-foreground whitespace-nowrap">
            Page {page} of {totalPages}
          </span>

          {/* Page navigation */}
          <div className="flex items-center gap-1">
            <Button
              variant="outline"
              size="icon"
              className="h-8 w-8"
              onClick={() => setPage(1)}
              disabled={page === 1 || isFetching}
              aria-label="First page"
            >
              <ChevronsLeftIcon className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              className="h-8 w-8"
              onClick={() => setPage((p) => Math.max(1, p - 1))}
              disabled={page === 1 || isFetching}
              aria-label="Previous page"
            >
              <ChevronLeftIcon className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              className="h-8 w-8"
              onClick={() => setPage((p) => Math.min(totalPages, p + 1))}
              disabled={page === totalPages || isFetching}
              aria-label="Next page"
            >
              <ChevronRightIcon className="h-4 w-4" />
            </Button>
            <Button
              variant="outline"
              size="icon"
              className="h-8 w-8"
              onClick={() => setPage(totalPages)}
              disabled={page === totalPages || isFetching}
              aria-label="Last page"
            >
              <ChevronsRightIcon className="h-4 w-4" />
            </Button>
          </div>
        </div>
      </div>
    </div>
  );
}
