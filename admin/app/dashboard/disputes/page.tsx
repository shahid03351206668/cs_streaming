"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { AlertTriangle, Search } from "lucide-react";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { DataPagination } from "@/components/ui/data-pagination";
import { adminListDisputes, type Dispute, type DisputeStatus } from "@/lib/api";

const STATUS_BADGE: Record<DisputeStatus, "default" | "secondary" | "destructive" | "outline"> = {
  open: "destructive",
  resolved: "default",
  closed: "secondary",
};

export default function DisputesPage() {
  const router = useRouter();

  const [disputes, setDisputes] = useState<Dispute[]>([]);
  const [loading, setLoading] = useState(true);
  const [total, setTotal] = useState(0);
  const [totalPages, setTotalPages] = useState(1);

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [page, setPage] = useState(1);
  const [pageSize] = useState(20);

  const fetchDisputes = (currentPage: number, status: string) => {
    setLoading(true);
    adminListDisputes({
      page: currentPage,
      limit: pageSize,
      status: status === "all" ? undefined : status,
    })
      .then((res) => {
        setDisputes(res.data.data ?? []);
        setTotal(res.data.meta?.total ?? 0);
        setTotalPages(res.data.meta?.total_pages ?? 1);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  };

  useEffect(() => {
    fetchDisputes(page, statusFilter);
  }, [page, statusFilter]);

  // Reset to page 1 on filter change
  const handleStatusChange = (val: string) => {
    setStatusFilter(val);
    setPage(1);
  };

  const filtered = search
    ? disputes.filter((d) =>
        `${d.contract?.title ?? ""} ${d.filed_by?.first_name ?? ""} ${d.filed_by?.last_name ?? ""} ${d.reason}`
          .toLowerCase()
          .includes(search.toLowerCase())
      )
    : disputes;

  // Count cards use server total (all pages), so only show counts from current loaded page for open/resolved/closed
  const counts = {
    open: disputes.filter((d) => d.status === "open").length,
    resolved: disputes.filter((d) => d.status === "resolved").length,
    closed: disputes.filter((d) => d.status === "closed").length,
  };

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Disputes</h1>
        <p className="text-muted-foreground">View and resolve contract disputes filed by users.</p>
      </div>

      {/* Summary cards */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        {[
          { label: "Total", value: total, color: "text-foreground" },
          { label: "Open", value: statusFilter === "all" ? counts.open : (statusFilter === "open" ? total : "—"), color: "text-red-500" },
          { label: "Resolved", value: statusFilter === "all" ? counts.resolved : (statusFilter === "resolved" ? total : "—"), color: "text-green-500" },
          { label: "Closed", value: statusFilter === "all" ? counts.closed : (statusFilter === "closed" ? total : "—"), color: "text-muted-foreground" },
        ].map(({ label, value, color }) => (
          <Card key={label}>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium">{label}</CardTitle>
              <AlertTriangle className={`h-4 w-4 ${color}`} />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">
                {loading ? <Skeleton className="h-8 w-12" /> : value}
              </div>
            </CardContent>
          </Card>
        ))}
      </div>

      <Card>
        <CardHeader>
          <div className="flex flex-col sm:flex-row items-start sm:items-center gap-3">
            <div className="relative flex-1 w-full sm:max-w-sm">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search by contract, user, or reason..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9"
              />
            </div>
            <Select value={statusFilter} onValueChange={handleStatusChange}>
              <SelectTrigger className="w-full sm:w-40">
                <SelectValue placeholder="Filter by status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Statuses</SelectItem>
                <SelectItem value="open">Open</SelectItem>
                <SelectItem value="resolved">Resolved</SelectItem>
                <SelectItem value="closed">Closed</SelectItem>
              </SelectContent>
            </Select>
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Contract</TableHead>
                <TableHead>Filed By</TableHead>
                <TableHead>Reason</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Filed</TableHead>
                <TableHead>Resolved By</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                Array.from({ length: 8 }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 6 }).map((_, j) => (
                      <TableCell key={j}><Skeleton className="h-4 w-full" /></TableCell>
                    ))}
                  </TableRow>
                ))
              ) : filtered.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground py-10">
                    No disputes found.
                  </TableCell>
                </TableRow>
              ) : (
                filtered.map((d) => (
                  <TableRow
                    key={d.id}
                    className="cursor-pointer hover:bg-muted/60"
                    onClick={() => router.push(`/dashboard/disputes/${d.id}`)}
                  >
                    <TableCell>
                      <div className="font-medium max-w-[200px] truncate">
                        {d.contract?.title ?? <span className="text-muted-foreground italic">—</span>}
                      </div>
                      <div className="text-xs text-muted-foreground font-mono">
                        {d.contract_id.slice(0, 8)}…
                      </div>
                    </TableCell>
                    <TableCell>
                      <div className="font-medium">
                        {d.filed_by ? `${d.filed_by.first_name} ${d.filed_by.last_name}` : "—"}
                      </div>
                      <div className="text-xs text-muted-foreground">{d.filed_by?.email}</div>
                    </TableCell>
                    <TableCell className="max-w-[180px] truncate text-sm text-muted-foreground">
                      {d.reason}
                    </TableCell>
                    <TableCell>
                      <Badge variant={STATUS_BADGE[d.status] ?? "outline"} className="capitalize">
                        {d.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground whitespace-nowrap">
                      {new Date(d.created_at).toLocaleDateString("en-GB", {
                        day: "2-digit", month: "short", year: "numeric",
                      })}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {d.resolved_by
                        ? `${d.resolved_by.first_name} ${d.resolved_by.last_name}`
                        : "—"}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
          {!loading && (
            <DataPagination
              total={total}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={() => {}}
            />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
