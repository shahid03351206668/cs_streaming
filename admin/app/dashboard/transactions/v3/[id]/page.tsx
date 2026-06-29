"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { ArrowLeft, Loader2 } from "lucide-react";
import { Badge } from "@/components/ui/badge";
import { Button } from "@/components/ui/button";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui/card";
import { Separator } from "@/components/ui/separator";
import { Skeleton } from "@/components/ui/skeleton";
import { useToast } from "@/components/ui/use-toast";
import { getTransactionV3ById, PaymentTransactionV3Detail } from "@/lib/api";

function statusVariant(
  status: string
): "default" | "secondary" | "destructive" | "outline" {
  switch (status?.toLowerCase()) {
    case "released":
    case "completed":
    case "accepted":
    case "open":
      return "default";
    case "refunded":
    case "cancelled":
    case "rejected":
      return "destructive";
    case "held":
    case "pending":
      return "secondary";
    default:
      return "outline";
  }
}

const fmtDecimal = (value: string | number | undefined, currency = "USD") => {
  const n = typeof value === "string" ? parseFloat(value) : value ?? 0;
  return `${currency?.toUpperCase() === "GBP" ? "£" : "$"}${n.toFixed(2)}`;
};

const fmtDateTime = (d: string | null | undefined) =>
  d
    ? new Date(d).toLocaleString("en-GB", {
        day: "2-digit",
        month: "short",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
      })
    : "—";

function Field({
  label,
  value,
  mono,
  colored,
}: {
  label: string;
  value: string;
  mono?: boolean;
  colored?: "green" | "orange" | "blue" | "red";
}) {
  const colorClass =
    colored === "green"
      ? "text-green-600"
      : colored === "orange"
      ? "text-orange-600"
      : colored === "blue"
      ? "text-blue-600"
      : colored === "red"
      ? "text-red-600"
      : "text-foreground";
  return (
    <div className="space-y-0.5">
      <p className="text-xs font-medium text-muted-foreground">{label}</p>
      <p className={`text-sm break-all ${mono ? "font-mono" : "font-medium"} ${colorClass}`}>
        {value}
      </p>
    </div>
  );
}

export default function TransactionV3DetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const { toast } = useToast();
  const [data, setData] = useState<PaymentTransactionV3Detail | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!id) return;
    getTransactionV3ById(id)
      .then((res) => setData(res.data?.data ?? null))
      .catch(() => {
        toast({
          variant: "destructive",
          title: "Error",
          description: "Transaction not found.",
        });
      })
      .finally(() => setLoading(false));
  }, [id]);

  const tx = data?.transaction;

  return (
    <div className="space-y-6">
      {/* Header */}
      <div className="flex items-center gap-3">
        <Button
          variant="ghost"
          size="icon"
          onClick={() => router.push("/dashboard/transactions")}
        >
          <ArrowLeft className="h-5 w-5" />
        </Button>
        <div>
          <h2 className="text-2xl font-bold tracking-tight">
            V3 Transaction Detail
          </h2>
          <p className="text-sm text-muted-foreground font-mono">{id}</p>
        </div>
        {tx && (
          <Badge variant={statusVariant(tx.status)} className="ml-auto">
            {tx.status?.charAt(0).toUpperCase() + tx.status?.slice(1)}
          </Badge>
        )}
      </div>

      {loading ? (
        <div className="space-y-4">
          {Array.from({ length: 4 }).map((_, i) => (
            <Skeleton key={i} className="h-36 w-full rounded-lg" />
          ))}
        </div>
      ) : !data || !tx ? (
        <Card>
          <CardContent className="flex flex-col items-center gap-3 py-16 text-center">
            <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
            <p className="text-muted-foreground">Transaction not found.</p>
          </CardContent>
        </Card>
      ) : (
        <>
          {/* Amounts */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Payment Details</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-6 sm:grid-cols-4">
                <div className="space-y-1">
                  <p className="text-xs font-medium text-muted-foreground">
                    Gross Amount
                  </p>
                  <p className="text-3xl font-bold font-mono">
                    {fmtDecimal(tx.gross_amount, tx.currency)}
                  </p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs font-medium text-muted-foreground">
                    Platform Fee
                  </p>
                  <p className="text-3xl font-bold font-mono text-orange-600">
                    {fmtDecimal(tx.platform_fee, tx.currency)}
                  </p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs font-medium text-muted-foreground">
                    Net Amount
                  </p>
                  <p className="text-3xl font-bold font-mono text-green-600">
                    {fmtDecimal(tx.net_amount, tx.currency)}
                  </p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs font-medium text-muted-foreground">
                    Payment Status
                  </p>
                  <Badge variant={statusVariant(tx.status)} className="text-sm">
                    {tx.status?.charAt(0).toUpperCase() + tx.status?.slice(1)}
                  </Badge>
                </div>
              </div>
            </CardContent>
          </Card>

          {/* Transaction info */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Transaction Info</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-2 gap-6 sm:grid-cols-3">
                <Field label="Transaction ID" value={tx.id} mono />
                <Field
                  label="Stripe Payment Intent"
                  value={tx.stripe_payment_intent_id || "—"}
                  mono
                />
                <Field label="Stripe Charge ID" value={tx.stripe_charge_id || "—"} mono />
                <Field label="Currency" value={tx.currency?.toUpperCase() || "—"} />
                <Field label="Flow Version" value={tx.flow_version || "—"} />
                <Field label="Created At" value={fmtDateTime(tx.created_at)} />
              </div>
            </CardContent>
          </Card>

          {/* Escrow */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Escrow</CardTitle>
            </CardHeader>
            <CardContent className="space-y-4">
              <div className="grid grid-cols-2 gap-6 sm:grid-cols-4">
                <Field
                  label="Escrow Amount"
                  value={fmtDecimal(data.escrow?.amount, data.escrow?.currency)}
                  mono
                  colored="green"
                />
                <Field
                  label="Escrow Status"
                  value={
                    data.escrow?.status
                      ? data.escrow.status.charAt(0).toUpperCase() +
                        data.escrow.status.slice(1)
                      : "—"
                  }
                />
                <Field
                  label="Stripe Transfer ID"
                  value={data.escrow?.stripe_transfer_id || "—"}
                  mono
                />
                <Field label="Held At" value={fmtDateTime(data.escrow?.held_at)} />
                <Field label="Released At" value={fmtDateTime(data.escrow?.released_at)} />
              </div>
            </CardContent>
          </Card>

          {/* Job & Contract */}
          <div className="grid gap-4 sm:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">Job Post</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <Field label="Title" value={data.job_post?.title || "—"} />
                <Separator />
                <Field
                  label="Status"
                  value={
                    data.job_post?.status
                      ? data.job_post.status.charAt(0).toUpperCase() +
                        data.job_post.status.slice(1)
                      : "—"
                  }
                />
                <Separator />
                <Field label="Posted At" value={fmtDateTime(data.job_post?.posted_at)} />
                <Separator />
                <Field
                  label="Budget"
                  value={
                    data.job_post?.open_budget
                      ? "Open budget"
                      : fmtDecimal(data.job_post?.budget, tx.currency)
                  }
                  mono
                />
                <Separator />
                <Field label="Total Proposals" value={String(data.total_proposals ?? 0)} />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">Contract</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <Field label="Title" value={data.contract?.title || "—"} />
                <Separator />
                <Field
                  label="Status"
                  value={
                    data.contract?.status
                      ? data.contract.status.charAt(0).toUpperCase() +
                        data.contract.status.slice(1)
                      : "—"
                  }
                />
                <Separator />
                <Field
                  label="Total Amount"
                  value={fmtDecimal(data.contract?.total_amount, tx.currency)}
                  mono
                />
                <Separator />
                <Field
                  label="Client Completed"
                  value={data.contract?.client_completed ? "Yes" : "No"}
                  colored={data.contract?.client_completed ? "green" : undefined}
                />
                <Separator />
                <Field
                  label="Freelancer Completed"
                  value={data.contract?.freelancer_completed ? "Yes" : "No"}
                  colored={data.contract?.freelancer_completed ? "green" : undefined}
                />
              </CardContent>
            </Card>
          </div>

          {/* Proposal */}
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Accepted Proposal</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-6 sm:grid-cols-3">
                <Field
                  label="Status"
                  value={
                    data.proposal?.status
                      ? data.proposal.status.charAt(0).toUpperCase() +
                        data.proposal.status.slice(1)
                      : "—"
                  }
                />
                <Field
                  label="Bid Amount"
                  value={fmtDecimal(data.proposal?.bid_amount, tx.currency)}
                  mono
                />
                <Field label="Duration (days)" value={String(data.proposal?.duration ?? "—")} />
              </div>
            </CardContent>
          </Card>

          {/* Users */}
          <div className="grid gap-4 sm:grid-cols-2">
            <Card>
              <CardHeader>
                <CardTitle className="text-base">From (Client)</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <Field
                  label="Full Name"
                  value={
                    `${tx.from_user?.first_name ?? ""} ${tx.from_user?.last_name ?? ""}`.trim() ||
                    "—"
                  }
                />
                <Separator />
                <Field label="Email" value={tx.from_user?.email || "—"} />
                <Separator />
                <Field label="User ID" value={tx.from_user?.id || "—"} mono />
              </CardContent>
            </Card>

            <Card>
              <CardHeader>
                <CardTitle className="text-base">To (Freelancer)</CardTitle>
              </CardHeader>
              <CardContent className="space-y-3">
                <Field
                  label="Full Name"
                  value={
                    `${tx.to_user?.first_name ?? ""} ${tx.to_user?.last_name ?? ""}`.trim() ||
                    "—"
                  }
                />
                <Separator />
                <Field label="Email" value={tx.to_user?.email || "—"} />
                <Separator />
                <Field label="User ID" value={tx.to_user?.id || "—"} mono />
              </CardContent>
            </Card>
          </div>
        </>
      )}
    </div>
  );
}
