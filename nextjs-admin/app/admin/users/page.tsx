import { UsersListView } from "./UsersListView";

export const metadata = { title: "Users — Admin" };

export default function UsersPage() {
  return (
    <main className="container mx-auto px-4 py-8 max-w-7xl">
      <UsersListView />
    </main>
  );
}
