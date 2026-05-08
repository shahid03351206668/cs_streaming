import type { ReactNode } from "react";

// ─── Sorting ────────────────────────────────────────────────────────────────

export type SortDirection = "asc" | "desc";

export interface SortState {
  column: string;
  direction: SortDirection;
}

// ─── Column Definition ───────────────────────────────────────────────────────

export interface ColumnDef<TRow> {
  /** Key used for sorting; can be a dot-path string or keyof TRow */
  key: string;
  label: string;
  /** Custom cell renderer. Falls back to String(value). */
  render?: (value: unknown, row: TRow) => ReactNode;
  sortable?: boolean;
  /** Extra Tailwind classes for the <td> */
  className?: string;
  /** Hide on small screens */
  hideOnMobile?: boolean;
}

// ─── Row Actions ─────────────────────────────────────────────────────────────

export interface RowAction<TRow> {
  label: string;
  icon?: ReactNode;
  onClick: (row: TRow) => void;
  variant?: "default" | "destructive";
  /** Return true to hide the action for a given row */
  hidden?: (row: TRow) => boolean;
  /** Return true to disable (show greyed out) */
  disabled?: (row: TRow) => boolean;
}

// ─── Filters ─────────────────────────────────────────────────────────────────

export interface FilterOption {
  label: string;
  value: string;
}

export interface FilterDef {
  key: string;
  label: string;
  options: FilterOption[];
  /** Placeholder shown in the select trigger */
  placeholder?: string;
}

// ─── Query Params (sent to your API) ─────────────────────────────────────────

export interface ListViewParams {
  page: number;
  limit: number;
  search: string;
  sortColumn: string;
  sortDirection: SortDirection;
  filters: Record<string, string>;
}

// ─── API Response shape ───────────────────────────────────────────────────────

export interface PaginatedResponse<TRow> {
  data: TRow[];
  total: number;
  page: number;
  limit: number;
  totalPages: number;
}

// ─── Main Component Props ─────────────────────────────────────────────────────

export interface ListViewProps<TRow extends { id: string | number }> {
  /** Page / section title shown above the table */
  title?: string;
  /** Optional subtitle */
  description?: string;

  /** Column definitions */
  columns: ColumnDef<TRow>[];

  /**
   * React Query key. The component appends the current params so every
   * unique param combination gets its own cache entry.
   */
  queryKey: readonly unknown[];

  /**
   * Data-fetching function. Receives the current params and must return a
   * PaginatedResponse. Wire this up to your real API or a mock.
   */
  queryFn: (params: ListViewParams) => Promise<PaginatedResponse<TRow>>;

  /** Per-row dropdown actions */
  actions?: RowAction<TRow>[];

  /** Filter dropdowns rendered in the toolbar */
  filters?: FilterDef[];

  /** Rows per page options */
  pageSizeOptions?: number[];
  defaultPageSize?: number;

  /** Default sort applied on first render */
  defaultSort?: SortState;

  /** "Create" button in the toolbar */
  onCreateClick?: () => void;
  createLabel?: string;

  /** Additional Tailwind classes on the row <tr> */
  rowClassName?: (row: TRow) => string;

  /** Disable search input */
  searchable?: boolean;
  searchPlaceholder?: string;
}
