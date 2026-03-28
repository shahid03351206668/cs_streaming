"use client";

import { usePathname, useRouter } from "next/navigation";
import { Menu, LogOut, Bell } from "lucide-react";
import { Button } from "@/components/ui/button";

const pageTitles: Record<string, string> = {
  "/dashboard": "Dashboard",
  "/dashboard/categories": "Categories",
  "/dashboard/promotions": "Promotions",
  "/dashboard/referrals": "Referrals",
  "/dashboard/transactions": "Transactions",
  "/dashboard/settings": "Settings",
};

interface HeaderProps {
  onMenuClick: () => void;
}

export function Header({ onMenuClick }: HeaderProps) {
  const pathname = usePathname();
  const router = useRouter();

  const pageTitle =
    pageTitles[pathname] ??
    (pathname.startsWith("/dashboard/jobs/") ? "Job Details" :
     pathname.startsWith("/dashboard/users/") ? "User Details" :
     "Dashboard");

  const handleLogout = () => {
    localStorage.removeItem("tasksy_token");
    router.push("/login");
  };

  return (
    <header className="sticky top-0 z-30 flex h-16 items-center justify-between border-b bg-white px-4 shadow-sm lg:px-6">
      <div className="flex items-center gap-4">
        <Button
          variant="ghost"
          size="icon"
          className="lg:hidden"
          onClick={onMenuClick}
        >
          <Menu className="h-5 w-5" />
        </Button>
        <h1 className="text-xl font-semibold text-gray-900">{pageTitle}</h1>
      </div>

      <div className="flex items-center gap-2">
        <Button variant="ghost" size="icon" className="text-gray-500">
          <Bell className="h-5 w-5" />
        </Button>
        <Button
          variant="ghost"
          size="sm"
          onClick={handleLogout}
          className="gap-2 text-gray-600 hover:text-red-600"
        >
          <LogOut className="h-4 w-4" />
          <span className="hidden sm:inline">Logout</span>
        </Button>
      </div>
    </header>
  );
}
