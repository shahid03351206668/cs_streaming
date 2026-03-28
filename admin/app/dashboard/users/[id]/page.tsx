"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft, ArrowDownLeft, ArrowUpRight, BadgePercent,
  Gift, Banknote, ReceiptText, TrendingUp, TrendingDown,
  Pencil, KeyRound, Loader2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { DataPagination } from "@/components/ui/data-pagination";
import { useToast } from "@/components/ui/use-toast";
import api, { adminUpdateUser, adminChangeUserPassword } from "@/lib/api";

// ─── Types ────────────────────────────────────────────────────────────────────

interface UserProfile {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
  phone_number: string;
  profile_photo: string;
  email_verified: boolean;
  phone_verified: boolean;
  identity_verified: boolean;
  disabled: boolean;
  wallet_balance: number;
  referral_reward_balance: number;
  created_at: string;
}

interface WalletSummary {
  total_paid: number;
  total_earned: number;
  total_discounts: number;
  total_referral_discounts: number;
  total_app_fees_paid: number;
  total_referral_rewards: number;
  total_transactions: number;
  paid_count: number;
  received_count: number;
}

interface TxRow {
  id: string;
  transaction_date: string;
  from_user_id: string;
  to_user_id: string;
  reference_type: string;
  reference_id: string;
  amount: number;
  net_amount: number;
  discount_amount: number;
  referral_discount_amount: number;
  app_fee_amount: number;
  client_commission_amount: number;
  freelancer_commission_amount: number;
  referral_reward_amount: number;
  payment_method: string;
  currency: string;
  status: string;
  direction: "paid" | "received";
}

interface EditForm {
  first_name: string;
  last_name: string;
  email: string;
  phone_number: string;
  disabled: boolean;
  email_verified: boolean;
  phone_verified: boolean;
  identity_verified: boolean;
}

type Tab = "all" | "paid" | "received";

// ─── Helpers ──────────────────────────────────────────────────────────────────

const fmt = (pence: number) => `£${((pence ?? 0) / 100).toFixed(2)}`;

function statusVariant(s: string): "default" | "secondary" | "destructive" {
  if (["success", "completed"].includes(s?.toLowerCase())) return "default";
  if (["failed", "cancelled"].includes(s?.toLowerCase())) return "destructive";
  return "secondary";
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function UserDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const { toast } = useToast();

  const [loading, setLoading] = useState(true);
  const [user, setUser] = useState<UserProfile | null>(null);
  const [summary, setSummary] = useState<WalletSummary | null>(null);
  const [transactions, setTransactions] = useState<TxRow[]>([]);
  const [tab, setTab] = useState<Tab>("all");
  const [page, setPage] = useState(1);
  const [pageSize, setPageSize] = useState(10);

  // Edit profile
  const [editOpen, setEditOpen] = useState(false);
  const [editForm, setEditForm] = useState<EditForm | null>(null);
  const [saving, setSaving] = useState(false);

  // Change password
  const [pwOpen, setPwOpen] = useState(false);
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [changingPw, setChangingPw] = useState(false);

  const fetchData = () => {
    setLoading(true);
    Promise.all([
      api.get(`/api/v1/admin/users/${id}`),
      api.get(`/api/v1/admin/users/${id}/wallet`),
    ])
      .then(([profileRes, walletRes]) => {
        const profile = profileRes.data?.data ?? profileRes.data;
        setUser(profile);
        setSummary(walletRes.data.summary);
        setTransactions(walletRes.data.transactions ?? []);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchData(); }, [id]);
  useEffect(() => { setPage(1); }, [tab]);

  const openEdit = () => {
    if (!user) return;
    setEditForm({
      first_name: user.first_name,
      last_name: user.last_name,
      email: user.email,
      phone_number: user.phone_number || "",
      disabled: user.disabled,
      email_verified: user.email_verified,
      phone_verified: user.phone_verified,
      identity_verified: user.identity_verified,
    });
    setEditOpen(true);
  };

  const handleSaveProfile = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editForm) return;
    setSaving(true);
    try {
      await adminUpdateUser(id, editForm);
      toast({ title: "Success", description: "User updated successfully." });
      setEditOpen(false);
      fetchData();
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to update user." });
    } finally {
      setSaving(false);
    }
  };

  const handleChangePassword = async (e: React.FormEvent) => {
    e.preventDefault();
    if (newPassword !== confirmPassword) {
      toast({ variant: "destructive", title: "Error", description: "Passwords do not match." });
      return;
    }
    setChangingPw(true);
    try {
      await adminChangeUserPassword(id, newPassword);
      toast({ title: "Success", description: "Password changed successfully." });
      setPwOpen(false);
      setNewPassword("");
      setConfirmPassword("");
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to change password." });
    } finally {
      setChangingPw(false);
    }
  };

  const filtered = tab === "all" ? transactions : transactions.filter((t) => t.direction === tab);
  const paginated = filtered.slice((page - 1) * pageSize, page * pageSize);

  const statCards = summary
    ? [
        { label: "Total Paid", value: fmt(summary.total_paid), icon: ArrowUpRight, color: "text-red-500", bg: "bg-red-50 dark:bg-red-950" },
        { label: "Total Earned", value: fmt(summary.total_earned), icon: ArrowDownLeft, color: "text-green-500", bg: "bg-green-50 dark:bg-green-950" },
        { label: "Discounts Received", value: fmt(summary.total_discounts + summary.total_referral_discounts), icon: BadgePercent, color: "text-blue-500", bg: "bg-blue-50 dark:bg-blue-950" },
        { label: "Referral Rewards", value: fmt(summary.total_referral_rewards), icon: Gift, color: "text-purple-500", bg: "bg-purple-50 dark:bg-purple-950" },
        { label: "App Fees Paid", value: fmt(summary.total_app_fees_paid), icon: ReceiptText, color: "text-orange-500", bg: "bg-orange-50 dark:bg-orange-950" },
        { label: "Reward Balance", value: fmt(user?.referral_reward_balance ?? 0), icon: Banknote, color: "text-teal-500", bg: "bg-teal-50 dark:bg-teal-950" },
      ]
    : [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => router.back()}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <h1 className="text-2xl font-bold tracking-tight">
            {loading ? <Skeleton className="h-7 w-40 inline-block" /> : `${user?.first_name} ${user?.last_name}`}
          </h1>
          <p className="text-muted-foreground text-sm">
            {loading ? <Skeleton className="h-4 w-56 mt-1 inline-block" /> : user?.email}
          </p>
        </div>
        {!loading && user && (
          <div className="flex items-center gap-2">
            <Badge variant={user.disabled ? "destructive" : "default"}>
              {user.disabled ? "Disabled" : "Active"}
            </Badge>
            {user.email_verified && <Badge variant="secondary">Email Verified</Badge>}
            {user.identity_verified && <Badge variant="secondary">ID Verified</Badge>}
            <Button variant="outline" size="sm" onClick={openEdit} className="gap-2">
              <Pencil className="h-4 w-4" />
              Edit User
            </Button>
            <Button variant="outline" size="sm" onClick={() => setPwOpen(true)} className="gap-2">
              <KeyRound className="h-4 w-4" />
              Change Password
            </Button>
          </div>
        )}
      </div>

      {/* Profile Info Card */}
      {!loading && user && (
        <Card>
          <CardHeader className="pb-2">
            <CardTitle className="text-base">Profile</CardTitle>
          </CardHeader>
          <CardContent>
            <div className="flex items-center gap-4 mb-4">
              {user.profile_photo ? (
                <img src={user.profile_photo} alt="" className="h-16 w-16 rounded-full object-cover border" />
              ) : (
                <div className="h-16 w-16 rounded-full bg-muted flex items-center justify-center text-xl font-bold text-muted-foreground">
                  {user.first_name?.[0]}{user.last_name?.[0]}
                </div>
              )}
              <div>
                <p className="font-semibold text-lg">{user.first_name} {user.last_name}</p>
                <p className="text-sm text-muted-foreground">{user.email}</p>
                <p className="text-sm text-muted-foreground">{user.phone_number || "No phone"}</p>
              </div>
            </div>
            <Separator className="mb-4" />
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 text-sm">
              {[
                { label: "User ID", value: <span className="font-mono text-xs">{user.id}</span> },
                { label: "Joined", value: new Date(user.created_at).toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" }) },
                { label: "Wallet Balance", value: <span className="font-semibold text-green-600">{fmt(user.wallet_balance)}</span> },
                { label: "Referral Balance", value: fmt(user.referral_reward_balance) },
                { label: "Account Status", value: <Badge variant={user.disabled ? "destructive" : "default"}>{user.disabled ? "Disabled" : "Active"}</Badge> },
                { label: "Email Verified", value: <Badge variant={user.email_verified ? "default" : "secondary"}>{user.email_verified ? "Yes" : "No"}</Badge> },
                { label: "Phone Verified", value: <Badge variant={user.phone_verified ? "default" : "secondary"}>{user.phone_verified ? "Yes" : "No"}</Badge> },
                { label: "Identity Verified", value: <Badge variant={user.identity_verified ? "default" : "secondary"}>{user.identity_verified ? "Yes" : "No"}</Badge> },
              ].map(({ label, value }) => (
                <div key={label}>
                  <p className="text-muted-foreground text-xs mb-0.5">{label}</p>
                  <p>{value}</p>
                </div>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Summary Cards */}
      <div className="grid grid-cols-2 gap-4 sm:grid-cols-3 lg:grid-cols-6">
        {loading
          ? Array.from({ length: 6 }).map((_, i) => (
              <Card key={i}><CardContent className="pt-4"><Skeleton className="h-10 w-full" /></CardContent></Card>
            ))
          : statCards.map(({ label, value, icon: Icon, color, bg }) => (
              <Card key={label} className="overflow-hidden">
                <CardHeader className="flex flex-row items-center justify-between pb-1 pt-4 px-4">
                  <CardTitle className="text-xs font-medium text-muted-foreground leading-tight">{label}</CardTitle>
                  <div className={`rounded-full p-1.5 ${bg}`}>
                    <Icon className={`h-3.5 w-3.5 ${color}`} />
                  </div>
                </CardHeader>
                <CardContent className="px-4 pb-4">
                  <p className="text-lg font-bold">{value}</p>
                </CardContent>
              </Card>
            ))
        }
      </div>

      {/* Transaction breakdown bar */}
      {!loading && summary && (
        <Card>
          <CardContent className="pt-4 pb-4">
            <div className="flex flex-wrap items-center gap-6 text-sm">
              <div className="flex items-center gap-2">
                <TrendingDown className="h-4 w-4 text-red-500" />
                <span className="text-muted-foreground">Payments made:</span>
                <span className="font-semibold">{summary.paid_count}</span>
              </div>
              <div className="flex items-center gap-2">
                <TrendingUp className="h-4 w-4 text-green-500" />
                <span className="text-muted-foreground">Payments received:</span>
                <span className="font-semibold">{summary.received_count}</span>
              </div>
              <div className="flex items-center gap-2">
                <BadgePercent className="h-4 w-4 text-blue-500" />
                <span className="text-muted-foreground">Promotional discount:</span>
                <span className="font-semibold">{fmt(summary.total_discounts)}</span>
              </div>
              <div className="flex items-center gap-2">
                <Gift className="h-4 w-4 text-purple-500" />
                <span className="text-muted-foreground">Referral discount:</span>
                <span className="font-semibold">{fmt(summary.total_referral_discounts)}</span>
              </div>
            </div>
          </CardContent>
        </Card>
      )}

      {/* Transactions Table */}
      <Card>
        <CardHeader className="pb-0">
          <div className="flex items-center justify-between">
            <CardTitle className="text-base">Transaction History</CardTitle>
            <span className="text-sm text-muted-foreground">{filtered.length} transactions</span>
          </div>
          <div className="flex gap-1 border-b mt-3">
            {(["all", "paid", "received"] as Tab[]).map((t) => (
              <button
                key={t}
                onClick={() => setTab(t)}
                className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors capitalize ${
                  tab === t
                    ? "border-primary text-primary"
                    : "border-transparent text-muted-foreground hover:text-foreground"
                }`}
              >
                {t}
                <span className="ml-2 rounded-full bg-muted px-2 py-0.5 text-xs">
                  {t === "all" ? transactions.length
                    : transactions.filter((x) => x.direction === t).length}
                </span>
              </button>
            ))}
          </div>
        </CardHeader>
        <CardContent className="p-0">
          {loading ? (
            <div className="p-6 space-y-3">
              {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}
            </div>
          ) : (
            <>
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Date</TableHead>
                    <TableHead>Direction</TableHead>
                    <TableHead>Type</TableHead>
                    <TableHead className="text-right">Amount</TableHead>
                    <TableHead className="text-right">Net</TableHead>
                    <TableHead className="text-right">Discount</TableHead>
                    <TableHead className="text-right">Referral Disc.</TableHead>
                    <TableHead className="text-right">App Fee</TableHead>
                    <TableHead>Status</TableHead>
                    <TableHead>Method</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {paginated.length === 0 ? (
                    <TableRow>
                      <TableCell colSpan={10} className="text-center text-muted-foreground py-10">
                        No transactions found.
                      </TableCell>
                    </TableRow>
                  ) : (
                    paginated.map((tx) => (
                      <TableRow key={tx.id}>
                        <TableCell className="text-sm text-muted-foreground whitespace-nowrap">
                          {new Date(tx.transaction_date).toLocaleDateString("en-GB", {
                            day: "2-digit", month: "short", year: "numeric",
                          })}
                        </TableCell>
                        <TableCell>
                          {tx.direction === "paid" ? (
                            <div className="flex items-center gap-1 text-red-500 font-medium text-sm">
                              <ArrowUpRight className="h-3.5 w-3.5" /> Paid
                            </div>
                          ) : (
                            <div className="flex items-center gap-1 text-green-500 font-medium text-sm">
                              <ArrowDownLeft className="h-3.5 w-3.5" /> Received
                            </div>
                          )}
                        </TableCell>
                        <TableCell className="text-sm capitalize text-muted-foreground">
                          {tx.reference_type?.replace(/_/g, " ") || "—"}
                        </TableCell>
                        <TableCell className="text-right font-semibold">{fmt(tx.amount)}</TableCell>
                        <TableCell className="text-right text-green-600 font-medium">{fmt(tx.net_amount)}</TableCell>
                        <TableCell className="text-right text-blue-600">
                          {tx.discount_amount > 0 ? fmt(tx.discount_amount) : "—"}
                        </TableCell>
                        <TableCell className="text-right text-purple-600">
                          {tx.referral_discount_amount > 0 ? fmt(tx.referral_discount_amount) : "—"}
                        </TableCell>
                        <TableCell className="text-right text-orange-500">
                          {tx.app_fee_amount > 0 ? fmt(tx.app_fee_amount) : "—"}
                        </TableCell>
                        <TableCell>
                          <Badge variant={statusVariant(tx.status)}>
                            {tx.status ? tx.status.charAt(0).toUpperCase() + tx.status.slice(1) : "—"}
                          </Badge>
                        </TableCell>
                        <TableCell className="text-sm capitalize text-muted-foreground">
                          {tx.payment_method?.replace(/_/g, " ") || "—"}
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

      {/* ─── Edit User Dialog ─────────────────────────────────────────────── */}
      {editForm && (
        <Dialog open={editOpen} onOpenChange={(open) => !open && setEditOpen(false)}>
          <DialogContent className="max-w-lg max-h-[90vh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>Edit User</DialogTitle>
              <DialogDescription>Update profile details and account flags.</DialogDescription>
            </DialogHeader>

            <form onSubmit={handleSaveProfile} className="space-y-4">
              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="edit-fname">First Name *</Label>
                  <Input
                    id="edit-fname"
                    value={editForm.first_name}
                    onChange={(e) => setEditForm({ ...editForm, first_name: e.target.value })}
                    required
                  />
                </div>
                <div className="space-y-2">
                  <Label htmlFor="edit-lname">Last Name *</Label>
                  <Input
                    id="edit-lname"
                    value={editForm.last_name}
                    onChange={(e) => setEditForm({ ...editForm, last_name: e.target.value })}
                    required
                  />
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="edit-email">Email *</Label>
                <Input
                  id="edit-email"
                  type="email"
                  value={editForm.email}
                  onChange={(e) => setEditForm({ ...editForm, email: e.target.value })}
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="edit-phone">Phone Number</Label>
                <Input
                  id="edit-phone"
                  value={editForm.phone_number}
                  onChange={(e) => setEditForm({ ...editForm, phone_number: e.target.value })}
                />
              </div>

              <Separator />

              <div className="space-y-3">
                <p className="text-sm font-medium">Account Flags</p>
                {(
                  [
                    { key: "disabled", label: "Account Disabled", desc: "Prevent this user from logging in", danger: true },
                    { key: "email_verified", label: "Email Verified", desc: "Mark email as verified", danger: false },
                    { key: "phone_verified", label: "Phone Verified", desc: "Mark phone as verified", danger: false },
                    { key: "identity_verified", label: "Identity Verified", desc: "Mark identity as verified", danger: false },
                  ] as { key: keyof EditForm; label: string; desc: string; danger: boolean }[]
                ).map(({ key, label, desc, danger }) => (
                  <div key={key} className="flex items-center justify-between rounded-lg border p-3">
                    <div>
                      <p className={`text-sm font-medium ${danger && editForm[key] ? "text-red-600" : ""}`}>{label}</p>
                      <p className="text-xs text-muted-foreground">{desc}</p>
                    </div>
                    <Switch
                      checked={editForm[key] as boolean}
                      onCheckedChange={(v) => setEditForm({ ...editForm, [key]: v })}
                    />
                  </div>
                ))}
              </div>

              <DialogFooter className="gap-2 pt-2">
                <Button type="button" variant="outline" onClick={() => setEditOpen(false)}>Cancel</Button>
                <Button type="submit" disabled={saving}>
                  {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Save Changes
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      )}

      {/* ─── Change Password Dialog ───────────────────────────────────────── */}
      <Dialog open={pwOpen} onOpenChange={(open) => { if (!open) { setPwOpen(false); setNewPassword(""); setConfirmPassword(""); } }}>
        <DialogContent className="max-w-sm">
          <DialogHeader>
            <DialogTitle>Change Password</DialogTitle>
            <DialogDescription>
              Set a new password for{" "}
              <span className="font-semibold">{user?.first_name} {user?.last_name}</span>.
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handleChangePassword} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="new-pw">New Password *</Label>
              <Input
                id="new-pw"
                type="password"
                placeholder="Min. 6 characters"
                value={newPassword}
                onChange={(e) => setNewPassword(e.target.value)}
                required
                minLength={6}
                autoComplete="new-password"
              />
            </div>
            <div className="space-y-2">
              <Label htmlFor="confirm-pw">Confirm Password *</Label>
              <Input
                id="confirm-pw"
                type="password"
                placeholder="Re-enter password"
                value={confirmPassword}
                onChange={(e) => setConfirmPassword(e.target.value)}
                required
                minLength={6}
                autoComplete="new-password"
              />
              {confirmPassword && newPassword !== confirmPassword && (
                <p className="text-xs text-red-500">Passwords do not match.</p>
              )}
            </div>

            <DialogFooter className="gap-2">
              <Button type="button" variant="outline" onClick={() => setPwOpen(false)}>Cancel</Button>
              <Button
                type="submit"
                disabled={changingPw || newPassword !== confirmPassword || newPassword.length < 6}
              >
                {changingPw && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Change Password
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
