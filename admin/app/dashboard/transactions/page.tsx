"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import {
  Search,
  Filter,
  ChevronDown,
  ChevronRight,
  X,
  ExternalLink,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table";
import {
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  CardDescription,
} from "@/components/ui/card";
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select";
import { DataPagination } from "@/components/ui/data-pagination";
import { useToast } from "@/components/ui/use-toast";
import {
  getTransactions,
  PaymentTransaction,
  TransactionFilters,
  PaginatedMeta,
} from "@/lib/api";

const STATUSES = ["success", "pending", "failed", "refunded", "cancelled"];
const REFERENCE_TYPES = ["contract", "job", "proposal", "escrow", "referral"];

function statusVariant(
  status: string
): "default" | "secondary" | "destructive" | "outline" {
  switch (status?.toLowerCase()) {
    case "success":
    case "completed":
    case "paid":
      return "default";
    case "failed":
    case "cancelled":
      return "destructive";
    case "refunded":
      return "outline";
    default:
      return "secondary";
  }
}

const fmt = (pence: number, currency = "GBP") =>
  `${currency?.toUpperCase() === "GBP" ? "£" : "$"}${(pence / 100).toFixed(2)}`;

const fmtDate = (d: string) =>
  d
    ? new Date(d).toLocaleDateString("en-GB", {
        day: "2-digit",
        month: "short",
        year: "numeric",
      })
    : "—";

const fmtDateTime = (d: string) =>
  d
    ? new Date(d).toLocaleString("en-GB", {
        day: "2-digit",
        month: "short",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "—";

export default function TransactionsPage() {
  const [transactions, setTransactions] = useState<PaymentTransaction[]>([]);
  const [meta, setMeta] = useState<PaginatedMeta>({
    total: 0,
    page: 1,
    limit: 20,
    total_pages: 0,
  });
  const [loading, setLoading] = useState(true);

  // Filters
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [refTypeFilter, setRefTypeFilter] = useState("all");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  // UI state
  const [expandedRow, setExpandedRow] = useState<string | null>(null);

  const { toast } = useToast();

  const fetchTransactions = useCallback(() => {
    setLoading(true);
    const filters: TransactionFilters = {
      page,
      limit: pageSize,
      search: search || undefined,
      status: statusFilter !== "all" ? statusFilter : undefined,
      reference_type: refTypeFilter !== "all" ? refTypeFilter : undefined,
      from_date: fromDate || undefined,
      to_date: toDate || undefined,
    };
    getTransactions(filters)
      .then((res) => {
        setTransactions(res.data?.data ?? []);
        if (res.data?.meta) setMeta(res.data.meta);
      })
      .catch(() =>
        toast({
          variant: "destructive",
          title: "Error",
          description: "Failed to load transactions.",
        })
      )
      .finally(() => setLoading(false));
  }, [page, pageSize, search, statusFilter, refTypeFilter, fromDate, toDate]);

  useEffect(() => {
    fetchTransactions();
  }, [fetchTransactions]);

  // Reset to page 1 whenever filters change
  useEffect(() => {
    setPage(1);
  }, [search, statusFilter, refTypeFilter, fromDate, toDate, pageSize]);

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("all");
    setRefTypeFilter("all");
    setFromDate("");
    setToDate("");
  };

  const hasFilters =
    search || statusFilter !== "all" || refTypeFilter !== "all" || fromDate || toDate;

  // Summary stats (from current page — server returns paginated slice)
  const pageVolume = transactions.reduce((s, t) => s + (t.amount || 0), 0);
  const pageFees = transactions.reduce((s, t) => s + (t.app_fee_amount || 0), 0);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold tracking-tight">
          Payment Transactions
        </h2>
        <p className="text-muted-foreground">
          View and filter all platform payment transactions.
        </p>
      </div>

      {/* Summary cards */}
      <div className="grid gap-4 sm:grid-cols-4">
        {[
          { label: "Total Transactions", value: String(meta.total), mono: false },
          {
            label: "Page Volume",
            value: fmt(pageVolume),
            mono: true,
          },
          { label: "Page Fees Collected", value: fmt(pageFees), mono: true, green: true },
          {
            label: "Pages",
            value: `${page} / ${meta.total_pages || 1}`,
            mono: false,
          },
        ].map(({ label, value, mono, green }) => (
          <Card key={label}>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">
                {label}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {loading ? (
                <Skeleton className="h-8 w-24" />
              ) : (
                <p
                  className={`text-2xl font-bold ${mono ? "font-mono" : ""} ${
                    green ? "text-green-600" : ""
                  }`}
                >
                  {value}
                </p>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Filter bar */}
      <Card>
        <CardContent className="pt-4 pb-4">
          <div className="flex flex-wrap gap-3">
            {/* Search */}
            <div className="relative min-w-[220px] flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search ID, reference ID, intent..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9"
              />
            </div>

            {/* Status */}
            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-[150px]">
                <Filter className="mr-2 h-4 w-4 text-muted-foreground" />
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Statuses</SelectItem>
                {STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {s.charAt(0).toUpperCase() + s.slice(1)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {/* Reference Type */}
            <Select value={refTypeFilter} onValueChange={setRefTypeFilter}>
              <SelectTrigger className="w-[160px]">
                <SelectValue placeholder="Reference type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Types</SelectItem>
                {REFERENCE_TYPES.map((t) => (
                  <SelectItem key={t} value={t}>
                    {t.charAt(0).toUpperCase() + t.slice(1)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

            {/* Date range */}
            <div className="flex items-center gap-2">
              <Input
                type="date"
                value={fromDate}
                onChange={(e) => setFromDate(e.target.value)}
                className="w-[150px]"
                placeholder="From date"
              />
              <span className="text-muted-foreground text-sm">–</span>
              <Input
                type="date"
                value={toDate}
                onChange={(e) => setToDate(e.target.value)}
                className="w-[150px]"
                placeholder="To date"
              />
            </div>

            {hasFilters && (
              <Button variant="outline" onClick={clearFilters} className="gap-1">
                <X className="h-4 w-4" />
                Clear
              </Button>
            )}
          </div>
        </CardContent>
      </Card>

      {/* Table */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Transactions</CardTitle>
          <CardDescription>
            {loading
              ? "Loading..."
              : `${meta.total} total — showing page ${page} of ${meta.total_pages || 1}`}
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="space-y-3 p-6">
              {Array.from({ length: 8 }).map((_, i) => (
                <Skeleton key={i} className="h-12 w-full" />
              ))}
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-8" />
                    <TableHead>ID</TableHead>
                    <TableHead>From</TableHead>
                    <TableHead>To</TableHead>
                    <TableHead className="text-right">Amount</TableHead>
                    <TableHead className="text-right">Fee</TableHead>
                    <TableHead className="text-right">Net</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Ref Type</TableHead>
                    <TableHead>Date</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {transactions.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={11}
                        className="py-12 text-center text-muted-foreground"
                      >
                        No transactions found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    transactions.map((tx) => (
                      <TransactionRow
                        key={tx.id}
                        tx={tx}
                        expanded={expandedRow === tx.id}
                        onToggle={() =>
                          setExpandedRow((prev) =>
                            prev === tx.id ? null : tx.id
                          )
                        }
                      />
                    ))
                  )}
                </TableBody>
              </Table>
              <DataPagination
                total={meta.total}
                page={page}
                pageSize={pageSize}
                onPageChange={setPage}
                onPageSizeChange={setPageSize}
              />
            </>
          )}
        </CardContent>
      </Card>
    </div>
  );
}

function TransactionRow({
  tx,
  expanded,
  onToggle,
}: {
  tx: PaymentTransaction;
  expanded: boolean;
  onToggle: () => void;
}) {
  return (
    <>
      <TableRow
        className="cursor-pointer hover:bg-muted/50"
        onClick={onToggle}
      >
        <TableCell>
          {expanded ? (
            <ChevronDown className="h-4 w-4 text-muted-foreground" />
          ) : (
            <ChevronRight className="h-4 w-4 text-muted-foreground" />
          )}
        </TableCell>
        <TableCell className="font-mono text-xs text-muted-foreground">
          {tx.id?.slice(0, 8)}&hellip;
        </TableCell>
        <TableCell>
          <div className="text-sm font-medium">
            {tx.from_user?.first_name} {tx.from_user?.last_name}
          </div>
          <div className="text-xs text-muted-foreground">{tx.from_user?.email}</div>
        </TableCell>
        <TableCell>
          <div className="text-sm font-medium">
            {tx.to_user?.first_name} {tx.to_user?.last_name}
          </div>
          <div className="text-xs text-muted-foreground">{tx.to_user?.email}</div>
        </TableCell>
        <TableCell className="text-right font-mono font-semibold">
          {fmt(tx.amount, tx.currency)}
        </TableCell>
        <TableCell className="text-right font-mono text-sm text-orange-600">
          {tx.app_fee_amount ? fmt(tx.app_fee_amount, tx.currency) : "—"}
        </TableCell>
        <TableCell className="text-right font-mono text-sm text-green-600">
          {fmt(tx.net_amount, tx.currency)}
        </TableCell>
        <TableCell>
          <Badge variant={statusVariant(tx.status)}>
            {tx.status
              ? tx.status.charAt(0).toUpperCase() + tx.status.slice(1)
              : "—"}
          </Badge>
        </TableCell>
        <TableCell className="text-sm capitalize text-muted-foreground">
          {tx.reference_type?.replace(/_/g, " ") || "—"}
        </TableCell>
        <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
          {fmtDate(tx.transaction_date || tx.created_at)}
        </TableCell>
        <TableCell>
          <Link
            href={`/dashboard/transactions/${tx.id}`}
            onClick={(e) => e.stopPropagation()}
          >
            <Button variant="ghost" size="icon" className="h-7 w-7">
              <ExternalLink className="h-3.5 w-3.5" />
            </Button>
          </Link>
        </TableCell>
      </TableRow>

      {expanded && (
        <TableRow className="bg-muted/20 hover:bg-muted/20">
          <TableCell colSpan={11} className="p-0">
            <div className="px-8 py-5 space-y-4">
              {/* IDs row */}
              <div className="grid grid-cols-2 gap-x-8 gap-y-2 sm:grid-cols-4 text-sm">
                <DetailField label="Transaction ID" value={tx.id} mono />
                <DetailField
                  label="Reference ID"
                  value={tx.reference_id || "—"}
                  mono
                />
                <DetailField
                  label="Referral Code ID"
                  value={tx.referral_code_id || "—"}
                  mono
                />
                <DetailField
                  label="Payment Method"
                  value={tx.payment_method?.replace(/_/g, " ") || "—"}
                />
              </div>

              <Separator />

              {/* Users */}
              <div className="grid grid-cols-1 gap-4 sm:grid-cols-2">
                <div>
                  <p className="mb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    From (Client)
                  </p>
                  <div className="space-y-1 text-sm">
                    <DetailField
                      label="Name"
                      value={`${tx.from_user?.first_name ?? ""} ${tx.from_user?.last_name ?? ""}`.trim() || "—"}
                    />
                    <DetailField label="Email" value={tx.from_user?.email || "—"} />
                    <DetailField label="User ID" value={tx.from_user?.id || "—"} mono />
                  </div>
                </div>
                <div>
                  <p className="mb-1 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                    To (Freelancer)
                  </p>
                  <div className="space-y-1 text-sm">
                    <DetailField
                      label="Name"
                      value={`${tx.to_user?.first_name ?? ""} ${tx.to_user?.last_name ?? ""}`.trim() || "—"}
                    />
                    <DetailField label="Email" value={tx.to_user?.email || "—"} />
                    <DetailField label="User ID" value={tx.to_user?.id || "—"} mono />
                  </div>
                </div>
              </div>

              <Separator />

              {/* Amounts breakdown */}
              <div>
                <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                  Amounts
                </p>
                <div className="grid grid-cols-2 gap-x-8 gap-y-1 sm:grid-cols-4 text-sm">
                  <DetailField
                    label="Gross Amount"
                    value={fmt(tx.amount, tx.currency)}
                    mono
                  />
                  <DetailField
                    label="App Fee"
                    value={fmt(tx.app_fee_amount, tx.currency)}
                    mono
                    colored="orange"
                  />
                  <DetailField
                    label="Discount"
                    value={
                      tx.discount_amount
                        ? fmt(tx.discount_amount, tx.currency)
                        : "—"
                    }
                    mono
                    colored="blue"
                  />
                  <DetailField
                    label="Net Amount"
                    value={fmt(tx.net_amount, tx.currency)}
                    mono
                    colored="green"
                  />
                </div>
              </div>

              <Separator />

              {/* Dates */}
              <div className="grid grid-cols-2 gap-x-8 gap-y-1 sm:grid-cols-3 text-sm">
                <DetailField
                  label="Transaction Date"
                  value={fmtDateTime(tx.transaction_date)}
                />
                <DetailField
                  label="Created At"
                  value={fmtDateTime(tx.created_at)}
                />
                <DetailField
                  label="Currency"
                  value={tx.currency?.toUpperCase() || "—"}
                />
              </div>
            </div>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}

function DetailField({
  label,
  value,
  mono,
  colored,
}: {
  label: string;
  value: string;
  mono?: boolean;
  colored?: "green" | "orange" | "blue";
}) {
  const colorClass =
    colored === "green"
      ? "text-green-600"
      : colored === "orange"
      ? "text-orange-600"
      : colored === "blue"
      ? "text-blue-600"
      : "";
  return (
    <div>
      <span className="text-xs text-muted-foreground">{label}</span>
      <p
        className={`text-sm font-medium break-all ${mono ? "font-mono" : ""} ${colorClass}`}
      >
        {value}
      </p>
    </div>
  );
}
