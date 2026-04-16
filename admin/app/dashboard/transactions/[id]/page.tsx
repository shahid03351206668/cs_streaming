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
import { getTransactionById, PaymentTransaction } from "@/lib/api";

function statusVariant(
  status: string
): "default" | "secondary" | "destructive" | "outline" {
  switch (status?.toLowerCase()) {
    case "success":
    case "completed":
    case "paid":
      return "default";
    case "failed":
    case "cancelled":
      return "destructive";
    case "refunded":
      return "outline";
    default:
      return "secondary";
  }
}

const fmt = (pence: number, currency = "GBP") =>
  `${currency?.toUpperCase() === "GBP" ? "£" : "$"}${(pence / 100).toFixed(2)}`;

const fmtDateTime = (d: string) =>
  d
    ? new Date(d).toLocaleString("en-GB", {
        day: "2-digit",
        month: "short",
        year: "numeric",
        hour: "2-digit",
        minute: "2-digit",
        second: "2-digit",
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

export default function TransactionDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const { toast } = useToast();
  const [tx, setTx] = useState<PaymentTransaction | null>(null);
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    if (!id) return;
    getTransactionById(id)
      .then((res) => setTx(res.data?.data ?? null))
      .catch(() => {
        toast({
          variant: "destructive",
          title: "Error",
          description: "Transaction not found.",
        });
      })
      .finally(() => setLoading(false));
  }, [id]);

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
            Transaction Detail
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
      ) : !tx ? (
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
              <CardTitle className="text-base">Amounts</CardTitle>
            </CardHeader>
            <CardContent>
              <div className="grid grid-cols-2 gap-6 sm:grid-cols-4">
                <div className="space-y-1">
                  <p className="text-xs font-medium text-muted-foreground">
                    Gross Amount
                  </p>
                  <p className="text-3xl font-bold font-mono">
                    {fmt(tx.amount, tx.currency)}
                  </p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs font-medium text-muted-foreground">
                    App Fee
                  </p>
                  <p className="text-3xl font-bold font-mono text-orange-600">
                    {fmt(tx.app_fee_amount, tx.currency)}
                  </p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs font-medium text-muted-foreground">
                    Discount
                  </p>
                  <p className="text-3xl font-bold font-mono text-blue-600">
                    {tx.discount_amount ? fmt(tx.discount_amount, tx.currency) : "—"}
                  </p>
                </div>
                <div className="space-y-1">
                  <p className="text-xs font-medium text-muted-foreground">
                    Net Amount
                  </p>
                  <p className="text-3xl font-bold font-mono text-green-600">
                    {fmt(tx.net_amount, tx.currency)}
                  </p>
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
                  label="Status"
                  value={
                    tx.status
                      ? tx.status.charAt(0).toUpperCase() + tx.status.slice(1)
                      : "—"
                  }
                />
                <Field
                  label="Currency"
                  value={tx.currency?.toUpperCase() || "—"}
                />
                <Field
                  label="Reference Type"
                  value={tx.reference_type?.replace(/_/g, " ") || "—"}
                />
                <Field
                  label="Reference ID"
                  value={tx.reference_id || "—"}
                  mono
                />
                <Field
                  label="Referral Code ID"
                  value={tx.referral_code_id || "—"}
                  mono
                />
                <Field
                  label="Payment Method"
                  value={tx.payment_method?.replace(/_/g, " ") || "—"}
                />
                <Field
                  label="Transaction Date"
                  value={fmtDateTime(tx.transaction_date)}
                />
                <Field label="Created At" value={fmtDateTime(tx.created_at)} />
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
