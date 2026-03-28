"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft, Briefcase, MapPin, Image as ImageIcon, FileText,
  Users, ClipboardList, CreditCard, CheckCircle2, Clock,
  AlertCircle, Pencil, Loader2,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import { Switch } from "@/components/ui/switch";
import { Card, CardContent, CardHeader } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import {
  Select, SelectContent, SelectItem, SelectTrigger, SelectValue,
} from "@/components/ui/select";
import { useToast } from "@/components/ui/use-toast";
import api, { adminUpdateJob, getCategories } from "@/lib/api";

// ─── Types ────────────────────────────────────────────────────────────────────

interface JobUser {
  id: string;
  first_name: string;
  last_name: string;
  email: string;
  profile_photo: string;
  identity_verified: boolean;
}

interface JobMedia {
  id: string;
  url: string;
  thumbnail: string;
  media_type: string;
  file_name: string;
  file_size: number;
}

interface JobLocation {
  job_post_id: string;
  latitude: number;
  longitude: number;
  postal_code: string;
  street: string;
  city: string;
  state: string;
  country: string;
}

interface ProposalAttachment {
  id: string;
  url: string;
  file_name: string;
  file_size: number;
  file_type: string;
}

interface Proposal {
  id: string;
  freelancer_id: string;
  freelancer: JobUser;
  cover_letter: string;
  bid_amount: number;
  duration: number;
  status: string;
  created_at: string;
  attachments: ProposalAttachment[];
}

interface Contract {
  id: string;
  title: string;
  description: string;
  total_amount: number;
  start_date: string;
  end_date: string;
  status: string;
  terms: string;
  client_completed: boolean;
  freelancer_completed: boolean;
  completed_at: string | null;
  escrow_status: string;
  escrow_amount: number;
  escrow_payment_intent_id: string;
  client: JobUser;
  freelancer: JobUser;
}

interface PaymentRow {
  id: string;
  transaction_date: string;
  from_user_id: string;
  from_user_name: string;
  to_user_id: string;
  to_user_name: string;
  amount: number;
  net_amount: number;
  app_fee_amount: number;
  discount_amount: number;
  currency: string;
  status: string;
  payment_method: string;
}

interface JobDetail {
  job: {
    id: string;
    title: string;
    description: string;
    budget: number;
    open_budget: boolean;
    status: string;
    address: string;
    created_by_id: string;
    created_by: JobUser;
    category: { id: string; name: string };
    job_media: JobMedia[];
    created_at: string;
    updated_at: string;
  };
  location: JobLocation;
  proposals: Proposal[];
  contract?: Contract;
  payments: PaymentRow[];
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

const fmt = (pence: number, currency = "gbp") =>
  new Intl.NumberFormat("en-GB", {
    style: "currency",
    currency: currency.toUpperCase(),
    minimumFractionDigits: 2,
  }).format(pence / 100);

const fmtBudget = (v: number) =>
  new Intl.NumberFormat("en-GB", { style: "currency", currency: "GBP" }).format(v);

const fmtDate = (s: string) =>
  s ? new Date(s).toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" }) : "—";

const fmtDateTime = (s: string) =>
  s ? new Date(s).toLocaleString("en-GB", { day: "2-digit", month: "short", year: "numeric", hour: "2-digit", minute: "2-digit" }) : "—";

const JOB_STATUS_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  open: "default",
  in_progress: "secondary",
  completed: "outline",
  cancelled: "destructive",
};

const PROPOSAL_STATUS_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  pending: "secondary",
  accepted: "default",
  rejected: "destructive",
  shortlisted: "outline",
  withdrawn: "outline",
};

const ESCROW_COLORS: Record<string, string> = {
  pending: "text-yellow-500",
  funded: "text-blue-500",
  released: "text-green-500",
  refunded: "text-orange-500",
  failed: "text-red-500",
  release_pending: "text-purple-500",
};

const CONTRACT_STATUS_VARIANT: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  pending: "secondary",
  active: "default",
  completed: "outline",
  cancelled: "destructive",
  disputed: "destructive",
};

// ─── Sub-components ───────────────────────────────────────────────────────────

function SectionHeader({ icon, title }: { icon: React.ReactNode; title: string }) {
  return (
    <div className="flex items-center gap-2 mb-4">
      <div className="text-muted-foreground">{icon}</div>
      <h2 className="text-lg font-semibold">{title}</h2>
    </div>
  );
}

function UserChip({ user }: { user: JobUser }) {
  return (
    <div className="flex items-center gap-2">
      {user.profile_photo ? (
        <img src={user.profile_photo} alt="" className="h-7 w-7 rounded-full object-cover" />
      ) : (
        <div className="h-7 w-7 rounded-full bg-muted flex items-center justify-center text-xs font-semibold">
          {user.first_name?.[0]}{user.last_name?.[0]}
        </div>
      )}
      <div>
        <p className="text-sm font-medium leading-none">
          {user.first_name} {user.last_name}
          {user.identity_verified && (
            <span className="ml-1 text-blue-500 text-xs">✓</span>
          )}
        </p>
        <p className="text-xs text-muted-foreground">{user.email}</p>
      </div>
    </div>
  );
}

// ─── Page ─────────────────────────────────────────────────────────────────────

interface Category {
  id: string;
  name: string;
}

interface EditForm {
  title: string;
  description: string;
  budget: string;
  open_budget: boolean;
  address: string;
  status: string;
  category_id: string;
}

const JOB_STATUSES = ["open", "in_progress", "completed", "cancelled", "closed", "on_hold", "draft"];

export default function JobDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const [data, setData] = useState<JobDetail | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState("");
  const [editOpen, setEditOpen] = useState(false);
  const [editForm, setEditForm] = useState<EditForm | null>(null);
  const [saving, setSaving] = useState(false);
  const [categories, setCategories] = useState<Category[]>([]);
  const { toast } = useToast();

  const fetchJob = () =>
    api.get(`/api/v1/admin/jobs/${id}`)
      .then((res) => setData(res.data?.data ?? null))
      .catch(() => setError("Failed to load job details."))
      .finally(() => setLoading(false));

  useEffect(() => {
    fetchJob();
    getCategories().then((res) => {
      const list = Array.isArray(res.data) ? res.data : (res.data?.data ?? []);
      setCategories(list);
    });
  }, [id]);

  const openEdit = () => {
    if (!data) return;
    const { job } = data;
    setEditForm({
      title: job.title,
      description: job.description,
      budget: String(job.budget),
      open_budget: job.open_budget,
      address: job.address || "",
      status: job.status,
      category_id: job.category?.id || "",
    });
    setEditOpen(true);
  };

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!editForm) return;
    setSaving(true);
    try {
      await adminUpdateJob(id, {
        title: editForm.title,
        description: editForm.description,
        budget: parseFloat(editForm.budget) || 0,
        open_budget: editForm.open_budget,
        address: editForm.address,
        status: editForm.status,
        category_id: editForm.category_id || undefined,
      });
      toast({ title: "Success", description: "Job updated successfully." });
      setEditOpen(false);
      setLoading(true);
      fetchJob();
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to update job." });
    } finally {
      setSaving(false);
    }
  };

  if (loading) return <LoadingSkeleton />;
  if (error || !data) {
    return (
      <div className="flex flex-col items-center justify-center py-24 gap-4">
        <AlertCircle className="h-10 w-10 text-destructive" />
        <p className="text-muted-foreground">{error || "Job not found."}</p>
        <Button variant="outline" onClick={() => router.back()}>Go back</Button>
      </div>
    );
  }

  const { job, location, proposals, contract, payments } = data;
  const acceptedProposal = proposals.find((p) => p.status === "accepted");

  return (
    <div className="space-y-6 pb-12">
      {/* Header */}
      <div className="flex items-start gap-4">
        <Button variant="ghost" size="icon" onClick={() => router.back()} className="mt-0.5 shrink-0">
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1 min-w-0">
          <div className="flex flex-wrap items-center gap-2">
            <h1 className="text-2xl font-bold tracking-tight truncate">{job.title}</h1>
            <Badge variant={JOB_STATUS_VARIANT[job.status] ?? "outline"}>
              {job.status?.replace(/_/g, " ")}
            </Badge>
          </div>
          <p className="text-muted-foreground text-sm mt-1">
            Job ID: <span className="font-mono">{job.id}</span>
          </p>
        </div>
        <div className="flex items-center gap-3 shrink-0">
          <div className="text-right">
            {job.open_budget ? (
              <span className="text-muted-foreground italic text-sm">Open Budget</span>
            ) : (
              <span className="text-2xl font-bold">{fmtBudget(job.budget)}</span>
            )}
          </div>
          <Button variant="outline" size="sm" onClick={openEdit} className="gap-2">
            <Pencil className="h-4 w-4" />
            Edit Job
          </Button>
        </div>
      </div>

      {/* Overview + Posted By */}
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <Card className="lg:col-span-2">
          <CardHeader className="pb-2">
            <SectionHeader icon={<Briefcase className="h-4 w-4" />} title="Job Overview" />
          </CardHeader>
          <CardContent className="space-y-4">
            <div>
              <p className="text-sm font-medium text-muted-foreground mb-1">Description</p>
              <p className="text-sm whitespace-pre-wrap">{job.description}</p>
            </div>
            <Separator />
            <div className="grid grid-cols-2 gap-4 text-sm">
              <div>
                <p className="text-muted-foreground">Category</p>
                <p className="font-medium">{job.category?.name || "—"}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Address</p>
                <p className="font-medium">{job.address || "—"}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Posted</p>
                <p className="font-medium">{fmtDateTime(job.created_at)}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Last Updated</p>
                <p className="font-medium">{fmtDateTime(job.updated_at)}</p>
              </div>
            </div>
          </CardContent>
        </Card>

        <Card>
          <CardHeader className="pb-2">
            <SectionHeader icon={<Users className="h-4 w-4" />} title="Posted By" />
          </CardHeader>
          <CardContent className="space-y-4">
            {job.created_by ? (
              <>
                <UserChip user={job.created_by} />
                <Separator />
                <div className="text-sm space-y-1">
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Proposals</span>
                    <span className="font-medium">{proposals.length}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Accepted</span>
                    <span className="font-medium">{acceptedProposal ? "Yes" : "No"}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Contract</span>
                    <span className="font-medium">{contract ? "Yes" : "No"}</span>
                  </div>
                  <div className="flex justify-between">
                    <span className="text-muted-foreground">Payments</span>
                    <span className="font-medium">{payments.length}</span>
                  </div>
                </div>
              </>
            ) : (
              <p className="text-sm text-muted-foreground">No user data</p>
            )}
          </CardContent>
        </Card>
      </div>

      {/* Location */}
      {(location?.city || location?.latitude) && (
        <Card>
          <CardHeader className="pb-2">
            <SectionHeader icon={<MapPin className="h-4 w-4" />} title="Location" />
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 text-sm">
              {[
                { label: "Street", value: location.street },
                { label: "City", value: location.city },
                { label: "State", value: location.state },
                { label: "Country", value: location.country },
                { label: "Postal Code", value: location.postal_code },
                { label: "Latitude", value: location.latitude?.toString() },
                { label: "Longitude", value: location.longitude?.toString() },
              ].map(({ label, value }) => (
                <div key={label}>
                  <p className="text-muted-foreground">{label}</p>
                  <p className="font-medium">{value || "—"}</p>
                </div>
              ))}
            </div>
            {location.latitude && location.longitude && (
              <div className="mt-4">
                <a
                  href={`https://www.google.com/maps?q=${location.latitude},${location.longitude}`}
                  target="_blank"
                  rel="noopener noreferrer"
                  className="text-sm text-blue-500 hover:underline flex items-center gap-1"
                >
                  <MapPin className="h-3 w-3" />
                  View on Google Maps
                </a>
              </div>
            )}
          </CardContent>
        </Card>
      )}

      {/* Media */}
      {job.job_media && job.job_media.length > 0 && (
        <Card>
          <CardHeader className="pb-2">
            <SectionHeader icon={<ImageIcon className="h-4 w-4" />} title={`Media (${job.job_media.length})`} />
          </CardHeader>
          <CardContent>
            <div className="grid grid-cols-2 sm:grid-cols-3 md:grid-cols-4 lg:grid-cols-5 gap-3">
              {job.job_media.map((m) => (
                <a key={m.id} href={m.url} target="_blank" rel="noopener noreferrer" className="group">
                  <div className="aspect-square rounded-lg overflow-hidden bg-muted border relative">
                    {m.media_type?.startsWith("video") ? (
                      <div className="h-full w-full flex items-center justify-center">
                        <div className="text-muted-foreground text-xs text-center px-2">
                          <FileText className="h-6 w-6 mx-auto mb-1" />
                          {m.file_name || "Video"}
                        </div>
                      </div>
                    ) : (
                      <img
                        src={m.thumbnail || m.url}
                        alt={m.file_name}
                        className="h-full w-full object-cover group-hover:scale-105 transition-transform"
                      />
                    )}
                  </div>
                  <p className="text-xs text-muted-foreground mt-1 truncate">{m.file_name}</p>
                  <p className="text-xs text-muted-foreground">
                    {m.file_size ? `${(m.file_size / 1024).toFixed(0)} KB` : ""}
                  </p>
                </a>
              ))}
            </div>
          </CardContent>
        </Card>
      )}

      {/* Proposals */}
      <Card>
        <CardHeader className="pb-2">
          <SectionHeader
            icon={<Users className="h-4 w-4" />}
            title={`Proposals (${proposals.length})`}
          />
        </CardHeader>
        <CardContent className="p-0">
          {proposals.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">No proposals yet.</p>
          ) : (
            <Table>
              <TableHeader>
                <TableRow>
                  <TableHead>Freelancer</TableHead>
                  <TableHead>Bid Amount</TableHead>
                  <TableHead>Duration</TableHead>
                  <TableHead>Status</TableHead>
                  <TableHead>Submitted</TableHead>
                  <TableHead>Attachments</TableHead>
                </TableRow>
              </TableHeader>
              <TableBody>
                {proposals.map((p) => (
                  <TableRow key={p.id} className={p.status === "accepted" ? "bg-green-50 dark:bg-green-950/20" : ""}>
                    <TableCell>
                      {p.freelancer ? (
                        <UserChip user={p.freelancer} />
                      ) : (
                        <span className="text-muted-foreground text-xs font-mono">{p.freelancer_id}</span>
                      )}
                    </TableCell>
                    <TableCell className="font-semibold">
                      £{Number(p.bid_amount).toFixed(2)}
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {p.duration} day{p.duration !== 1 ? "s" : ""}
                    </TableCell>
                    <TableCell>
                      <Badge variant={PROPOSAL_STATUS_VARIANT[p.status] ?? "outline"}>
                        {p.status}
                      </Badge>
                    </TableCell>
                    <TableCell className="text-muted-foreground text-sm">
                      {fmtDate(p.created_at)}
                    </TableCell>
                    <TableCell className="text-sm text-muted-foreground">
                      {p.attachments?.length > 0 ? (
                        <div className="flex flex-col gap-0.5">
                          {p.attachments.map((a) => (
                            <a key={a.id} href={a.url} target="_blank" rel="noopener noreferrer"
                              className="text-blue-500 hover:underline truncate max-w-[120px]">
                              {a.file_name}
                            </a>
                          ))}
                        </div>
                      ) : "—"}
                    </TableCell>
                  </TableRow>
                ))}
              </TableBody>
            </Table>
          )}

          {/* Cover letters */}
          {proposals.length > 0 && (
            <div className="p-4 space-y-3 border-t">
              <p className="text-sm font-medium text-muted-foreground">Cover Letters</p>
              <div className="space-y-3">
                {proposals.map((p) => (
                  <div key={p.id} className="rounded-lg border p-3 text-sm">
                    <div className="flex items-center justify-between mb-1.5">
                      <span className="font-medium">
                        {p.freelancer?.first_name} {p.freelancer?.last_name}
                      </span>
                      <Badge variant={PROPOSAL_STATUS_VARIANT[p.status] ?? "outline"} className="text-xs">
                        {p.status}
                      </Badge>
                    </div>
                    <p className="text-muted-foreground whitespace-pre-wrap">{p.cover_letter}</p>
                  </div>
                ))}
              </div>
            </div>
          )}
        </CardContent>
      </Card>

      {/* Contract */}
      {contract ? (
        <Card>
          <CardHeader className="pb-2">
            <SectionHeader icon={<ClipboardList className="h-4 w-4" />} title="Contract" />
          </CardHeader>
          <CardContent className="space-y-4">
            <div className="flex flex-wrap items-center gap-3">
              <Badge variant={CONTRACT_STATUS_VARIANT[contract.status] ?? "outline"}>
                {contract.status?.replace(/_/g, " ")}
              </Badge>
              <span className={`text-sm font-medium flex items-center gap-1 ${ESCROW_COLORS[contract.escrow_status] ?? "text-muted-foreground"}`}>
                Escrow: {contract.escrow_status?.replace(/_/g, " ")}
              </span>
            </div>

            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4 text-sm">
              <div>
                <p className="text-muted-foreground">Title</p>
                <p className="font-medium">{contract.title}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Total Amount</p>
                <p className="font-semibold text-base">£{Number(contract.total_amount).toFixed(2)}</p>
              </div>
              <div>
                <p className="text-muted-foreground">Start Date</p>
                <p className="font-medium">{fmtDate(contract.start_date)}</p>
              </div>
              <div>
                <p className="text-muted-foreground">End Date</p>
                <p className="font-medium">{fmtDate(contract.end_date)}</p>
              </div>
              {contract.completed_at && (
                <div>
                  <p className="text-muted-foreground">Completed At</p>
                  <p className="font-medium">{fmtDateTime(contract.completed_at)}</p>
                </div>
              )}
              {contract.escrow_amount > 0 && (
                <div>
                  <p className="text-muted-foreground">Escrow Amount</p>
                  <p className="font-medium">{fmt(contract.escrow_amount)}</p>
                </div>
              )}
            </div>

            {/* Completion status */}
            <div className="flex gap-4">
              <div className={`flex items-center gap-1.5 text-sm ${contract.client_completed ? "text-green-600" : "text-muted-foreground"}`}>
                {contract.client_completed
                  ? <CheckCircle2 className="h-4 w-4" />
                  : <Clock className="h-4 w-4" />}
                Client {contract.client_completed ? "marked complete" : "pending"}
              </div>
              <div className={`flex items-center gap-1.5 text-sm ${contract.freelancer_completed ? "text-green-600" : "text-muted-foreground"}`}>
                {contract.freelancer_completed
                  ? <CheckCircle2 className="h-4 w-4" />
                  : <Clock className="h-4 w-4" />}
                Freelancer {contract.freelancer_completed ? "marked complete" : "pending"}
              </div>
            </div>

            <Separator />

            {/* Parties */}
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              <div>
                <p className="text-xs text-muted-foreground mb-2 uppercase tracking-wide">Client</p>
                {contract.client && <UserChip user={contract.client} />}
              </div>
              <div>
                <p className="text-xs text-muted-foreground mb-2 uppercase tracking-wide">Freelancer</p>
                {contract.freelancer && <UserChip user={contract.freelancer} />}
              </div>
            </div>

            {contract.description && (
              <>
                <Separator />
                <div>
                  <p className="text-sm font-medium text-muted-foreground mb-1">Description</p>
                  <p className="text-sm whitespace-pre-wrap">{contract.description}</p>
                </div>
              </>
            )}
            {contract.terms && (
              <div>
                <p className="text-sm font-medium text-muted-foreground mb-1">Terms</p>
                <p className="text-sm whitespace-pre-wrap">{contract.terms}</p>
              </div>
            )}
            {contract.escrow_payment_intent_id && (
              <div className="rounded-md bg-muted px-3 py-2 text-xs font-mono text-muted-foreground break-all">
                Stripe PI: {contract.escrow_payment_intent_id}
              </div>
            )}
          </CardContent>
        </Card>
      ) : (
        <Card>
          <CardHeader className="pb-2">
            <SectionHeader icon={<ClipboardList className="h-4 w-4" />} title="Contract" />
          </CardHeader>
          <CardContent>
            <p className="text-sm text-muted-foreground py-4 text-center">No contract created for this job yet.</p>
          </CardContent>
        </Card>
      )}

      {/* Payments */}
      <Card>
        <CardHeader className="pb-2">
          <SectionHeader
            icon={<CreditCard className="h-4 w-4" />}
            title={`Payments (${payments.length})`}
          />
        </CardHeader>
        <CardContent className="p-0">
          {payments.length === 0 ? (
            <p className="text-sm text-muted-foreground text-center py-8">No payment transactions yet.</p>
          ) : (
            <>
              {/* Summary row */}
              <div className="grid grid-cols-2 sm:grid-cols-4 gap-4 p-4 border-b">
                {[
                  {
                    label: "Total Volume",
                    value: fmt(payments.reduce((s, p) => s + p.amount, 0), payments[0]?.currency),
                  },
                  {
                    label: "Net Paid Out",
                    value: fmt(payments.reduce((s, p) => s + p.net_amount, 0), payments[0]?.currency),
                  },
                  {
                    label: "App Fees",
                    value: fmt(payments.reduce((s, p) => s + p.app_fee_amount, 0), payments[0]?.currency),
                  },
                  {
                    label: "Discounts",
                    value: fmt(payments.reduce((s, p) => s + p.discount_amount, 0), payments[0]?.currency),
                  },
                ].map(({ label, value }) => (
                  <div key={label}>
                    <p className="text-xs text-muted-foreground">{label}</p>
                    <p className="text-sm font-semibold">{value}</p>
                  </div>
                ))}
              </div>

              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead>Date</TableHead>
                    <TableHead>From</TableHead>
                    <TableHead>To</TableHead>
                    <TableHead>Amount</TableHead>
                    <TableHead>Net</TableHead>
                    <TableHead>App Fee</TableHead>
                    <TableHead>Discount</TableHead>
                    <TableHead>Method</TableHead>
                    <TableHead>Status</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {payments.map((p) => (
                    <TableRow key={p.id}>
                      <TableCell className="text-sm text-muted-foreground whitespace-nowrap">
                        {fmtDateTime(p.transaction_date)}
                      </TableCell>
                      <TableCell className="text-sm">{p.from_user_name || "—"}</TableCell>
                      <TableCell className="text-sm">{p.to_user_name || "—"}</TableCell>
                      <TableCell className="font-medium">{fmt(p.amount, p.currency)}</TableCell>
                      <TableCell className="text-green-600 font-medium">{fmt(p.net_amount, p.currency)}</TableCell>
                      <TableCell className="text-muted-foreground">{fmt(p.app_fee_amount, p.currency)}</TableCell>
                      <TableCell className="text-blue-500">{fmt(p.discount_amount, p.currency)}</TableCell>
                      <TableCell className="text-sm text-muted-foreground capitalize">{p.payment_method}</TableCell>
                      <TableCell>
                        <Badge variant={
                          p.status === "succeeded" ? "default"
                          : p.status === "failed" ? "destructive"
                          : "secondary"
                        }>
                          {p.status}
                        </Badge>
                      </TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            </>
          )}
        </CardContent>
      </Card>
      {/* Edit Job Dialog */}
      {editForm && (
        <Dialog open={editOpen} onOpenChange={(open) => !open && setEditOpen(false)}>
          <DialogContent className="max-w-xl max-h-[90vh] overflow-y-auto">
            <DialogHeader>
              <DialogTitle>Edit Job</DialogTitle>
              <DialogDescription>
                Update this job post on behalf of the user.
              </DialogDescription>
            </DialogHeader>

            <form onSubmit={handleSave} className="space-y-4">
              <div className="space-y-2">
                <Label htmlFor="edit-title">Title *</Label>
                <Input
                  id="edit-title"
                  value={editForm.title}
                  onChange={(e) => setEditForm({ ...editForm, title: e.target.value })}
                  required
                />
              </div>

              <div className="space-y-2">
                <Label htmlFor="edit-description">Description *</Label>
                <textarea
                  id="edit-description"
                  className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm min-h-[100px] resize-y focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-ring"
                  value={editForm.description}
                  onChange={(e) => setEditForm({ ...editForm, description: e.target.value })}
                  required
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label htmlFor="edit-budget">Budget (£)</Label>
                  <Input
                    id="edit-budget"
                    type="number"
                    min={0}
                    step="0.01"
                    value={editForm.budget}
                    onChange={(e) => setEditForm({ ...editForm, budget: e.target.value })}
                    disabled={editForm.open_budget}
                  />
                </div>
                <div className="flex items-end gap-2 pb-0.5">
                  <Switch
                    id="edit-open-budget"
                    checked={editForm.open_budget}
                    onCheckedChange={(v) => setEditForm({ ...editForm, open_budget: v })}
                  />
                  <Label htmlFor="edit-open-budget">Open Budget</Label>
                </div>
              </div>

              <div className="space-y-2">
                <Label htmlFor="edit-address">Address</Label>
                <Input
                  id="edit-address"
                  value={editForm.address}
                  onChange={(e) => setEditForm({ ...editForm, address: e.target.value })}
                />
              </div>

              <div className="grid grid-cols-2 gap-4">
                <div className="space-y-2">
                  <Label>Status</Label>
                  <Select
                    value={editForm.status}
                    onValueChange={(v) => setEditForm({ ...editForm, status: v })}
                  >
                    <SelectTrigger>
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent>
                      {JOB_STATUSES.map((s) => (
                        <SelectItem key={s} value={s}>
                          {s.replace(/_/g, " ")}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>

                <div className="space-y-2">
                  <Label>Category</Label>
                  <Select
                    value={editForm.category_id}
                    onValueChange={(v) => setEditForm({ ...editForm, category_id: v })}
                  >
                    <SelectTrigger>
                      <SelectValue placeholder="Select category" />
                    </SelectTrigger>
                    <SelectContent>
                      {categories.map((cat) => (
                        <SelectItem key={cat.id} value={cat.id}>
                          {cat.name}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </div>
              </div>

              <DialogFooter className="gap-2 pt-2">
                <Button type="button" variant="outline" onClick={() => setEditOpen(false)}>
                  Cancel
                </Button>
                <Button type="submit" disabled={saving}>
                  {saving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                  Save Changes
                </Button>
              </DialogFooter>
            </form>
          </DialogContent>
        </Dialog>
      )}
    </div>
  );
}

// ─── Loading Skeleton ─────────────────────────────────────────────────────────

function LoadingSkeleton() {
  return (
    <div className="space-y-6">
      <div className="flex items-center gap-4">
        <Skeleton className="h-9 w-9 rounded-md" />
        <div className="space-y-2 flex-1">
          <Skeleton className="h-7 w-64" />
          <Skeleton className="h-4 w-48" />
        </div>
      </div>
      <div className="grid grid-cols-1 lg:grid-cols-3 gap-4">
        <Skeleton className="h-48 lg:col-span-2" />
        <Skeleton className="h-48" />
      </div>
      <Skeleton className="h-32" />
      <Skeleton className="h-56" />
      <Skeleton className="h-56" />
    </div>
  );
}
