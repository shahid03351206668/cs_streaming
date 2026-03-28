"use client";

import { useEffect, useState } from "react";
import { Search, Filter } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Card, CardContent, CardHeader, CardTitle, CardDescription } from "@/components/ui/card";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { DataPagination } from "@/components/ui/data-pagination";
import { useToast } from "@/components/ui/use-toast";
import { getTransactions, PaymentTransaction } from "@/lib/api";

function statusVariant(status: string): "default" | "secondary" | "destructive" | "outline" {
  switch (status?.toLowerCase()) {
    case "success": case "completed": case "paid": return "default";
    case "failed": case "cancelled": case "refunded": return "destructive";
    default: return "secondary";
  }
}

export default function TransactionsPage() {
  const [transactions, setTransactions] = useState<PaymentTransaction[]>([]);
  const [loading, setLoading] = useState(true);
  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);
  const { toast } = useToast();

  useEffect(() => {
    getTransactions()
      .then((res) => {
        const data = Array.isArray(res.data)
          ? res.data
          : (res.data as { data?: PaymentTransaction[] })?.data ?? [];
        setTransactions(data);
      })
      .catch(() => toast({ variant: "destructive", title: "Error", description: "Failed to load transactions." }))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => { setPage(1); }, [search, statusFilter]);

  const filtered = transactions.filter((t) => {
    const matchStatus = statusFilter === "all" || t.status?.toLowerCase() === statusFilter;
    const q = search.toLowerCase();
    const matchSearch = !search || [t.id, t.from_user_id, t.to_user_id, t.reference_type, t.payment_method]
      .some((v) => v?.toLowerCase().includes(q));
    return matchStatus && matchSearch;
  });

  const paginated = filtered.slice((page - 1) * pageSize, page * pageSize);

  const totalRevenue = transactions
    .filter((t) => ["success", "completed"].includes(t.status?.toLowerCase()))
    .reduce((s, t) => s + (t.amount || 0), 0);

  const fmt = (p: number, currency = "GBP") =>
    `${currency === "GBP" ? "£" : "$"}${(p / 100).toFixed(2)}`;

  const uniqueStatuses = [...new Set(transactions.map((t) => t.status).filter(Boolean))];

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold tracking-tight">Transactions</h2>
        <p className="text-muted-foreground">View and monitor all payment transactions.</p>
      </div>

      <div className="grid gap-4 sm:grid-cols-3">
        {[
          { label: "Total Transactions", display: String(transactions.length) },
          { label: "Total Volume", display: fmt(transactions.reduce((s, t) => s + (t.amount || 0), 0)) },
          { label: "Successful Revenue", display: fmt(totalRevenue), green: true },
        ].map(({ label, display, green }) => (
          <Card key={label}>
            <CardHeader className="pb-2">
              <CardTitle className="text-sm font-medium text-muted-foreground">{label}</CardTitle>
            </CardHeader>
            <CardContent>
              {loading
                ? <Skeleton className="h-8 w-24" />
                : <p className={`text-3xl font-bold ${green ? "text-green-600" : ""}`}>{display}</p>
              }
            </CardContent>
          </Card>
        ))}
      </div>

      <div className="flex flex-col gap-3 sm:flex-row">
        <div className="relative flex-1">
          <Search className="absolute left-3 top-1/2 h-4 w-4 -translate-y-1/2 text-muted-foreground" />
          <Input
            placeholder="Search by ID, user, reference type..."
            value={search}
            onChange={(e) => setSearch(e.target.value)}
            className="pl-9"
          />
        </div>
        <div className="flex gap-2">
          <Select value={statusFilter} onValueChange={setStatusFilter}>
            <SelectTrigger className="w-[160px]">
              <Filter className="mr-2 h-4 w-4 text-muted-foreground" />
              <SelectValue placeholder="Filter status" />
            </SelectTrigger>
            <SelectContent>
              <SelectItem value="all">All Statuses</SelectItem>
              {uniqueStatuses.map((s) => (
                <SelectItem key={s} value={s.toLowerCase()}>
                  {s.charAt(0).toUpperCase() + s.slice(1)}
                </SelectItem>
              ))}
            </SelectContent>
          </Select>
          {(search || statusFilter !== "all") && (
            <Button variant="outline" onClick={() => { setSearch(""); setStatusFilter("all"); }}>
              Clear
            </Button>
          )}
        </div>
      </div>

      <Card>
        <CardHeader>
          <CardTitle className="text-base">Payment Transactions</CardTitle>
          <CardDescription>
            {loading ? "Loading..." : `Showing ${filtered.length} of ${transactions.length} transactions`}
          </CardDescription>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 6 }).map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>ID</TableHead>
                    <TableHead>From</TableHead>
                    <TableHead>To</TableHead>
                    <TableHead className="text-right">Amount</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Method</TableHead>
                    <TableHead>Reference</TableHead>
                    <TableHead>Date</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {paginated.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={8} className="text-center text-muted-foreground py-10">
                        No transactions found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    paginated.map((tx) => (
                      <TableRow key={tx.id}>
                        <TableCell className="font-mono text-xs">{tx.id?.slice(0, 8)}…</TableCell>
                        <TableCell className="font-mono text-xs text-muted-foreground">{tx.from_user_id?.slice(0, 8)}…</TableCell>
                        <TableCell className="font-mono text-xs text-muted-foreground">{tx.to_user_id?.slice(0, 8)}…</TableCell>
                        <TableCell className="text-right font-semibold">{fmt(tx.amount, tx.currency)}</TableCell>
                        <TableCell>
                          <Badge variant={statusVariant(tx.status)}>
                            {tx.status ? tx.status.charAt(0).toUpperCase() + tx.status.slice(1) : "—"}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-sm capitalize text-muted-foreground">
                          {tx.payment_method?.replace(/_/g, " ") || "—"}
                        </TableCell>
                        <TableCell className="text-sm capitalize text-muted-foreground">
                          {tx.reference_type?.replace(/_/g, " ") || "—"}
                        </TableCell>
                        <TableCell className="text-sm text-muted-foreground whitespace-nowrap">
                          {tx.created_at ? new Date(tx.created_at).toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" }) : "—"}
                        </TableCell>
                      </TableRow>
                    ))
                  )}
                </TableBody>
              </Table>
              <DataPagination
                total={filtered.length}
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
