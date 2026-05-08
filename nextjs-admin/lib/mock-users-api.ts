import type { ListViewParams, PaginatedResponse } from "@/components/list-view/types";

// ─── Domain type ──────────────────────────────────────────────────────────────

export type UserRole = "admin" | "moderator" | "user";
export type UserStatus = "active" | "inactive" | "suspended";

export interface User {
  id: string;
  name: string;
  email: string;
  role: UserRole;
  status: UserStatus;
  createdAt: string; // ISO date string
  jobsPosted: number;
  walletBalance: number; // pence
}

// ─── Seed data ────────────────────────────────────────────────────────────────

const SEED_USERS: User[] = Array.from({ length: 87 }, (_, i) => ({
  id: `usr_${String(i + 1).padStart(4, "0")}`,
  name: [
    "Alice Johnson",
    "Bob Smith",
    "Carol White",
    "David Brown",
    "Eve Davis",
    "Frank Miller",
    "Grace Wilson",
    "Henry Moore",
    "Isla Taylor",
    "Jack Anderson",
  ][i % 10],
  email: `user${i + 1}@example.com`,
  role: (["admin", "moderator", "user", "user", "user"] as UserRole[])[i % 5],
  status: (["active", "active", "active", "inactive", "suspended"] as UserStatus[])[i % 5],
  createdAt: new Date(
    Date.now() - Math.floor(Math.random() * 365 * 24 * 60 * 60 * 1000)
  ).toISOString(),
  jobsPosted: Math.floor(Math.random() * 30),
  walletBalance: Math.floor(Math.random() * 500_00), // up to £500 in pence
}));

// ─── Mock API ─────────────────────────────────────────────────────────────────

/** Simulates a network request with filtering, sorting, and pagination. */
export async function fetchUsers(
  params: ListViewParams
): Promise<PaginatedResponse<User>> {
  await new Promise((r) => setTimeout(r, 350)); // simulate latency

  let results = [...SEED_USERS];

  // ── Search ────────────────────────────────────────────────────────────────
  if (params.search) {
    const q = params.search.toLowerCase();
    results = results.filter(
      (u) => u.name.toLowerCase().includes(q) || u.email.toLowerCase().includes(q)
    );
  }

  // ── Filters ───────────────────────────────────────────────────────────────
  if (params.filters.role) {
    results = results.filter((u) => u.role === params.filters.role);
  }
  if (params.filters.status) {
    results = results.filter((u) => u.status === params.filters.status);
  }

  // ── Sorting ───────────────────────────────────────────────────────────────
  if (params.sortColumn) {
    results.sort((a, b) => {
      const aVal = (a as Record<string, unknown>)[params.sortColumn];
      const bVal = (b as Record<string, unknown>)[params.sortColumn];
      const cmp =
        typeof aVal === "string" && typeof bVal === "string"
          ? aVal.localeCompare(bVal)
          : Number(aVal) - Number(bVal);
      return params.sortDirection === "asc" ? cmp : -cmp;
    });
  }

  // ── Pagination ────────────────────────────────────────────────────────────
  const total = results.length;
  const totalPages = Math.max(1, Math.ceil(total / params.limit));
  const offset = (params.page - 1) * params.limit;
  const pageData = results.slice(offset, offset + params.limit);

  return { data: pageData, total, page: params.page, limit: params.limit, totalPages };
}
