"use client";

import { useEffect, useState, useCallback } from "react";
import {
  AlertTriangle,
  CheckCircle2,
  ChevronDown,
  ChevronRight,
  Filter,
  Search,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
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
  getLedgerReport,
  LedgerReportResponse,
  LedgerTransaction,
} from "@/lib/api";

const TX_TYPE_LABELS: Record<string, string> = {
  escrow_fund: "Escrow Fund",
  escrow_release: "Escrow Release",
  escrow_refund: "Escrow Refund",
  platform_fee: "Platform Fee",
  referral_reward: "Referral Reward",
  referral_discount: "Referral Discount",
  payout: "Payout",
};

function typeLabel(t: string) {
  return TX_TYPE_LABELS[t] || t.replace(/_/g, " ");
}

const fmt = (pence: number) => `\u00a3${(pence / 100).toFixed(2)}`;

export default function LedgerPage() {
  const [report, setReport] = useState<LedgerReportResponse | null>(null);
  const [loading, setLoading] = useState(true);
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);
  const [typeFilter, setTypeFilter] = useState("all");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [expandedRows, setExpandedRows] = useState<Set<string>>(new Set());
  const { toast } = useToast();

  const fetchReport = useCallback(() => {
    setLoading(true);
    getLedgerReport({
      page,
      limit: pageSize,
      type: typeFilter === "all" ? undefined : typeFilter,
      from_date: fromDate || undefined,
      to_date: toDate || undefined,
    })
      .then((res) => {
        setReport(res.data.data);
      })
      .catch(() =>
        toast({
          variant: "destructive",
          title: "Error",
          description: "Failed to load ledger report.",
        })
      )
      .finally(() => setLoading(false));
  }, [page, pageSize, typeFilter, fromDate, toDate]);

  useEffect(() => {
    fetchReport();
  }, [fetchReport]);

  useEffect(() => {
    setPage(1);
  }, [typeFilter, fromDate, toDate]);

  const toggleRow = (id: string) => {
    setExpandedRows((prev) => {
      const next = new Set(prev);
      if (next.has(id)) next.delete(id);
      else next.add(id);
      return next;
    });
  };

  const transactions = report?.transactions ?? [];
  const meta = report?.meta ?? { total: 0, page: 1, limit: 20, total_pages: 0 };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold tracking-tight">General Ledger</h2>
        <p className="text-muted-foreground">
          Double-entry ledger overview and system integrity monitor.
        </p>
      </div>

      {/* System health banner */}
      {report && !report.system_healthy && (
        <Card className="border-red-500 bg-red-50">
          <CardContent className="flex items-center gap-3 py-4">
            <AlertTriangle className="h-6 w-6 text-red-600" />
            <div>
              <p className="font-semibold text-red-800">
                System Imbalance Detected
              </p>
              <p className="text-sm text-red-700">
                Total GL sum is {fmt(report.system_balance)} (expected
                &pound;0.00). Investigate immediately.
              </p>
            </div>
          </CardContent>
        </Card>
      )}
      {report && report.system_healthy && (
        <Card className="border-green-300 bg-green-50">
          <CardContent className="flex items-center gap-3 py-4">
            <CheckCircle2 className="h-5 w-5 text-green-600" />
            <p className="text-sm font-medium text-green-800">
              System balanced &mdash; all entries sum to &pound;0.00
            </p>
          </CardContent>
        </Card>
      )}

      {/* Summary cards */}
      <div className="grid gap-4 sm:grid-cols-3">
        {[
          {
            label: "Total in Escrow",
            value: report?.summary.total_escrow ?? 0,
            color: "text-blue-600",
          },
          {
            label: "Total Revenue",
            value: report?.summary.total_revenue ?? 0,
            color: "text-green-600",
          },
          {
            label: "Marketing Spend",
            value: report?.summary.total_marketing ?? 0,
            color: "text-orange-600",
          },
        ].map(({ label, value, color }) => (
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
                <p className={`text-3xl font-bold ${color}`}>{fmt(value)}</p>
              )}
            </CardContent>
          </Card>
        ))}
      </div>

      {/* Filters */}
      <div className="flex flex-col gap-3 sm:flex-row">
        <Select value={typeFilter} onValueChange={setTypeFilter}>
          <SelectTrigger className="w-[200px]">
            <Filter className="mr-2 h-4 w-4 text-muted-foreground" />
            <SelectValue placeholder="Transaction type" />
          </SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All Types</SelectItem>
            {Object.entries(TX_TYPE_LABELS).map(([val, label]) => (
              <SelectItem key={val} value={val}>
                {label}
              </SelectItem>
            ))}
          </SelectContent>
        </Select>

        <Input
          type="date"
          value={fromDate}
          onChange={(e) => setFromDate(e.target.value)}
          className="w-[160px]"
          placeholder="From date"
        />
        <Input
          type="date"
          value={toDate}
          onChange={(e) => setToDate(e.target.value)}
          className="w-[160px]"
          placeholder="To date"
        />

        {(typeFilter !== "all" || fromDate || toDate) && (
          <Button
            variant="outline"
            onClick={() => {
              setTypeFilter("all");
              setFromDate("");
              setToDate("");
            }}
          >
            Clear
          </Button>
        )}
      </div>

      {/* Transaction table */}
      <Card>
        <CardHeader>
          <CardTitle className="text-base">Ledger Transactions</CardTitle>
          <CardDescription>
            {loading
              ? "Loading..."
              : `Showing ${transactions.length} of ${meta.total} transactions`}
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="space-y-3 p-6">
              {Array.from({ length: 6 }).map((_, i) => (
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
                    <TableHead>Type</TableHead>
                    <TableHead>Reference</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Description</TableHead>
                    <TableHead>Date</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {transactions.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={7}
                        className="py-10 text-center text-muted-foreground"
                      >
                        No ledger transactions found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    transactions.map((txn) => (
                      <TransactionRow
                        key={txn.id}
                        txn={txn}
                        expanded={expandedRows.has(txn.id)}
                        onToggle={() => toggleRow(txn.id)}
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
  txn,
  expanded,
  onToggle,
}: {
  txn: LedgerTransaction;
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
            <ChevronDown className="h-4 w-4" />
          ) : (
            <ChevronRight className="h-4 w-4" />
          )}
        </TableCell>
        <TableCell className="font-mono text-xs">
          {txn.id.slice(0, 8)}&hellip;
        </TableCell>
        <TableCell>
          <Badge variant="outline">{typeLabel(txn.type)}</Badge>
        </TableCell>
        <TableCell className="font-mono text-xs text-muted-foreground">
          {txn.reference_id ? `${txn.reference_id.slice(0, 8)}\u2026` : "\u2014"}
        </TableCell>
        <TableCell>
          <Badge
            variant={txn.status === "posted" ? "default" : "secondary"}
          >
            {txn.status}
          </Badge>
        </TableCell>
        <TableCell className="max-w-[250px] truncate text-sm text-muted-foreground">
          {txn.description || "\u2014"}
        </TableCell>
        <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
          {new Date(txn.posting_date).toLocaleDateString("en-GB", {
            day: "2-digit",
            month: "short",
            year: "numeric",
          })}
        </TableCell>
      </TableRow>

      {expanded && (
        <TableRow className="bg-muted/30">
          <TableCell colSpan={7} className="p-0">
            <div className="px-8 py-4">
              <p className="mb-2 text-xs font-semibold uppercase tracking-wider text-muted-foreground">
                GL Entries
              </p>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Account</TableHead>
                    <TableHead>Type</TableHead>
                    <TableHead>Category</TableHead>
                    <TableHead className="text-right">Debit</TableHead>
                    <TableHead className="text-right">Credit</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {txn.entries.map((entry) => (
                    <TableRow key={entry.id}>
                      <TableCell className="text-sm font-medium">
                        {entry.account?.name || entry.account_id.slice(0, 8)}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground capitalize">
                        {entry.account?.type?.replace(/_/g, " ") || "\u2014"}
                      </TableCell>
                      <TableCell className="text-xs text-muted-foreground capitalize">
                        {entry.category?.replace(/_/g, " ") || "\u2014"}
                      </TableCell>
                      <TableCell className="text-right font-mono text-sm text-red-600">
                        {entry.amount < 0 ? fmt(Math.abs(entry.amount)) : ""}
                      </TableCell>
                      <TableCell className="text-right font-mono text-sm text-green-600">
                        {entry.amount > 0 ? fmt(entry.amount) : ""}
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </div>
          </TableCell>
        </TableRow>
      )}
    </>
  );
}
