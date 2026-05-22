// All category-related API calls live here so components stay decoupled.

const BASE = process.env.NEXT_PUBLIC_API_URL ?? "http://localhost:8080";

export interface Category {
  id: string;
  name: string;
  sort_order: number;
}

export async function fetchCategories(): Promise<Category[]> {
  const res = await fetch(`${BASE}/api/v1/category/list`, {
    next: { tags: ["categories"] },
  });
  if (!res.ok) throw new Error("Failed to fetch categories");
  const json = await res.json();
  return json.data as Category[];
}

export async function createCategory(name: string): Promise<Category> {
  const res = await fetch(`${BASE}/api/v1/category/create`, {
    method: "POST",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ name }),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error ?? "Failed to create category");
  }
  return (await res.json()).data as Category;
}

export async function updateCategory(
  id: string,
  payload: { name: string; disable?: boolean }
): Promise<Category> {
  const res = await fetch(`${BASE}/api/v1/category/update/${id}`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify(payload),
  });
  if (!res.ok) {
    const err = await res.json().catch(() => ({}));
    throw new Error(err.error ?? "Failed to update category");
  }
  return (await res.json()).data as Category;
}

export async function deleteCategory(id: string): Promise<void> {
  const res = await fetch(`${BASE}/api/v1/category/delete/${id}`, {
    method: "DELETE",
  });
  if (!res.ok) throw new Error("Failed to delete category");
}

/** Sends the new ordered list of IDs to the backend. */
export async function reorderCategories(ids: string[]): Promise<Category[]> {
  const res = await fetch(`${BASE}/api/v1/category/reorder`, {
    method: "PUT",
    headers: { "Content-Type": "application/json" },
    body: JSON.stringify({ ids }),
  });
  if (!res.ok) throw new Error("Failed to save new order");
  return (await res.json()).data as Category[];
}
