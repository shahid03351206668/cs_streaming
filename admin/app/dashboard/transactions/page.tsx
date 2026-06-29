"use client";

import { useEffect, useState, useCallback } from "react";
import Link from "next/link";
import { Search, Filter, X, ExternalLink } from "lucide-react";
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
  Chart as ChartJS,
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  ArcElement,
  Tooltip,
  Legend,
} from "chart.js";
import { Line, Doughnut } from "react-chartjs-2";
import {
  getTransactionsV3,
  getPaymentV3Stats,
  PaymentTransactionV3,
  PaymentV3Stats,
  TransactionV3Filters,
  PaginatedMeta,
} from "@/lib/api";

ChartJS.register(
  CategoryScale,
  LinearScale,
  PointElement,
  LineElement,
  BarElement,
  ArcElement,
  Tooltip,
  Legend
);

const V3_STATUS_COLORS: Record<string, string> = {
  held: "#f59e0b",
  released: "#22c55e",
  refunded: "#6b7280",
};

const V3_STATUSES = ["held", "released", "refunded"];

function statusVariant(
  status: string
): "default" | "secondary" | "destructive" | "outline" {
  switch (status?.toLowerCase()) {
    case "released":
      return "default";
    case "refunded":
      return "outline";
    case "held":
      return "secondary";
    default:
      return "secondary";
  }
}

// gross_amount/platform_fee/net_amount on v3 are already major-unit decimals
// (shopspring/decimal serialized as a quoted JSON string) — no /100 division.
const fmtDecimal = (value: string | number | undefined, currency = "USD") => {
  const n = typeof value === "string" ? parseFloat(value) : value ?? 0;
  return `${currency?.toUpperCase() === "GBP" ? "£" : "$"}${n.toFixed(2)}`;
};

const fmtDate = (d: string) =>
  d
    ? new Date(d).toLocaleDateString("en-GB", {
        day: "2-digit",
        month: "short",
        year: "numeric",
      })
    : "—";

export default function TransactionsPage() {
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

      <V3TransactionsTab />
    </div>
  );
}

function V3PaymentCharts() {
  const [stats, setStats] = useState<PaymentV3Stats | null>(null);
  const [loading, setLoading] = useState(true);
  const { toast } = useToast();

  useEffect(() => {
    getPaymentV3Stats(30)
      .then((res) => setStats(res.data?.data ?? null))
      .catch(() =>
        toast({
          variant: "destructive",
          title: "Error",
          description: "Failed to load payment stats.",
        })
      )
      .finally(() => setLoading(false));
  }, []);

  if (loading) {
    return (
      <div className="grid gap-4 sm:grid-cols-3">
        <Skeleton className="h-72 w-full sm:col-span-2" />
        <Skeleton className="h-72 w-full" />
      </div>
    );
  }

  if (!stats) return null;

  const dailyRows = stats.daily ?? [];
  const statusRows = stats.status_breakdown ?? [];

  const labels = dailyRows.map((d) =>
    new Date(d.date).toLocaleDateString("en-GB", { day: "2-digit", month: "short" })
  );

  const lineData = {
    labels,
    datasets: [
      {
        label: "Gross Volume",
        data: dailyRows.map((d) => d.gross_volume),
        borderColor: "#3b82f6",
        backgroundColor: "rgba(59, 130, 246, 0.1)",
        tension: 0.3,
        fill: true,
      },
      {
        label: "Platform Fee",
        data: dailyRows.map((d) => d.platform_fee),
        borderColor: "#f59e0b",
        backgroundColor: "rgba(245, 158, 11, 0.1)",
        tension: 0.3,
        fill: true,
      },
    ],
  };

  const doughnutData = {
    labels: statusRows.map(
      (s) => s.status.charAt(0).toUpperCase() + s.status.slice(1)
    ),
    datasets: [
      {
        data: statusRows.map((s) => s.count),
        backgroundColor: statusRows.map(
          (s) => V3_STATUS_COLORS[s.status] ?? "#94a3b8"
        ),
        borderWidth: 0,
      },
    ],
  };

  return (
    <div className="grid gap-4 sm:grid-cols-3">
      <Card className="sm:col-span-2">
        <CardHeader>
          <CardTitle className="text-base">Volume & Fees (Last 30 Days)</CardTitle>
          <CardDescription>
            Total volume {fmtDecimal(stats.summary.total_volume)} · fees{" "}
            {fmtDecimal(stats.summary.total_fees)} · net {fmtDecimal(stats.summary.total_net)}
          </CardDescription>
        </CardHeader>
        <CardContent className="h-64">
          <Line
            data={lineData}
            options={{
              responsive: true,
              maintainAspectRatio: false,
              plugins: { legend: { position: "bottom" } },
              scales: { y: { beginAtZero: true } },
            }}
          />
        </CardContent>
      </Card>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Status Breakdown</CardTitle>
          <CardDescription>Held vs released vs refunded</CardDescription>
        </CardHeader>
        <CardContent className="h-64">
          <Doughnut
            data={doughnutData}
            options={{
              responsive: true,
              maintainAspectRatio: false,
              plugins: { legend: { position: "bottom" } },
            }}
          />
        </CardContent>
      </Card>
    </div>
  );
}

function V3TransactionsTab() {
  const [transactions, setTransactions] = useState<PaymentTransactionV3[]>([]);
  const [meta, setMeta] = useState<PaginatedMeta>({
    total: 0,
    page: 1,
    limit: 20,
    total_pages: 0,
  });
  const [loading, setLoading] = useState(true);

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const { toast } = useToast();

  const fetchTransactions = useCallback(() => {
    setLoading(true);
    const filters: TransactionV3Filters = {
      page,
      limit: pageSize,
      search: search || undefined,
      status: statusFilter !== "all" ? statusFilter : undefined,
      from_date: fromDate || undefined,
      to_date: toDate || undefined,
    };
    getTransactionsV3(filters)
      .then((res) => {
        setTransactions(res.data?.data ?? []);
        if (res.data?.meta) setMeta(res.data.meta);
      })
      .catch(() =>
        toast({
          variant: "destructive",
          title: "Error",
          description: "Failed to load V3 transactions.",
        })
      )
      .finally(() => setLoading(false));
  }, [page, pageSize, search, statusFilter, fromDate, toDate]);

  useEffect(() => {
    fetchTransactions();
  }, [fetchTransactions]);

  useEffect(() => {
    setPage(1);
  }, [search, statusFilter, fromDate, toDate, pageSize]);

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("all");
    setFromDate("");
    setToDate("");
  };

  const hasFilters = search || statusFilter !== "all" || fromDate || toDate;

  const pageVolume = transactions.reduce(
    (s, t) => s + parseFloat(t.gross_amount || "0"),
    0
  );
  const pageFees = transactions.reduce(
    (s, t) => s + parseFloat(t.platform_fee || "0"),
    0
  );

  return (
    <div className="space-y-6 pt-4">
      <V3PaymentCharts />

      <div className="grid gap-4 sm:grid-cols-4">
        {[
          { label: "Total Transactions", value: String(meta.total), mono: false },
          { label: "Page Volume", value: fmtDecimal(pageVolume), mono: true },
          {
            label: "Page Platform Fees",
            value: fmtDecimal(pageFees),
            mono: true,
            green: true,
          },
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

      <Card>
        <CardContent className="pt-4 pb-4">
          <div className="flex flex-wrap gap-3">
            <div className="relative min-w-[220px] flex-1">
              <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
              <Input
                placeholder="Search ID, contract ID, payment intent..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9"
              />
            </div>

            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-[150px]">
                <Filter className="mr-2 h-4 w-4 text-muted-foreground" />
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Statuses</SelectItem>
                {V3_STATUSES.map((s) => (
                  <SelectItem key={s} value={s}>
                    {s.charAt(0).toUpperCase() + s.slice(1)}
                  </SelectItem>
                ))}
              </SelectContent>
            </Select>

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

      <Card>
        <CardHeader>
          <CardTitle className="text-base">V3 Transactions</CardTitle>
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
                    <TableHead>ID</TableHead>
                    <TableHead>From</TableHead>
                    <TableHead>To</TableHead>
                    <TableHead className="text-right">Gross</TableHead>
                    <TableHead className="text-right">Platform Fee</TableHead>
                    <TableHead className="text-right">Net</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Contract</TableHead>
                    <TableHead>Date</TableHead>
                    <TableHead />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {transactions.length === 0 ? (
                    <TableRow>
                      <TableCell
                        colSpan={10}
                        className="py-12 text-center text-muted-foreground"
                      >
                        No V3 transactions found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    transactions.map((tx) => (
                      <TableRow key={tx.id} className="hover:bg-muted/50">
                        <TableCell className="font-mono text-xs text-muted-foreground">
                          {tx.id?.slice(0, 8)}&hellip;
                        </TableCell>
                        <TableCell>
                          <div className="text-sm font-medium">
                            {tx.from_user?.first_name} {tx.from_user?.last_name}
                          </div>
                          <div className="text-xs text-muted-foreground">
                            {tx.from_user?.email}
                          </div>
                        </TableCell>
                        <TableCell>
                          <div className="text-sm font-medium">
                            {tx.to_user?.first_name} {tx.to_user?.last_name}
                          </div>
                          <div className="text-xs text-muted-foreground">
                            {tx.to_user?.email}
                          </div>
                        </TableCell>
                        <TableCell className="text-right font-mono font-semibold">
                          {fmtDecimal(tx.gross_amount, tx.currency)}
                        </TableCell>
                        <TableCell className="text-right font-mono text-sm text-orange-600">
                          {fmtDecimal(tx.platform_fee, tx.currency)}
                        </TableCell>
                        <TableCell className="text-right font-mono text-sm text-green-600">
                          {fmtDecimal(tx.net_amount, tx.currency)}
                        </TableCell>
                        <TableCell>
                          <Badge variant={statusVariant(tx.status)}>
                            {tx.status
                              ? tx.status.charAt(0).toUpperCase() + tx.status.slice(1)
                              : "—"}
                          </Badge>
                        </TableCell>
                        <TableCell className="font-mono text-xs text-muted-foreground">
                          {tx.contract_id?.slice(0, 8)}&hellip;
                        </TableCell>
                        <TableCell className="whitespace-nowrap text-sm text-muted-foreground">
                          {fmtDate(tx.created_at)}
                        </TableCell>
                        <TableCell>
                          <Link href={`/dashboard/transactions/v3/${tx.id}`}>
                            <Button variant="ghost" size="icon" className="h-7 w-7">
                              <ExternalLink className="h-3.5 w-3.5" />
                            </Button>
                          </Link>
                        </TableCell>
                      </TableRow>
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

