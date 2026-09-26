"use client";

import { useCallback, useEffect, useState } from "react";
import { Banknote, AlertTriangle, CheckCircle2, Circle } from "lucide-react";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Skeleton } from "@/components/ui/skeleton";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs";
import { DataPagination } from "@/components/ui/data-pagination";
import { useToast } from "@/components/ui/use-toast";
import api from "@/lib/api";

interface AdminUser {
  id: string;
  name: string;
  email: string;
  has_payout_account: boolean;
}

interface EscrowRow {
  id: string;
  status: string;
  amount: number;
  currency: string;
  held_at: string;
  released_at: string | null;
  stripe_transfer_id: string;
  released_by?: string;
  release_note?: string;
  contract: {
    id: string;
    title: string;
    status: string;
    client_completed: boolean;
    freelancer_completed: boolean;
    open_disputes: number;
  };
  freelancer: AdminUser;
  client: AdminUser;
  can_release: boolean;
  block_reason?: string;
}

interface WithdrawalRow {
  id: string;
  user: AdminUser;
  amount: number;
  currency: string;
  status: string;
  bank_last4: string;
  stripe_payout_id: string;
  failure_message: string;
  arrival_date: string | null;
  created_at: string;
}

interface Meta {
  total: number;
}

const STATUS_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  held: "secondary",
  releasing: "outline",
  released: "default",
  refunded: "destructive",
  pending: "secondary",
  in_transit: "outline",
  paid: "default",
  failed: "destructive",
  canceled: "destructive",
};

const money = (amount: number, currency: string) =>
  `${currency?.toLowerCase() === "gbp" ? "£" : ""}${amount.toFixed(2)}`;

function apiError(err: unknown, fallback: string) {
  const e = err as { response?: { status?: number; data?: { error?: string } } };
  if (e.response?.status === 403) return "Your account needs the admin role to manage payouts.";
  return e.response?.data?.error ?? fallback;
}

function Tick({ done, label }: { done: boolean; label: string }) {
  return (
    <span className="flex items-center gap-1 text-xs">
      {done ? <CheckCircle2 className="h-3.5 w-3.5 text-green-600" /> : <Circle className="h-3.5 w-3.5 text-muted-foreground" />}
      {label}
    </span>
  );
}

function EscrowsTab() {
  const { toast } = useToast();
  const [rows, setRows] = useState<EscrowRow[]>([]);
  const [meta, setMeta] = useState<Meta>({ total: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("held");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  const [selected, setSelected] = useState<EscrowRow | null>(null);
  const [note, setNote] = useState("");
  const [releasing, setReleasing] = useState(false);

  const fetchRows = useCallback(() => {
    setLoading(true);
    setError("");
    const params = new URLSearchParams({ page: String(page), limit: String(pageSize) });
    if (status !== "all") params.set("status", status);
    api.get(`/api/v3/admin/payouts/escrows?${params}`)
      .then((res) => {
        setRows(res.data?.data ?? []);
        if (res.data?.meta) setMeta(res.data.meta);
      })
      .catch((err) => setError(apiError(err, "Failed to load payments")))
      .finally(() => setLoading(false));
  }, [page, pageSize, status]);

  useEffect(() => { fetchRows(); }, [fetchRows]);
  useEffect(() => { setPage(1); }, [status, pageSize]);

  const release = () => {
    if (!selected || !note.trim()) return;
    setReleasing(true);
    api.post(`/api/v3/admin/payouts/escrows/${selected.id}/release`, { note: note.trim() })
      .then(() => {
        toast({ title: "Payment released", description: `${money(selected.amount, selected.currency)} sent to ${selected.freelancer.name}.` });
        setSelected(null);
        setNote("");
        fetchRows();
      })
      .catch((err) => toast({ variant: "destructive", title: "Release failed", description: apiError(err, "Could not release payment") }))
      .finally(() => setReleasing(false));
  };

  const bothComplete = selected?.contract.client_completed && selected?.contract.freelancer_completed;

  return (
    <Card>
      <CardHeader>
        <Select value={status} onValueChange={setStatus}>
          <SelectTrigger className="w-[180px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="held">Held (awaiting release)</SelectItem>
            <SelectItem value="releasing">Releasing</SelectItem>
            <SelectItem value="released">Released</SelectItem>
            <SelectItem value="refunded">Refunded</SelectItem>
          </SelectContent>
        </Select>
      </CardHeader>
      <CardContent className="p-0">
        {error && (
          <div className="mx-4 mb-4 flex items-center gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
            <AlertTriangle className="h-4 w-4" /> {error}
          </div>
        )}
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Contract</TableHead>
              <TableHead>Freelancer</TableHead>
              <TableHead>Client</TableHead>
              <TableHead>Amount</TableHead>
              <TableHead>Completion</TableHead>
              <TableHead>Status</TableHead>
              <TableHead className="text-right">Action</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 7 }).map((_, j) => (
                    <TableCell key={j}><Skeleton className="h-4 w-full" /></TableCell>
                  ))}
                </TableRow>
              ))
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={7} className="py-10 text-center text-muted-foreground">No payments found.</TableCell>
              </TableRow>
            ) : (
              rows.map((r) => (
                <TableRow key={r.id}>
                  <TableCell>
                    <div className="font-medium">{r.contract.title || "—"}</div>
                    <div className="text-xs text-muted-foreground">
                      {r.contract.status}
                      {r.contract.open_disputes > 0 && <span className="ml-2 text-destructive">• {r.contract.open_disputes} open dispute(s)</span>}
                    </div>
                  </TableCell>
                  <TableCell>
                    <div className="text-sm">{r.freelancer.name || "—"}</div>
                    <div className="text-xs text-muted-foreground">{r.freelancer.email}</div>
                  </TableCell>
                  <TableCell>
                    <div className="text-sm">{r.client.name || "—"}</div>
                    <div className="text-xs text-muted-foreground">{r.client.email}</div>
                  </TableCell>
                  <TableCell className="font-medium">{money(r.amount, r.currency)}</TableCell>
                  <TableCell>
                    <div className="space-y-0.5">
                      <Tick done={r.contract.client_completed} label="Client" />
                      <Tick done={r.contract.freelancer_completed} label="Freelancer" />
                    </div>
                  </TableCell>
                  <TableCell>
                    <Badge variant={STATUS_VARIANT[r.status] ?? "outline"}>{r.status}</Badge>
                    {r.released_by && <div className="mt-1 text-xs text-muted-foreground">admin: {r.release_note}</div>}
                  </TableCell>
                  <TableCell className="text-right">
                    {r.can_release ? (
                      <Button size="sm" onClick={() => setSelected(r)}>Release</Button>
                    ) : (
                      <span className="text-xs text-muted-foreground">{r.block_reason}</span>
                    )}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        {!loading && (
          <DataPagination total={meta.total} page={page} pageSize={pageSize} onPageChange={setPage} onPageSizeChange={setPageSize} />
        )}
      </CardContent>

      <Dialog open={!!selected} onOpenChange={(open) => { if (!open) { setSelected(null); setNote(""); } }}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>Release payment to freelancer</DialogTitle>
            <DialogDescription>
              {selected && <>Transfer <strong>{money(selected.amount, selected.currency)}</strong> to {selected.freelancer.name} for &quot;{selected.contract.title}&quot;. This cannot be undone.</>}
            </DialogDescription>
          </DialogHeader>
          {selected && !bothComplete && (
            <div className="flex gap-2 rounded-md border border-yellow-500/40 bg-yellow-500/10 p-3 text-sm">
              <AlertTriangle className="h-4 w-4 shrink-0 text-yellow-600" />
              Admin override: both parties have not confirmed completion yet.
            </div>
          )}
          <textarea
            className="min-h-[90px] w-full rounded-md border bg-background p-2 text-sm"
            placeholder="Reason for release (required, saved to audit trail)"
            value={note}
            onChange={(e) => setNote(e.target.value)}
          />
          <DialogFooter>
            <Button variant="outline" onClick={() => setSelected(null)} disabled={releasing}>Cancel</Button>
            <Button onClick={release} disabled={releasing || !note.trim()}>
              {releasing ? "Releasing..." : "Release payment"}
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </Card>
  );
}

function WithdrawalsTab() {
  const [rows, setRows] = useState<WithdrawalRow[]>([]);
  const [meta, setMeta] = useState<Meta>({ total: 0 });
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [status, setStatus] = useState("all");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(20);

  useEffect(() => {
    setLoading(true);
    setError("");
    const params = new URLSearchParams({ page: String(page), limit: String(pageSize) });
    if (status !== "all") params.set("status", status);
    api.get(`/api/v3/admin/payouts/withdrawals?${params}`)
      .then((res) => {
        setRows(res.data?.data ?? []);
        if (res.data?.meta) setMeta(res.data.meta);
      })
      .catch((err) => setError(apiError(err, "Failed to load withdrawals")))
      .finally(() => setLoading(false));
  }, [page, pageSize, status]);

  useEffect(() => { setPage(1); }, [status, pageSize]);

  return (
    <Card>
      <CardHeader>
        <Select value={status} onValueChange={setStatus}>
          <SelectTrigger className="w-[160px]"><SelectValue /></SelectTrigger>
          <SelectContent>
            <SelectItem value="all">All statuses</SelectItem>
            <SelectItem value="pending">Pending</SelectItem>
            <SelectItem value="in_transit">In transit</SelectItem>
            <SelectItem value="paid">Paid</SelectItem>
            <SelectItem value="failed">Failed</SelectItem>
            <SelectItem value="canceled">Canceled</SelectItem>
          </SelectContent>
        </Select>
      </CardHeader>
      <CardContent className="p-0">
        {error && (
          <div className="mx-4 mb-4 flex items-center gap-2 rounded-md border border-destructive/40 bg-destructive/10 p-3 text-sm text-destructive">
            <AlertTriangle className="h-4 w-4" /> {error}
          </div>
        )}
        <Table>
          <TableHeader>
            <TableRow>
              <TableHead>Tasker</TableHead>
              <TableHead>Amount</TableHead>
              <TableHead>Bank</TableHead>
              <TableHead>Status</TableHead>
              <TableHead>Requested</TableHead>
              <TableHead>Arrives</TableHead>
            </TableRow>
          </TableHeader>
          <TableBody>
            {loading ? (
              Array.from({ length: 5 }).map((_, i) => (
                <TableRow key={i}>
                  {Array.from({ length: 6 }).map((_, j) => (
                    <TableCell key={j}><Skeleton className="h-4 w-full" /></TableCell>
                  ))}
                </TableRow>
              ))
            ) : rows.length === 0 ? (
              <TableRow>
                <TableCell colSpan={6} className="py-10 text-center text-muted-foreground">No withdrawals found.</TableCell>
              </TableRow>
            ) : (
              rows.map((w) => (
                <TableRow key={w.id}>
                  <TableCell>
                    <div className="text-sm">{w.user.name || "—"}</div>
                    <div className="text-xs text-muted-foreground">{w.user.email}</div>
                  </TableCell>
                  <TableCell className="font-medium">{money(w.amount, w.currency)}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">{w.bank_last4 ? `•••• ${w.bank_last4}` : "Default"}</TableCell>
                  <TableCell>
                    <Badge variant={STATUS_VARIANT[w.status] ?? "outline"}>{w.status.replace("_", " ")}</Badge>
                    {w.failure_message && <div className="mt-1 max-w-[220px] text-xs text-destructive">{w.failure_message}</div>}
                  </TableCell>
                  <TableCell className="text-sm text-muted-foreground">{new Date(w.created_at).toLocaleString()}</TableCell>
                  <TableCell className="text-sm text-muted-foreground">
                    {w.arrival_date ? new Date(w.arrival_date).toLocaleDateString() : "—"}
                  </TableCell>
                </TableRow>
              ))
            )}
          </TableBody>
        </Table>
        {!loading && (
          <DataPagination total={meta.total} page={page} pageSize={pageSize} onPageChange={setPage} onPageSizeChange={setPageSize} />
        )}
      </CardContent>
    </Card>
  );
}

export default function PayoutsPage() {
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3">
        <Banknote className="h-6 w-6" />
        <div>
          <h1 className="text-2xl font-bold tracking-tight">Payouts</h1>
          <p className="text-muted-foreground">Release escrowed payments to taskers and track bank withdrawals.</p>
        </div>
      </div>

      <Tabs defaultValue="escrows">
        <TabsList>
          <TabsTrigger value="escrows">Escrow payments</TabsTrigger>
          <TabsTrigger value="withdrawals">Withdrawals</TabsTrigger>
        </TabsList>
        <TabsContent value="escrows"><EscrowsTab /></TabsContent>
        <TabsContent value="withdrawals"><WithdrawalsTab /></TabsContent>
      </Tabs>
    </div>
  );
}
