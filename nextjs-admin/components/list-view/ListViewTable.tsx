"use client";

import { ArrowDownIcon, ArrowUpIcon, ChevronsUpDownIcon, MoreHorizontalIcon } from "lucide-react";
import { cn } from "@/lib/utils";

import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuSeparator,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";

import type { ColumnDef, RowAction, SortState } from "./types";

// ─── Helpers ─────────────────────────────────────────────────────────────────

/** Safely read a nested path like "user.name" from an object */
function getNestedValue(obj: unknown, path: string): unknown {
  return path.split(".").reduce<unknown>((acc, key) => {
    if (acc !== null && typeof acc === "object") {
      return (acc as Record<string, unknown>)[key];
    }
    return undefined;
  }, obj);
}

// ─── Sort Header Button ───────────────────────────────────────────────────────

function SortIcon({
  columnKey,
  sort,
}: {
  columnKey: string;
  sort: SortState;
}) {
  if (sort.column !== columnKey) {
    return <ChevronsUpDownIcon className="ml-1.5 h-3.5 w-3.5 text-muted-foreground/60" />;
  }
  return sort.direction === "asc" ? (
    <ArrowUpIcon className="ml-1.5 h-3.5 w-3.5" />
  ) : (
    <ArrowDownIcon className="ml-1.5 h-3.5 w-3.5" />
  );
}

// ─── Loading Skeleton ─────────────────────────────────────────────────────────

function TableSkeleton({
  columns,
  rows = 8,
  hasActions,
}: {
  columns: number;
  rows?: number;
  hasActions: boolean;
}) {
  return (
    <>
      {Array.from({ length: rows }).map((_, rowIdx) => (
        <TableRow key={rowIdx} className="hover:bg-transparent">
          {Array.from({ length: columns }).map((_, colIdx) => (
            <TableCell key={colIdx}>
              <Skeleton className="h-4 w-full max-w-[180px]" />
            </TableCell>
          ))}
          {hasActions && (
            <TableCell>
              <Skeleton className="h-8 w-8 rounded-md" />
            </TableCell>
          )}
        </TableRow>
      ))}
    </>
  );
}

// ─── Empty State ──────────────────────────────────────────────────────────────

function EmptyState({ colSpan }: { colSpan: number }) {
  return (
    <TableRow>
      <TableCell colSpan={colSpan} className="h-48 text-center">
        <div className="flex flex-col items-center gap-2 text-muted-foreground">
          <svg
            xmlns="http://www.w3.org/2000/svg"
            className="h-10 w-10 opacity-40"
            fill="none"
            viewBox="0 0 24 24"
            stroke="currentColor"
          >
            <path
              strokeLinecap="round"
              strokeLinejoin="round"
              strokeWidth={1.5}
              d="M9 17v-2a4 4 0 014-4h0a4 4 0 014 4v2M9 7a3 3 0 110-6 3 3 0 010 6zm6 0a3 3 0 110-6 3 3 0 010 6z"
            />
          </svg>
          <p className="text-sm font-medium">No results found</p>
          <p className="text-xs">Try adjusting your search or filters.</p>
        </div>
      </TableCell>
    </TableRow>
  );
}

// ─── Row Actions Dropdown ─────────────────────────────────────────────────────

function RowActionsMenu<TRow>({
  row,
  actions,
}: {
  row: TRow;
  actions: RowAction<TRow>[];
}) {
  const visible = actions.filter((a) => !a.hidden?.(row));
  if (visible.length === 0) return null;

  return (
    <DropdownMenu>
      <DropdownMenuTrigger asChild>
        <Button
          variant="ghost"
          size="icon"
          className="h-8 w-8 data-[state=open]:bg-muted"
          aria-label="Row actions"
        >
          <MoreHorizontalIcon className="h-4 w-4" />
        </Button>
      </DropdownMenuTrigger>
      <DropdownMenuContent align="end" className="w-40">
        {visible.map((action, idx) => {
          const isDestructive = action.variant === "destructive";
          const isDisabled = action.disabled?.(row) ?? false;

          // Add a separator before the first destructive item
          const prevAction = visible[idx - 1];
          const addSeparator =
            isDestructive && prevAction && prevAction.variant !== "destructive";

          return (
            <span key={action.label}>
              {addSeparator && <DropdownMenuSeparator />}
              <DropdownMenuItem
                onClick={() => !isDisabled && action.onClick(row)}
                disabled={isDisabled}
                className={cn(
                  "gap-2",
                  isDestructive && "text-destructive focus:text-destructive"
                )}
              >
                {action.icon && (
                  <span className="h-4 w-4">{action.icon}</span>
                )}
                {action.label}
              </DropdownMenuItem>
            </span>
          );
        })}
      </DropdownMenuContent>
    </DropdownMenu>
  );
}

// ─── Main Table ───────────────────────────────────────────────────────────────

interface ListViewTableProps<TRow extends { id: string | number }> {
  columns: ColumnDef<TRow>[];
  rows: TRow[];
  actions?: RowAction<TRow>[];
  sort: SortState;
  onSortChange: (column: string) => void;
  isLoading: boolean;
  rowClassName?: (row: TRow) => string;
}

export function ListViewTable<TRow extends { id: string | number }>({
  columns,
  rows,
  actions = [],
  sort,
  onSortChange,
  isLoading,
  rowClassName,
}: ListViewTableProps<TRow>) {
  const totalCols = columns.length + (actions.length > 0 ? 1 : 0);

  return (
    <div className="rounded-md border">
      <Table>
        {/* ── Header ── */}
        <TableHeader>
          <TableRow>
            {columns.map((col) => (
              <TableHead
                key={col.key}
                className={cn(
                  col.hideOnMobile && "hidden sm:table-cell",
                  col.className
                )}
              >
                {col.sortable ? (
                  <button
                    onClick={() => onSortChange(col.key)}
                    className="flex items-center font-semibold hover:text-foreground transition-colors"
                  >
                    {col.label}
                    <SortIcon columnKey={col.key} sort={sort} />
                  </button>
                ) : (
                  col.label
                )}
              </TableHead>
            ))}
            {actions.length > 0 && (
              <TableHead className="w-10 text-right" aria-label="Actions" />
            )}
          </TableRow>
        </TableHeader>

        {/* ── Body ── */}
        <TableBody>
          {isLoading ? (
            <TableSkeleton
              columns={columns.length}
              hasActions={actions.length > 0}
            />
          ) : rows.length === 0 ? (
            <EmptyState colSpan={totalCols} />
          ) : (
            rows.map((row) => (
              <TableRow
                key={row.id}
                className={cn(
                  "transition-colors",
                  rowClassName?.(row)
                )}
              >
                {columns.map((col) => {
                  const rawValue = getNestedValue(row, col.key);
                  const cellContent = col.render
                    ? col.render(rawValue, row)
                    : rawValue !== null && rawValue !== undefined
                    ? String(rawValue)
                    : "—";

                  return (
                    <TableCell
                      key={col.key}
                      className={cn(
                        col.hideOnMobile && "hidden sm:table-cell",
                        col.className
                      )}
                    >
                      {cellContent}
                    </TableCell>
                  );
                })}

                {actions.length > 0 && (
                  <TableCell className="text-right">
                    <RowActionsMenu row={row} actions={actions} />
                  </TableCell>
                )}
              </TableRow>
            ))
          )}
        </TableBody>
      </Table>
    </div>
  );
}
