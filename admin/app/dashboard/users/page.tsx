"use client";

import { useEffect, useState, useCallback } from "react";
import { useRouter } from "next/navigation";
import { Search, User, Wallet, X } from "lucide-react";
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
import api from "@/lib/api";

interface UserRecord {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
  phone_number: string;
  disabled: boolean;
  email_verified: boolean;
  phone_verified: boolean;
  created_at: string;
}

interface PaginatedMeta {
  total: number;
  page: number;
  limit: number;
  total_pages: number;
}

export default function UsersPage() {
  const router = useRouter();
  const [users, setUsers] = useState<UserRecord[]>([]);
  const [meta, setMeta] = useState<PaginatedMeta>({ total: 0, page: 1, limit: 10, total_pages: 0 });
  const [loading, setLoading] = useState(true);

  const [search, setSearch] = useState("");
  const [statusFilter, setStatusFilter] = useState("all");
  const [emailVerifiedFilter, setEmailVerifiedFilter] = useState("all");
  const [phoneVerifiedFilter, setPhoneVerifiedFilter] = useState("all");
  const [fromDate, setFromDate] = useState("");
  const [toDate, setToDate] = useState("");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  const fetchUsers = useCallback(() => {
    setLoading(true);
    const params = new URLSearchParams();
    params.set("page", String(page - 1));
    params.set("limit", String(pageSize));
    if (search) params.set("search", search);
    if (statusFilter !== "all") params.set("status", statusFilter);
    if (emailVerifiedFilter !== "all") params.set("email_verified", emailVerifiedFilter);
    if (phoneVerifiedFilter !== "all") params.set("phone_verified", phoneVerifiedFilter);
    if (fromDate) params.set("from_date", fromDate);
    if (toDate) params.set("to_date", toDate);

    api.get(`/api/v1/admin/users?${params.toString()}`)
      .then((res) => {
        const data = res.data?.data ?? res.data?.users ?? [];
        setUsers(Array.isArray(data) ? data : []);
        if (res.data?.meta) setMeta(res.data.meta);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  }, [page, pageSize, search, statusFilter, emailVerifiedFilter, phoneVerifiedFilter, fromDate, toDate]);

  useEffect(() => { fetchUsers(); }, [fetchUsers]);

  useEffect(() => {
    setPage(1);
  }, [search, statusFilter, emailVerifiedFilter, phoneVerifiedFilter, fromDate, toDate, pageSize]);

  const clearFilters = () => {
    setSearch("");
    setStatusFilter("all");
    setEmailVerifiedFilter("all");
    setPhoneVerifiedFilter("all");
    setFromDate("");
    setToDate("");
  };

  const hasFilters =
    search || statusFilter !== "all" || emailVerifiedFilter !== "all" ||
    phoneVerifiedFilter !== "all" || fromDate || toDate;

  return (
    <div className="space-y-6">
      <div>
        <h1 className="text-2xl font-bold tracking-tight">Users</h1>
        <p className="text-muted-foreground">All registered users on the platform.</p>
      </div>

      <div className="grid grid-cols-1 gap-4 sm:grid-cols-3">
        {[
          { label: "Total (filtered)", value: meta.total, color: "text-muted-foreground" },
          { label: "Active (page)", value: users.filter((u) => !u.disabled).length, color: "text-green-500" },
          { label: "Disabled (page)", value: users.filter((u) => u.disabled).length, color: "text-red-500" },
        ].map(({ label, value, color }) => (
          <Card key={label}>
            <CardHeader className="flex flex-row items-center justify-between pb-2">
              <CardTitle className="text-sm font-medium">{label}</CardTitle>
              <User className={`h-4 w-4 ${color}`} />
            </CardHeader>
            <CardContent>
              <div className="text-2xl font-bold">
                {loading ? <Skeleton className="h-8 w-16" /> : value}
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
                placeholder="Search by name, email or phone..."
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                className="pl-9"
              />
            </div>

            <Select value={statusFilter} onValueChange={setStatusFilter}>
              <SelectTrigger className="w-[140px]">
                <SelectValue placeholder="Status" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">All Statuses</SelectItem>
                <SelectItem value="active">Active</SelectItem>
                <SelectItem value="disabled">Disabled</SelectItem>
              </SelectContent>
            </Select>

            <Select value={emailVerifiedFilter} onValueChange={setEmailVerifiedFilter}>
              <SelectTrigger className="w-[160px]">
                <SelectValue placeholder="Email verified" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Email: Any</SelectItem>
                <SelectItem value="true">Email Verified</SelectItem>
                <SelectItem value="false">Email Unverified</SelectItem>
              </SelectContent>
            </Select>

            <Select value={phoneVerifiedFilter} onValueChange={setPhoneVerifiedFilter}>
              <SelectTrigger className="w-[160px]">
                <SelectValue placeholder="Phone verified" />
              </SelectTrigger>
              <SelectContent>
                <SelectItem value="all">Phone: Any</SelectItem>
                <SelectItem value="true">Phone Verified</SelectItem>
                <SelectItem value="false">Phone Unverified</SelectItem>
              </SelectContent>
            </Select>

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
                <TableHead>Name</TableHead>
                <TableHead>Email</TableHead>
                <TableHead>Phone</TableHead>
                <TableHead>Status</TableHead>
                <TableHead>Joined</TableHead>
                <TableHead></TableHead>
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
              ) : users.length === 0 ? (
                <TableRow>
                  <TableCell colSpan={6} className="text-center text-muted-foreground py-10">
                    No users found.
                  </TableCell>
                </TableRow>
              ) : (
                users.map((user) => (
                  <TableRow
                    key={user.id}
                    className="cursor-pointer hover:bg-muted/50"
                    onClick={() => router.push(`/dashboard/users/${user.id}`)}
                  >
                    <TableCell className="font-medium">
                      {user.first_name} {user.last_name}
                    </TableCell>
                    <TableCell className="text-muted-foreground">{user.email}</TableCell>
                    <TableCell className="text-muted-foreground">{user.phone_number || "—"}</TableCell>
                    <TableCell>
                      <Badge variant={user.disabled ? "destructive" : "default"}>
                        {user.disabled ? "Disabled" : "Active"}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {new Date(user.created_at).toLocaleDateString()}
                    </TableCell>
                    <TableCell>
                      <Wallet className="h-4 w-4 text-muted-foreground" />
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
