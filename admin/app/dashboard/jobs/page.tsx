"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Search, Briefcase, X } from "lucide-react";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Input } from "@/components/ui/input";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { DataPagination } from "@/components/ui/data-pagination";
import api, { getCategories } from "@/lib/api";

interface JobRecord {
  id: string;
  title: string;
  description: string;
  budget: number;
  open_budget: boolean;
  status: string;
  address: string;
  created_by_id: string;
  category_id: string;
  created_at: string;
}

interface Category {
  id: string;
  name: string;
}

interface PaginatedMeta {
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

const STATUS_COLORS: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  open: "default",
  in_progress: "secondary",
  completed: "outline",
  cancelled: "destructive",
};

export default function JobsPage() {
  const router = useRouter();
  const [jobs, setJobs] = useState<JobRecord[]>([]);
  const [categories, setCategories] = useState<Category[]>([]);
  const [meta, setMeta] = useState<PaginatedMeta>({ total: 0, page: 1, limit: 10, total_pages: 0 });
  const [loading, setLoading] = useState(true);

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [categoryFilter, setCategoryFilter] = useState("all");
  const [budgetTypeFilter, setBudgetTypeFilter] = useState("all"); // all | open | fixed
  const [minBudget, setMinBudget] = useState("");
  const [maxBudget, setMaxBudget] = useState("");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  useEffect(() => {
    getCategories()
      .then((res) => {
        const data = Array.isArray(res.data) ? res.data : res.data?.data ?? [];
        setCategories(Array.isArray(data) ? data : []);
      })
      .catch(() => {});
  }, []);

  const fetchJobs = useCallback(() => {
    setLoading(true);
    const params = new URLSearchParams();
    params.set("page", String(page - 1));
    params.set("limit", String(pageSize));
    if (search) params.set("search", search);
    if (statusFilter !== "all") params.set("status", statusFilter);
    if (categoryFilter !== "all") params.set("category_id", categoryFilter);
    if (budgetTypeFilter !== "all") params.set("open_budget", budgetTypeFilter === "open" ? "true" : "false");
    if (minBudget) params.set("min_budget", minBudget);
    if (maxBudget) params.set("max_budget", maxBudget);
    if (fromDate) params.set("from_date", fromDate);
    if (toDate) params.set("to_date", toDate);

    api.get(`/api/v1/admin/jobs?${params.toString()}`)
      .then((res) => {
        const data = res.data?.data ?? [];
        setJobs(Array.isArray(data) ? data : []);
        if (res.data?.meta) setMeta(res.data.meta);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [page, pageSize, search, statusFilter, categoryFilter, budgetTypeFilter, minBudget, maxBudget, fromDate, toDate]);

  useEffect(() => { fetchJobs(); }, [fetchJobs]);

  useEffect(() => {
    setPage(1);
  }, [search, statusFilter, categoryFilter, budgetTypeFilter, minBudget, maxBudget, fromDate, toDate, pageSize]);

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("all");
    setCategoryFilter("all");
    setBudgetTypeFilter("all");
    setMinBudget("");
    setMaxBudget("");
    setFromDate("");
    setToDate("");
  };

  const hasFilters =
    search || statusFilter !== "all" || categoryFilter !== "all" || budgetTypeFilter !== "all" ||
    minBudget || maxBudget || fromDate || toDate;

  const categoryName = (id: string) => categories.find((c) => c.id === id)?.name ?? "—";

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Jobs</h1>
        <p className="text-muted-foreground">All job posts on the platform.</p>
      </div>

      <div className="grid grid-cols-2 gap-4 sm:grid-cols-4">
        {[
          { label: "Total (filtered)", value: meta.total, color: "text-foreground" },
          { label: "Open", value: jobs.filter((j) => j.status === "open").length, color: "text-blue-500" },
          { label: "In Progress", value: jobs.filter((j) => j.status === "in_progress").length, color: "text-yellow-500" },
          { label: "Completed", value: jobs.filter((j) => j.status === "completed").length, color: "text-green-500" },
        ].map(({ label, value, color }) => (
          <Card key={label}>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium">{label}</CardTitle>
              <Briefcase className={`h-4 w-4 ${color}`} />
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
          <div className="flex flex-wrap items-start gap-3">
            <div className="relative min-w-[220px] flex-1">
              <Search className="absolute left-3 top-1/2 -translate-y-1/2 h-4 w-4 text-muted-foreground" />
              <Input
                placeholder="Search by title or address..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9"
              />
            </div>

            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-[150px]">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Statuses</SelectItem>
                <SelectItem value="open">Open</SelectItem>
                <SelectItem value="in_progress">In Progress</SelectItem>
                <SelectItem value="completed">Completed</SelectItem>
                <SelectItem value="cancelled">Cancelled</SelectItem>
              </SelectContent>
            </Select>

            <Select value={categoryFilter} onValueChange={setCategoryFilter}>
              <SelectTrigger className="w-[170px]">
                <SelectValue placeholder="Category" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Categories</SelectItem>
                {categories.map((cat) => (
                  <SelectItem key={cat.id} value={cat.id}>{cat.name}</SelectItem>
                ))}
              </SelectContent>
            </Select>

            <Select value={budgetTypeFilter} onValueChange={setBudgetTypeFilter}>
              <SelectTrigger className="w-[140px]">
                <SelectValue placeholder="Budget type" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Any Budget</SelectItem>
                <SelectItem value="fixed">Fixed</SelectItem>
                <SelectItem value="open">Open</SelectItem>
              </SelectContent>
            </Select>

            <div className="flex items-center gap-2">
              <Input
                type="number"
                placeholder="Min £"
                value={minBudget}
                onChange={(e) => setMinBudget(e.target.value)}
                className="w-[90px]"
              />
              <span className="text-muted-foreground text-sm">–</span>
              <Input
                type="number"
                placeholder="Max £"
                value={maxBudget}
                onChange={(e) => setMaxBudget(e.target.value)}
                className="w-[90px]"
              />
            </div>

            <div className="flex items-center gap-2">
              <Input
                type="date"
                value={fromDate}
                onChange={(e) => setFromDate(e.target.value)}
                className="w-[150px]"
              />
              <span className="text-muted-foreground text-sm">–</span>
              <Input
                type="date"
                value={toDate}
                onChange={(e) => setToDate(e.target.value)}
                className="w-[150px]"
              />
            </div>

            {hasFilters && (
              <Button variant="outline" onClick={clearFilters} className="gap-1">
                <X className="h-4 w-4" />
                Clear
              </Button>
            )}
          </div>
        </CardHeader>
        <CardContent className="p-0">
          <Table>
            <TableHeader>
              <TableRow>
                <TableHead>Title</TableHead>
                <TableHead>Category</TableHead>
                <TableHead>Budget</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Address</TableHead>
                <TableHead>Posted</TableHead>
              </TableRow>
            </TableHeader>
            <TableBody>
              {loading ? (
                Array.from({ length: pageSize }).map((_, i) => (
                  <TableRow key={i}>
                    {Array.from({ length: 6 }).map((_, j) => (
                      <TableCell key={j}><Skeleton className="h-4 w-full" /></TableCell>
                    ))}
                  </TableRow>
                ))
              ) : jobs.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground py-10">
                    No jobs found.
                  </TableCell>
                </TableRow>
              ) : (
                jobs.map((job) => (
                  <TableRow
                    key={job.id}
                    className="cursor-pointer hover:bg-muted/60"
                    onClick={() => router.push(`/dashboard/jobs/${job.id}`)}
                  >
                    <TableCell>
                      <div className="font-medium">{job.title}</div>
                      <div className="text-xs text-muted-foreground line-clamp-1 max-w-xs">
                        {job.description}
                      </div>
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {categoryName(job.category_id)}
                    </TableCell>
                    <TableCell>
                      {job.open_budget
                        ? <span className="text-muted-foreground italic">Open</span>
                        : <span className="font-medium">£{job.budget?.toFixed(2)}</span>
                      }
                    </TableCell>
                    <TableCell>
                      <Badge variant={STATUS_COLORS[job.status] ?? "outline"}>
                        {job.status?.replace("_", " ")}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm max-w-[160px] truncate">
                      {job.address || "—"}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {new Date(job.created_at).toLocaleDateString()}
                    </TableCell>
                  </TableRow>
                ))
              )}
            </TableBody>
          </Table>
          {!loading && (
            <DataPagination
              total={meta.total}
              page={page}
              pageSize={pageSize}
              onPageChange={setPage}
              onPageSizeChange={setPageSize}
            />
          )}
        </CardContent>
      </Card>
    </div>
  );
}
