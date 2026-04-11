"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import {
  ArrowLeft, CheckCircle, Clock, User, FileText, Loader2,
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
  adminGetDispute, adminResolveDispute,
  type Dispute, type DisputeStatus,
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

function InfoRow({ label, value }: { label: string; value: React.ReactNode }) {
  return (
    <div>
      <p className="text-xs text-muted-foreground mb-0.5">{label}</p>
      <div className="text-sm font-medium">{value}</div>
    </div>
  );
}

// ─── Page ─────────────────────────────────────────────────────────────────────

export default function DisputeDetailPage() {
  const { id } = useParams<{ id: string }>();
  const router = useRouter();
  const { toast } = useToast();

  const [dispute, setDispute] = useState<Dispute | null>(null);
  const [loading, setLoading] = useState(true);

  const [resolveOpen, setResolveOpen] = useState(false);
  const [resolution, setResolution] = useState("");
  const [resolving, setResolving] = useState(false);

  const fetchDispute = () => {
    setLoading(true);
    adminGetDispute(id)
      .then((res) => setDispute(res.data.data))
      .catch(console.error)
      .finally(() => setLoading(false));
  };

  useEffect(() => { fetchDispute(); }, [id]);

  const handleResolve = async (e: React.FormEvent) => {
    e.preventDefault();
    if (!resolution.trim() || resolution.length < 5) return;
    setResolving(true);
    try {
      await adminResolveDispute(id, resolution.trim());
      toast({ title: "Dispute resolved", description: "The dispute has been marked as resolved." });
      setResolveOpen(false);
      setResolution("");
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
            onClick={() => setResolveOpen(true)}
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

          {/* Dispute info */}
          <div className="grid gap-4 lg:grid-cols-2">
            {/* Left: dispute details */}
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
                    </div>
                  </>
                )}
              </CardContent>
            </Card>

            {/* Right: contract + parties */}
            <div className="space-y-4">
              {/* Contract */}
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base">Contract</CardTitle>
                </CardHeader>
                <CardContent className="grid grid-cols-2 gap-4">
                  <InfoRow
                    label="Title"
                    value={dispute.contract?.title ?? "—"}
                  />
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
                  <InfoRow
                    label="Contract ID"
                    value={
                      <span className="font-mono text-xs break-all">{dispute.contract_id}</span>
                    }
                  />
                </CardContent>
              </Card>

              {/* Filed By */}
              <Card>
                <CardHeader className="pb-2">
                  <CardTitle className="text-base flex items-center gap-2">
                    <User className="h-4 w-4" /> Filed By
                  </CardTitle>
                </CardHeader>
                <CardContent className="flex items-center gap-3">
                  {dispute.filed_by?.profile_photo ? (
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
                  <div>
                    <p className="font-semibold text-sm">
                      {dispute.filed_by
                        ? `${dispute.filed_by.first_name} ${dispute.filed_by.last_name}`
                        : "—"}
                    </p>
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
        </>
      )}

      {/* ─── Resolve Dialog ───────────────────────────────────────────────── */}
      <Dialog open={resolveOpen} onOpenChange={(open) => { if (!open) { setResolveOpen(false); setResolution(""); } }}>
        <DialogContent className="max-w-lg">
          <DialogHeader>
            <DialogTitle>Resolve Dispute</DialogTitle>
            <DialogDescription>
              Provide a resolution for this dispute. Both parties will be notified.
            </DialogDescription>
          </DialogHeader>

          <form onSubmit={handleResolve} className="space-y-4">
            <div className="space-y-2">
              <Label htmlFor="resolution">Resolution *</Label>
              <textarea
                id="resolution"
                rows={5}
                placeholder="Describe how the dispute is resolved (min. 5 characters)..."
                value={resolution}
                onChange={(e) => setResolution(e.target.value)}
                required
                minLength={5}
                className="flex w-full rounded-md border border-input bg-background px-3 py-2 text-sm shadow-sm placeholder:text-muted-foreground focus-visible:outline-none focus-visible:ring-1 focus-visible:ring-ring resize-none"
              />
              <p className="text-xs text-muted-foreground">
                {resolution.length} characters
              </p>
            </div>

            <DialogFooter className="gap-2">
              <Button type="button" variant="outline" onClick={() => setResolveOpen(false)}>
                Cancel
              </Button>
              <Button
                type="submit"
                disabled={resolving || resolution.trim().length < 5}
                className="bg-green-600 hover:bg-green-700 text-white"
              >
                {resolving && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                Confirm Resolution
              </Button>
            </DialogFooter>
          </form>
        </DialogContent>
      </Dialog>
    </div>
  );
}
