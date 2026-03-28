"use client";

import { useEffect, useState } from "react";
import {
  Tag,
  Users,
  CreditCard,
  TrendingUp,
  ArrowUpRight,
} from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { Skeleton } from "@/components/ui/skeleton";
import { getPromotions, getReferralCodes, getTransactions, PaymentTransaction } from "@/lib/api";

interface DashboardStats {
  totalPromotions: number;
  totalReferralCodes: number;
  totalTransactions: number;
  totalRevenue: number;
}

function StatCard({
  title,
  value,
  description,
  icon: Icon,
  loading,
  color,
}: {
  title: string;
  value: string | number;
  description: string;
  icon: React.ElementType;
  loading: boolean;
  color: string;
}) {
  return (
    <Card className="relative overflow-hidden">
      <CardHeader className="flex flex-row items-center justify-between pb-2">
        <CardTitle className="text-sm font-medium text-muted-foreground">
          {title}
        </CardTitle>
        <div className={`rounded-full p-2 ${color}`}>
          <Icon className="h-4 w-4 text-white" />
        </div>
      </CardHeader>
      <CardContent>
        {loading ? (
          <Skeleton className="h-8 w-24" />
        ) : (
          <div className="text-3xl font-bold">{value}</div>
        )}
        <p className="text-xs text-muted-foreground mt-1 flex items-center gap-1">
          <ArrowUpRight className="h-3 w-3 text-green-500" />
          {description}
        </p>
      </CardContent>
    </Card>
  );
}

export default function DashboardPage() {
  const [stats, setStats] = useState<DashboardStats>({
    totalPromotions: 0,
    totalReferralCodes: 0,
    totalTransactions: 0,
    totalRevenue: 0,
  });
  const [loading, setLoading] = useState(true);

  useEffect(() => {
    const fetchStats = async () => {
      try {
        const [promotionsRes, codesRes, transactionsRes] = await Promise.allSettled([
          getPromotions(),
          getReferralCodes(),
          getTransactions(),
        ]);

        const promotions =
          promotionsRes.status === "fulfilled" ? promotionsRes.value.data : [];
        const codes =
          codesRes.status === "fulfilled" ? codesRes.value.data : [];
        const transactions =
          transactionsRes.status === "fulfilled"
            ? transactionsRes.value.data
            : [];

        const promotionsList = Array.isArray(promotions) ? promotions : (promotions?.data || []);
        const codesList = Array.isArray(codes) ? codes : (codes?.data || []);
        const transactionsList: PaymentTransaction[] = Array.isArray(transactions) ? transactions : (transactions?.data || []);

        const totalRevenue = transactionsList.reduce(
          (sum: number, t: PaymentTransaction) => sum + (t.amount || 0),
          0
        );

        setStats({
          totalPromotions: promotionsList.length,
          totalReferralCodes: codesList.length,
          totalTransactions: transactionsList.length,
          totalRevenue,
        });
      } catch (error) {
        console.error("Failed to fetch dashboard stats:", error);
      } finally {
        setLoading(false);
      }
    };

    fetchStats();
  }, []);

  const formatCurrency = (pence: number) =>
    `£${(pence / 100).toFixed(2)}`;

  const statCards = [
    {
      title: "Total Promotions",
      value: stats.totalPromotions,
      description: "Active promotional offers",
      icon: Tag,
      color: "bg-blue-500",
    },
    {
      title: "Referral Codes",
      value: stats.totalReferralCodes,
      description: "Total referral codes issued",
      icon: Users,
      color: "bg-purple-500",
    },
    {
      title: "Transactions",
      value: stats.totalTransactions,
      description: "Total payment transactions",
      icon: CreditCard,
      color: "bg-green-500",
    },
    {
      title: "Total Revenue",
      value: formatCurrency(stats.totalRevenue),
      description: "Gross transaction volume",
      icon: TrendingUp,
      color: "bg-orange-500",
    },
  ];

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold tracking-tight">Overview</h2>
        <p className="text-muted-foreground">
          Welcome to the Tasksy admin dashboard.
        </p>
      </div>

      <div className="grid gap-4 sm:grid-cols-2 lg:grid-cols-4">
        {statCards.map((card) => (
          <StatCard
            key={card.title}
            title={card.title}
            value={card.value}
            description={card.description}
            icon={card.icon}
            loading={loading}
            color={card.color}
          />
        ))}
      </div>

      {/* Quick actions */}
      <div className="grid gap-4 md:grid-cols-2">
        <Card>
          <CardHeader>
            <CardTitle className="text-base">Quick Actions</CardTitle>
            <CardDescription>Common administrative tasks</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <a
              href="/dashboard/promotions"
              className="flex items-center gap-3 rounded-lg border p-3 hover:bg-muted transition-colors"
            >
              <Tag className="h-5 w-5 text-blue-500" />
              <div>
                <p className="text-sm font-medium">Manage Promotions</p>
                <p className="text-xs text-muted-foreground">
                  Create and edit promotional offers
                </p>
              </div>
            </a>
            <a
              href="/dashboard/referrals"
              className="flex items-center gap-3 rounded-lg border p-3 hover:bg-muted transition-colors"
            >
              <Users className="h-5 w-5 text-purple-500" />
              <div>
                <p className="text-sm font-medium">View Referrals</p>
                <p className="text-xs text-muted-foreground">
                  Monitor referral codes and usage
                </p>
              </div>
            </a>
            <a
              href="/dashboard/transactions"
              className="flex items-center gap-3 rounded-lg border p-3 hover:bg-muted transition-colors"
            >
              <CreditCard className="h-5 w-5 text-green-500" />
              <div>
                <p className="text-sm font-medium">Payment Transactions</p>
                <p className="text-xs text-muted-foreground">
                  View all payment history
                </p>
              </div>
            </a>
          </CardContent>
        </Card>

        <Card>
          <CardHeader>
            <CardTitle className="text-base">System Status</CardTitle>
            <CardDescription>Current platform health</CardDescription>
          </CardHeader>
          <CardContent className="space-y-3">
            <div className="flex items-center justify-between">
              <span className="text-sm">API Server</span>
              <span className="flex items-center gap-1.5 text-sm text-green-600">
                <span className="h-2 w-2 rounded-full bg-green-500" />
                Operational
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm">Payment Gateway</span>
              <span className="flex items-center gap-1.5 text-sm text-green-600">
                <span className="h-2 w-2 rounded-full bg-green-500" />
                Operational
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm">Escrow Service</span>
              <span className="flex items-center gap-1.5 text-sm text-green-600">
                <span className="h-2 w-2 rounded-full bg-green-500" />
                Operational
              </span>
            </div>
            <div className="flex items-center justify-between">
              <span className="text-sm">Notification Service</span>
              <span className="flex items-center gap-1.5 text-sm text-green-600">
                <span className="h-2 w-2 rounded-full bg-green-500" />
                Operational
              </span>
            </div>
          </CardContent>
        </Card>
      </div>
    </div>
  );
}
