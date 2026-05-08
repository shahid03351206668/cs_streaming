"use client";

import { useRouter } from "next/navigation";
import { EditIcon, EyeIcon, ShieldBanIcon, TrashIcon } from "lucide-react";

import { Badge } from "@/components/ui/badge";
import { Avatar, AvatarFallback } from "@/components/ui/avatar";

import { ListView } from "@/components/list-view/ListView";
import type { ColumnDef, FilterDef, RowAction } from "@/components/list-view/types";
import { fetchUsers, type User } from "@/lib/mock-users-api";

// ─── Status / Role badges ─────────────────────────────────────────────────────

const STATUS_VARIANT = {
  active:    "default",
  inactive:  "secondary",
  suspended: "destructive",
} as const;

const ROLE_VARIANT = {
  admin:     "default",
  moderator: "secondary",
  user:      "outline",
} as const;

// ─── Column definitions ───────────────────────────────────────────────────────

const COLUMNS: ColumnDef<User>[] = [
  {
    key: "name",
    label: "User",
    sortable: true,
    render: (_, row) => (
      <div className="flex items-center gap-3">
        <Avatar className="h-8 w-8">
          <AvatarFallback className="text-xs">
            {row.name.split(" ").map((n) => n[0]).join("").slice(0, 2)}
          </AvatarFallback>
        </Avatar>
        <div className="leading-tight">
          <p className="font-medium">{row.name}</p>
          <p className="text-xs text-muted-foreground">{row.email}</p>
        </div>
      </div>
    ),
  },
  {
    key: "role",
    label: "Role",
    sortable: true,
    render: (value) => (
      <Badge variant={ROLE_VARIANT[value as keyof typeof ROLE_VARIANT] ?? "outline"}>
        {String(value)}
      </Badge>
    ),
  },
  {
    key: "status",
    label: "Status",
    sortable: true,
    render: (value) => (
      <Badge variant={STATUS_VARIANT[value as keyof typeof STATUS_VARIANT] ?? "secondary"}>
        {String(value)}
      </Badge>
    ),
  },
  {
    key: "jobsPosted",
    label: "Jobs",
    sortable: true,
    hideOnMobile: true,
    className: "text-center",
    render: (value) => <span className="font-mono">{String(value)}</span>,
  },
  {
    key: "walletBalance",
    label: "Wallet",
    sortable: true,
    hideOnMobile: true,
    render: (value) =>
      `£${(Number(value) / 100).toLocaleString("en-GB", { minimumFractionDigits: 2 })}`,
  },
  {
    key: "createdAt",
    label: "Joined",
    sortable: true,
    hideOnMobile: true,
    render: (value) =>
      new Intl.DateTimeFormat("en-GB", { dateStyle: "medium" }).format(
        new Date(String(value))
      ),
  },
];

// ─── Filter definitions ───────────────────────────────────────────────────────

const FILTERS: FilterDef[] = [
  {
    key: "role",
    label: "Role",
    options: [
      { label: "Admin",     value: "admin" },
      { label: "Moderator", value: "moderator" },
      { label: "User",      value: "user" },
    ],
  },
  {
    key: "status",
    label: "Status",
    options: [
      { label: "Active",    value: "active" },
      { label: "Inactive",  value: "inactive" },
      { label: "Suspended", value: "suspended" },
    ],
  },
];

// ─── Component ────────────────────────────────────────────────────────────────

export function UsersListView() {
  const router = useRouter();

  const ACTIONS: RowAction<User>[] = [
    {
      label: "View",
      icon: <EyeIcon className="h-4 w-4" />,
      onClick: (row) => router.push(`/admin/users/${row.id}`),
    },
    {
      label: "Edit",
      icon: <EditIcon className="h-4 w-4" />,
      onClick: (row) => router.push(`/admin/users/${row.id}/edit`),
    },
    {
      label: "Suspend",
      icon: <ShieldBanIcon className="h-4 w-4" />,
      onClick: (row) => {
        // call suspend API, then invalidate query
        console.log("Suspend user", row.id);
      },
      hidden: (row) => row.status === "suspended",
    },
    {
      label: "Delete",
      icon: <TrashIcon className="h-4 w-4" />,
      variant: "destructive",
      onClick: (row) => {
        // show confirmation dialog, then call delete API
        console.log("Delete user", row.id);
      },
      disabled: (row) => row.role === "admin",
    },
  ];

  return (
    <ListView<User>
      title="Users"
      description="Manage platform users, roles, and wallet balances."
      columns={COLUMNS}
      queryKey={["admin", "users"]}
      queryFn={fetchUsers}
      actions={ACTIONS}
      filters={FILTERS}
      defaultSort={{ column: "createdAt", direction: "desc" }}
      pageSizeOptions={[10, 25, 50]}
      onCreateClick={() => router.push("/admin/users/new")}
      createLabel="Add User"
      searchPlaceholder="Search by name or email…"
      rowClassName={(row) =>
        row.status === "suspended" ? "opacity-60" : ""
      }
    />
  );
}
