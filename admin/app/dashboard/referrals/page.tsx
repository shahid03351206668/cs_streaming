"use client";

import { useEffect, useState } from "react";
import { Badge } from "@/components/ui/badge";
import { Skeleton } from "@/components/ui/skeleton";
import {
  Table, TableBody, TableCell, TableHead, TableHeader, TableRow,
} from "@/components/ui/table";
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "@/components/ui/card";
import { DataPagination } from "@/components/ui/data-pagination";
import { useToast } from "@/components/ui/use-toast";
import { getReferralCodes, getReferralUsages } from "@/lib/api";

interface ReferralCode {
  id: string;
  code: string;
  owner_id: string;
  discount_percentage: number;
  discount_amount: number;
  max_uses: number;
  current_uses: number;
  is_active: boolean;
  created_at: string;
}

interface ReferralUsage {
  id: string;
  referral_code: string;
  referee_id: string;
  qualified: boolean;
  discount_applied: number;
  created_at: string;
}

type Tab = "codes" | "usages";

export default function ReferralsPage() {
  const [activeTab, setActiveTab] = useState<Tab>("codes");
  const [codes, setCodes] = useState<ReferralCode[]>([]);
  const [usages, setUsages] = useState<ReferralUsage[]>([]);
  const [codesLoading, setCodesLoading] = useState(true);
  const [usagesLoading, setUsagesLoading] = useState(false);
  const [usagesFetched, setUsagesFetched] = useState(false);
  const [codesPage, setCodesPage] = useState(1);
  const [codesPageSize, setCodesPageSize] = useState(10);
  const [usagesPage, setUsagesPage] = useState(1);
  const [usagesPageSize, setUsagesPageSize] = useState(10);
  const { toast } = useToast();

  const fmt = (p: number) => `£${(p / 100).toFixed(2)}`;

  useEffect(() => {
    getReferralCodes()
      .then((res) => {
        const data = Array.isArray(res.data) ? res.data : (res.data?.data ?? []);
        setCodes(data);
      })
      .catch(() => toast({ variant: "destructive", title: "Error", description: "Failed to load referral codes." }))
      .finally(() => setCodesLoading(false));
  }, []);

  useEffect(() => {
    if (activeTab === "usages" && !usagesFetched) {
      setUsagesLoading(true);
      getReferralUsages()
        .then((res) => {
          const data = Array.isArray(res.data) ? res.data : (res.data?.data ?? []);
          setUsages(data);
          setUsagesFetched(true);
        })
        .catch(() => toast({ variant: "destructive", title: "Error", description: "Failed to load referral usages." }))
        .finally(() => setUsagesLoading(false));
    }
  }, [activeTab, usagesFetched]);

  const paginatedCodes = codes.slice((codesPage - 1) * codesPageSize, codesPage * codesPageSize);
  const paginatedUsages = usages.slice((usagesPage - 1) * usagesPageSize, usagesPage * usagesPageSize);

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold tracking-tight">Referrals</h2>
        <p className="text-muted-foreground">Monitor referral codes and their usage.</p>
      </div>

      <div className="flex gap-1 border-b">
        {(["codes", "usages"] as Tab[]).map((tab) => (
          <button
            key={tab}
            onClick={() => setActiveTab(tab)}
            className={`px-4 py-2 text-sm font-medium border-b-2 transition-colors ${
              activeTab === tab
                ? "border-primary text-primary"
                : "border-transparent text-muted-foreground hover:text-foreground"
            }`}
          >
            {tab === "codes" ? "Referral Codes" : "Usages"}
            <span className="ml-2 rounded-full bg-muted px-2 py-0.5 text-xs">
              {tab === "codes" ? (codesLoading ? "…" : codes.length) : (usagesFetched ? usages.length : "…")}
            </span>
          </button>
        ))}
      </div>

      {activeTab === "codes" && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Referral Codes</CardTitle>
            <CardDescription>All referral codes issued to users.</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {codesLoading ? (
              <div className="p-6 space-y-3">
                {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}
              </div>
            ) : (
              <>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Code</TableHead>
                      <TableHead>Owner ID</TableHead>
                      <TableHead className="text-right">Discount %</TableHead>
                      <TableHead className="text-right">Discount Amt</TableHead>
                      <TableHead className="text-right">Max Uses</TableHead>
                      <TableHead className="text-right">Used</TableHead>
                      <TableHead>Status</TableHead>
                      <TableHead>Created</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {paginatedCodes.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={8} className="text-center text-muted-foreground py-10">
                          No referral codes found.
                        </TableCell>
                      </TableRow>
                    ) : (
                      paginatedCodes.map((code) => (
                        <TableRow key={code.id}>
                          <TableCell>
                            <code className="rounded bg-muted px-2 py-1 text-sm font-mono font-semibold">
                              {code.code}
                            </code>
                          </TableCell>
                          <TableCell className="font-mono text-xs text-muted-foreground">
                            {code.owner_id?.slice(0, 8)}…
                          </TableCell>
                          <TableCell className="text-right">
                            {code.discount_percentage > 0 ? `${code.discount_percentage}%` : "—"}
                          </TableCell>
                          <TableCell className="text-right">
                            {code.discount_amount > 0 ? fmt(code.discount_amount) : "—"}
                          </TableCell>
                          <TableCell className="text-right">{code.max_uses || "∞"}</TableCell>
                          <TableCell className="text-right">
                            <span className={code.current_uses >= (code.max_uses || Infinity) ? "text-red-600 font-medium" : ""}>
                              {code.current_uses}
                            </span>
                          </TableCell>
                          <TableCell>
                            <Badge variant={code.is_active ? "default" : "secondary"}>
                              {code.is_active ? "Active" : "Inactive"}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {code.created_at ? new Date(code.created_at).toLocaleDateString() : "—"}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
                <DataPagination
                  total={codes.length}
                  page={codesPage}
                  pageSize={codesPageSize}
                  onPageChange={setCodesPage}
                  onPageSizeChange={setCodesPageSize}
                />
              </>
            )}
          </CardContent>
        </Card>
      )}

      {activeTab === "usages" && (
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Referral Usages</CardTitle>
            <CardDescription>Track how referral codes are being used.</CardDescription>
          </CardHeader>
          <CardContent className="p-0">
            {usagesLoading ? (
              <div className="p-6 space-y-3">
                {Array.from({ length: 5 }).map((_, i) => <Skeleton key={i} className="h-12 w-full" />)}
              </div>
            ) : (
              <>
                <Table>
                  <TableHeader>
                    <TableRow>
                      <TableHead>Referral Code</TableHead>
                      <TableHead>Referee ID</TableHead>
                      <TableHead>Qualified</TableHead>
                      <TableHead className="text-right">Discount Applied</TableHead>
                      <TableHead>Created</TableHead>
                    </TableRow>
                  </TableHeader>
                  <TableBody>
                    {paginatedUsages.length === 0 ? (
                      <TableRow>
                        <TableCell colSpan={5} className="text-center text-muted-foreground py-10">
                          No referral usages found.
                        </TableCell>
                      </TableRow>
                    ) : (
                      paginatedUsages.map((usage) => (
                        <TableRow key={usage.id}>
                          <TableCell>
                            <code className="rounded bg-muted px-2 py-1 text-sm font-mono">
                              {usage.referral_code}
                            </code>
                          </TableCell>
                          <TableCell className="font-mono text-xs text-muted-foreground">
                            {usage.referee_id?.slice(0, 8)}…
                          </TableCell>
                          <TableCell>
                            <Badge variant={usage.qualified ? "default" : "secondary"}>
                              {usage.qualified ? "Yes" : "No"}
                            </Badge>
                          </TableCell>
                          <TableCell className="text-right">
                            {usage.discount_applied > 0 ? fmt(usage.discount_applied) : "—"}
                          </TableCell>
                          <TableCell className="text-sm text-muted-foreground">
                            {usage.created_at ? new Date(usage.created_at).toLocaleDateString() : "—"}
                          </TableCell>
                        </TableRow>
                      ))
                    )}
                  </TableBody>
                </Table>
                <DataPagination
                  total={usages.length}
                  page={usagesPage}
                  pageSize={usagesPageSize}
                  onPageChange={setUsagesPage}
                  onPageSizeChange={setUsagesPageSize}
                />
              </>
            )}
          </CardContent>
        </Card>
      )}
    </div>
  );
}
