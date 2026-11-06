"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft, CheckCircle, Clock, User, FileText, Loader2,
  Paperclip, Download, Image, File as FileIcon, Wallet,
} from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { Separator } from "@/components/ui/separator";
import { Label } from "@/components/ui/label";
import {
  Dialog, DialogContent, DialogDescription, DialogFooter,
  DialogHeader, DialogTitle,
} from "@/components/ui/dialog";
import { useToast } from "@/components/ui/use-toast";
import {
  adminGetDispute, adminResolveDispute, adminRefundDispute,
  type Dispute, type DisputeStatus, type DisputeAttachment, type DisputePayment,
} from "@/lib/api";

// ─── Helpers ──────────────────────────────────────────────────────────────────

const STATUS_BADGE: Record<DisputeStatus, "default" | "secondary" | "destructive" | "outline"> = {
  open: "destructive",
  resolved: "default",
  closed: "secondary",
};

const CONTRACT_STATUS_BADGE: Record<string, "default" | "secondary" | "destructive" | "outline"> = {
  active: "default",
  disputed: "destructive",
  completed: "outline",
  cancelled: "secondary",
};

const ROLE_BADGE: Record<string, string> = {
  client: "bg-blue-100 text-blue-700 dark:bg-blue-900 dark:text-blue-300",
  freelancer: "bg-purple-100 text-purple-700 dark:bg-purple-900 dark:text-purple-300",
};

type ResolveChoice = "refund_all" | "refund_bid" | "refund_custom" | "pay_freelancer" | "remarks_only";

const PAYOUT_LABEL: Record<string, string> = {
  refunded_in_full: "Client refunded in full",
  refunded_excluding_fees: "Client refunded the job amount; fees kept as a penalty",
  refunded_custom_amount: "Client refunded a custom amount",
  already_refunded: "Client had already been refunded",
  released_to_freelancer: "Payment released to the freelancer",
  already_released: "Payment had already been released to the freelancer",
  none: "No money moved",
};

const money = (n: number, currency = "gbp") =>
  new Intl.NumberFormat("en-GB", { style: "currency", currency: currency.toUpperCase() }).format(n);

function fmtBytes(bytes: number): string {
  if (bytes === 0) return "0 B";
  const k = 1024;
  const sizes = ["B", "KB", "MB", "GB"];
  const i = Math.floor(Math.log(bytes) / Math.log(k));
  return `${parseFloat((bytes / Math.pow(k, i)).toFixed(1))} ${sizes[i]}`;
}

function isImage(mediaType: string, fileName: string): boolean {
  return (
    mediaType?.startsWith("image/") ||
    /\.(jpg|jpeg|png|gif|webp|svg)$/i.test(fileName)
  );
}

function InfoRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <p className="text-xs text-muted-foreground mb-0.5">{label}</p>
      <div className="text-sm font-medium">{value}</div>
    </div>
  );
}

// ─── Attachment card ──────────────────────────────────────────────────────────

function AttachmentItem({ file }: { file: DisputeAttachment }) {
  const img = isImage(file.media_type, file.file_name);

  return (
    <a
      href={file.url}
      target="_blank"
      rel="noopener noreferrer"
      className="group flex items-center gap-3 rounded-lg border bg-muted/40 p-3 hover:bg-muted transition-colors"
    >
      {img ? (
        <div className="h-10 w-10 rounded overflow-hidden border shrink-0 bg-white">
          {/* eslint-disable-next-line @next/next/no-img-element */}
          <img
            src={file.url}
            alt={file.file_name}
            className="h-full w-full object-cover"
            onError={(e) => {
              (e.target as HTMLImageElement).style.display = "none";
            }}
          />
        </div>
      ) : (
        <div className="h-10 w-10 rounded border bg-white flex items-center justify-center shrink-0">
          <FileIcon className="h-5 w-5 text-muted-foreground" />
        </div>
      )}

      <div className="flex-1 min-w-0">
        <p className="text-sm font-medium truncate">{file.file_name}</p>
        <p className="text-xs text-muted-foreground">
          {file.media_type || "unknown type"} · {fmtBytes(file.file_size)}
        </p>
      </div>

      <Download className="h-4 w-4 text-muted-foreground group-hover:text-foreground shrink-0 transition-colors" />
    </a>
  );
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function DisputeDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const { toast } = useToast();

  const [dispute, setDispute] = useState<Dispute | null>(null);
  const [payment, setPayment] = useState<DisputePayment | null>(null);
  const [loading, setLoading] = useState(true);

  const [resolveOpen, setResolveOpen] = useState(false);
  const [resolution, setResolution] = useState("");
  const [resolving, setResolving] = useState(false);
  const [choice, setChoice] = useState<ResolveChoice>("remarks_only");
  const [customAmount, setCustomAmount] = useState("");

  const fetchDispute = () => {
    setLoading(true);
    adminGetDispute(id)
      .then((res) => {
        setDispute(res.data.data);
        setPayment(res.data.payment ?? null);
      })
      .catch(console.error)
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchDispute(); }, [id]);

  const canRefund = !!payment?.can_refund;
  const canPayFreelancer = payment?.has_payment && payment.escrow_status === "held";
  const customValue = parseFloat(customAmount);
  const customValid =
    !isNaN(customValue) && customValue > 0 && customValue <= (payment?.max_refundable ?? 0);

  const openResolve = () => {
    setChoice(canRefund ? "refund_all" : "remarks_only");
    setCustomAmount("");
    setResolveOpen(true);
  };

  const closeResolve = () => {
    setResolveOpen(false);
    setResolution("");
    setCustomAmount("");
  };

  const handleResolve = async (e: React.FormEvent) => {
    e.preventDefault();
    const remarks = resolution.trim();
    if (remarks.length < 5) return;
    if (choice === "refund_custom" && !customValid) return;
    setResolving(true);
    try {
      if (choice === "refund_all") {
        await adminResolveDispute(id, { resolution: remarks, outcome: "client", refund_fees: true });
      } else if (choice === "refund_bid") {
        await adminResolveDispute(id, { resolution: remarks, outcome: "client", refund_fees: false });
      } else if (choice === "refund_custom") {
        await adminRefundDispute(id, remarks, customValue);
      } else if (choice === "pay_freelancer") {
        await adminResolveDispute(id, { resolution: remarks, outcome: "freelancer" });
      } else {
        await adminResolveDispute(id, { resolution: remarks, outcome: "none" });
      }
      toast({ title: "Dispute resolved", description: "The decision has been applied and both parties notified." });
      closeResolve();
      fetchDispute();
    } catch (err: unknown) {
      const msg =
        (err as { response?: { data?: { error?: string } } })?.response?.data?.error ??
        "Failed to resolve dispute.";
      toast({ variant: "destructive", title: "Error", description: msg });
    } finally {
      setResolving(false);
    }
  };

  const choices: { value: ResolveChoice; title: string; detail: string; disabled: boolean }[] = payment
    ? [
        {
          value: "refund_all",
          title: `Refund customer in full — ${money(payment.client_paid, payment.currency)}`,
          detail: "The job amount plus all commission and fees go back to the customer. The contract is cancelled.",
          disabled: !canRefund,
        },
        {
          value: "refund_bid",
          title: `Refund the job amount only — ${money(payment.bid, payment.currency)}`,
          detail: `The platform keeps the ${money(payment.client_paid - payment.bid, payment.currency)} of fees as a penalty. The contract is cancelled.`,
          disabled: !canRefund || payment.bid >= payment.client_paid,
        },
        {
          value: "refund_custom",
          title: "Refund a custom amount",
          detail: `Up to ${money(payment.max_refundable, payment.currency)}. The rest stays with the platform and the freelancer is not paid.`,
          disabled: !canRefund,
        },
        {
          value: "pay_freelancer",
          title: `Pay the freelancer — ${money(payment.freelancer_net, payment.currency)}`,
          detail: "The held payment is released to the freelancer and the contract is completed.",
          disabled: !canPayFreelancer,
        },
        {
          value: "remarks_only",
          title: "Close with remarks only",
          detail: "No money moves. The contract is reopened.",
          disabled: false,
        },
      ]
    : [];

  const attachments = dispute?.attachments ?? [];

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Button variant="ghost" size="icon" onClick={() => router.back()}>
          <ArrowLeft className="h-4 w-4" />
        </Button>
        <div className="flex-1">
          <h1 className="text-2xl font-bold tracking-tight">
            {loading ? <Skeleton className="h-7 w-48 inline-block" /> : "Dispute Detail"}
          </h1>
          <p className="text-muted-foreground text-sm font-mono">
            {loading ? <Skeleton className="h-4 w-64 mt-1 inline-block" /> : id}
          </p>
        </div>
        {!loading && dispute && dispute.status === "open" && (
          <Button
            onClick={openResolve}
            className="gap-2 bg-green-600 hover:bg-green-700 text-white"
          >
            <CheckCircle className="h-4 w-4" />
            Resolve Dispute
          </Button>
        )}
      </div>

      {loading ? (
        <div className="space-y-4">
          {Array.from({ length: 3 }).map((_, i) => (
            <Card key={i}>
              <CardContent className="pt-6">
                <Skeleton className="h-24 w-full" />
              </CardContent>
            </Card>
          ))}
        </div>
      ) : !dispute ? (
        <Card>
          <CardContent className="py-16 text-center text-muted-foreground">
            Dispute not found.
          </CardContent>
        </Card>
      ) : (
        <>
          {/* Status banner */}
          <div
            className={`rounded-lg border p-4 flex items-center gap-3 ${
              dispute.status === "open"
                ? "bg-red-50 border-red-200 dark:bg-red-950 dark:border-red-800"
                : dispute.status === "resolved"
                ? "bg-green-50 border-green-200 dark:bg-green-950 dark:border-green-800"
                : "bg-muted border-border"
            }`}
          >
            {dispute.status === "open" ? (
              <Clock className="h-5 w-5 text-red-500 shrink-0" />
            ) : (
              <CheckCircle className="h-5 w-5 text-green-500 shrink-0" />
            )}
            <div className="flex-1">
              <p className="font-semibold text-sm capitalize">
                Dispute is{" "}
                <Badge variant={STATUS_BADGE[dispute.status]} className="capitalize">
                  {dispute.status}
                </Badge>
              </p>
              {dispute.status === "resolved" && dispute.resolved_at && (
                <p className="text-xs text-muted-foreground mt-0.5">
                  Resolved on{" "}
                  {new Date(dispute.resolved_at).toLocaleDateString("en-GB", {
                    day: "2-digit", month: "short", year: "numeric",
                  })}
                </p>
              )}
            </div>
          </div>

          {/* Main grid */}
          <div className="grid gap-4 lg:grid-cols-2">
            {/* ── Left: dispute details ──────────────────────────────────── */}
            <Card>
              <CardHeader className="pb-2">
                <CardTitle className="text-base flex items-center gap-2">
                  <FileText className="h-4 w-4" /> Dispute Details
                </CardTitle>
              </CardHeader>
              <CardContent className="space-y-4">
                <div className="grid grid-cols-2 gap-4">
                  <InfoRow
                    label="Filed On"
                    value={new Date(dispute.created_at).toLocaleDateString("en-GB", {
                      day: "2-digit", month: "short", year: "numeric",
                    })}
                  />
                  <InfoRow
                    label="Status"
                    value={
                      <Badge variant={STATUS_BADGE[dispute.status]} className="capitalize">
                        {dispute.status}
                      </Badge>
                    }
                  />
                </div>

                <Separator />

                <div>
                  <p className="text-xs text-muted-foreground mb-1">Reason</p>
                  <p className="text-sm font-medium">{dispute.reason}</p>
                </div>

                <div>
                  <p className="text-xs text-muted-foreground mb-1">Description</p>
                  <p className="text-sm leading-relaxed whitespace-pre-wrap">{dispute.description}</p>
                </div>

                {dispute.resolution && (
                  <>
                    <Separator />
                    <div>
                      <p className="text-xs text-muted-foreground mb-1">Resolution</p>
                      <p className="text-sm leading-relaxed whitespace-pre-wrap bg-muted rounded-md p-3">
                        {dispute.resolution}
                      </p>
                      {dispute.payout_action && (
                        <p className="text-xs text-muted-foreground mt-2">
                          Outcome: {PAYOUT_LABEL[dispute.payout_action] ?? dispute.payout_action}
                        </p>
                      )}
                    </div>
                  </>
                )}
              </CardContent>
            </Card>

            {/* ── Right: contract + parties ──────────────────────────────── */}
            <div className="space-y-4">
              {/* Contract */}
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base">Contract</CardTitle>
                </CardHeader>
                <CardContent className="grid grid-cols-2 gap-4">
                  <InfoRow label="Title" value={dispute.contract?.title ?? "—"} />
                  <InfoRow
                    label="Status"
                    value={
                      <Badge
                        variant={CONTRACT_STATUS_BADGE[dispute.contract?.status ?? ""] ?? "outline"}
                        className="capitalize"
                      >
                        {dispute.contract?.status?.replace(/_/g, " ") ?? "—"}
                      </Badge>
                    }
                  />
                  <div className="col-span-2">
                    <InfoRow
                      label="Contract ID"
                      value={<span className="font-mono text-xs break-all">{dispute.contract_id}</span>}
                    />
                  </div>
                </CardContent>
              </Card>

              {/* Payment */}
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base flex items-center gap-2">
                    <Wallet className="h-4 w-4" /> Payment
                  </CardTitle>
                </CardHeader>
                <CardContent>
                  {!payment?.has_payment ? (
                    <p className="text-sm text-muted-foreground">
                      The client has not paid for this contract, so there is nothing to refund.
                    </p>
                  ) : (
                    <div className="grid grid-cols-2 gap-4">
                      <InfoRow label="Customer paid" value={money(payment.client_paid, payment.currency)} />
                      <InfoRow label="Job amount" value={money(payment.bid, payment.currency)} />
                      <InfoRow label="Freelancer would receive" value={money(payment.freelancer_net, payment.currency)} />
                      <InfoRow label="Platform fees" value={money(payment.client_paid - payment.bid, payment.currency)} />
                      <InfoRow label="Already refunded" value={money(payment.already_refunded, payment.currency)} />
                      <InfoRow
                        label="Money is"
                        value={
                          <Badge variant="outline" className="capitalize">
                            {payment.escrow_status === "held"
                              ? "held in escrow"
                              : payment.escrow_status === "released"
                              ? "paid to freelancer"
                              : payment.escrow_status ?? payment.status}
                          </Badge>
                        }
                      />
                    </div>
                  )}
                </CardContent>
              </Card>

              {/* Filed By — with role badge */}
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base flex items-center gap-2">
                    <User className="h-4 w-4" /> Filed By
                  </CardTitle>
                </CardHeader>
                <CardContent className="flex items-center gap-3">
                  {dispute.filed_by?.profile_photo ? (
                    // eslint-disable-next-line @next/next/no-img-element
                    <img
                      src={dispute.filed_by.profile_photo}
                      alt=""
                      className="h-10 w-10 rounded-full object-cover border shrink-0"
                    />
                  ) : (
                    <div className="h-10 w-10 rounded-full bg-muted flex items-center justify-center text-sm font-bold text-muted-foreground shrink-0">
                      {dispute.filed_by?.first_name?.[0]}
                      {dispute.filed_by?.last_name?.[0]}
                    </div>
                  )}
                  <div className="flex-1">
                    <div className="flex items-center gap-2">
                      <p className="font-semibold text-sm">
                        {dispute.filed_by
                          ? `${dispute.filed_by.first_name} ${dispute.filed_by.last_name}`
                          : "—"}
                      </p>
                      {dispute.filed_by_role && (
                        <span
                          className={`inline-flex items-center rounded-full px-2 py-0.5 text-xs font-medium capitalize ${
                            ROLE_BADGE[dispute.filed_by_role] ?? "bg-muted text-muted-foreground"
                          }`}
                        >
                          {dispute.filed_by_role}
                        </span>
                      )}
                    </div>
                    <p className="text-xs text-muted-foreground">{dispute.filed_by?.email}</p>
                  </div>
                </CardContent>
              </Card>

              {/* Resolved By */}
              {dispute.resolved_by && (
                <Card>
                  <CardHeader className="pb-2">
                    <CardTitle className="text-base flex items-center gap-2">
                      <CheckCircle className="h-4 w-4 text-green-500" /> Resolved By
                    </CardTitle>
                  </CardHeader>
                  <CardContent className="flex items-center gap-3">
                    {dispute.resolved_by.profile_photo ? (
                      // eslint-disable-next-line @next/next/no-img-element
                      <img
                        src={dispute.resolved_by.profile_photo}
                        alt=""
                        className="h-10 w-10 rounded-full object-cover border shrink-0"
                      />
                    ) : (
                      <div className="h-10 w-10 rounded-full bg-muted flex items-center justify-center text-sm font-bold text-muted-foreground shrink-0">
                        {dispute.resolved_by.first_name?.[0]}
                        {dispute.resolved_by.last_name?.[0]}
                      </div>
                    )}
                    <div>
                      <p className="font-semibold text-sm">
                        {dispute.resolved_by.first_name} {dispute.resolved_by.last_name}
                      </p>
                      <p className="text-xs text-muted-foreground">{dispute.resolved_by.email}</p>
                      {dispute.resolved_at && (
                        <p className="text-xs text-muted-foreground">
                          {new Date(dispute.resolved_at).toLocaleDateString("en-GB", {
                            day: "2-digit", month: "short", year: "numeric",
                          })}
                        </p>
                      )}
                    </div>
                  </CardContent>
                </Card>
              )}
            </div>
          </div>

          {/* ── Attachments ─────────────────────────────────────────────── */}
          <Card>
            <CardHeader className="pb-2">
              <CardTitle className="text-base flex items-center gap-2">
                <Paperclip className="h-4 w-4" />
                Attachments
                {attachments.length > 0 && (
                  <span className="ml-1 rounded-full bg-muted px-2 py-0.5 text-xs font-normal text-muted-foreground">
                    {attachments.length}
                  </span>
                )}
              </CardTitle>
            </CardHeader>
            <CardContent>
              {attachments.length === 0 ? (
                <p className="text-sm text-muted-foreground py-4 text-center">
                  No attachments were submitted with this dispute.
                </p>
              ) : (
                <div className="grid gap-2 sm:grid-cols-2">
                  {attachments.map((file) => (
                    <AttachmentItem key={file.id} file={file} />
                  ))}
                </div>
              )}
            </CardContent>
          </Card>
        </>
      )}

      {/* ─── Resolve Dialog ───────────────────────────────────────────────── */}
      <Dialog open={resolveOpen} onOpenChange={(open) => { if (!open) closeResolve(); }}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Resolve Dispute</DialogTitle>
            <DialogDescription>
              Choose whose favour the dispute is resolved in. Money moves immediately and both parties are notified.
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handleResolve} className="space-y-4">
            {choices.length > 0 && (
              <div className="space-y-2">
                {choices.map((c) => (
                  <label
                    key={c.value}
                    className={`flex gap-3 rounded-lg border p-3 text-left ${
                      c.disabled ? "opacity-50 cursor-not-allowed" : "cursor-pointer hover:bg-muted"
                    } ${choice === c.value ? "border-primary bg-muted" : ""}`}
                  >
                    <input
                      type="radio"
                      name="resolve-choice"
                      className="mt-1"
                      checked={choice === c.value}
                      disabled={c.disabled}
                      onChange={() => setChoice(c.value)}
                    />
                    <div className="flex-1">
                      <p className="text-sm font-medium">{c.title}</p>
                      <p className="text-xs text-muted-foreground">{c.detail}</p>
                      {c.value === "refund_custom" && choice === "refund_custom" && (
                        <div className="mt-2 flex items-center gap-2">
                          <span className="text-sm">£</span>
                          <input
                            type="number"
                            min="0.01"
                            step="0.01"
                            max={payment?.max_refundable}
                            value={customAmount}
                            onChange={(e) => setCustomAmount(e.target.value)}
                            placeholder="Amount"
                            className="h-8 w-32 rounded-md border border-input bg-background px-2 text-sm"
                          />
                          {customAmount !== "" && !customValid && (
                            <span className="text-xs text-red-600">
                              Enter an amount up to {money(payment?.max_refundable ?? 0, payment?.currency)}
                            </span>
                          )}
                        </div>
                      )}
                    </div>
                  </label>
                ))}
              </div>
            )}

            <div className="space-y-2">
              <Label htmlFor="resolution">Remarks *</Label>
              <textarea
                id="resolution"
                rows={4}
                placeholder="Explain the decision (min. 5 characters)..."
                value={resolution}
                onChange={(e) => setResolution(e.target.value)}
                required
                minLength={5}
                className="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring resize-none"
              />
              <p className="text-xs text-muted-foreground">{resolution.length} characters</p>
            </div>

            <DialogFooter className="gap-2">
              <Button type="button" variant="outline" onClick={closeResolve}>
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={
                  resolving ||
                  resolution.trim().length < 5 ||
                  (choice === "refund_custom" && !customValid)
                }
                className="bg-green-600 hover:bg-green-700 text-white"
              >
                {resolving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                {choice === "remarks_only" ? "Confirm Resolution" : "Confirm and Apply"}
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
