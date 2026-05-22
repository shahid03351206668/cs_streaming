import { CategoryList } from "@/components/categories/CategoryList";

export const metadata = { title: "Categories — Admin" };

export default function CategoriesPage() {
  return (
    <main className="container mx-auto max-w-2xl px-4 py-8">
      <div className="mb-6">
        <h1 className="text-2xl font-bold tracking-tight">Categories</h1>
        <p className="mt-1 text-muted-foreground">
          Manage job categories. Drag to set the display order shown to users.
        </p>
      </div>
      <CategoryList />
    </main>
  );
}
