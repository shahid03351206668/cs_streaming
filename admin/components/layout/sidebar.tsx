"use client";

import Link from "next/link";
import { usePathname } from "next/navigation";
import {
  LayoutDashboard,
  Tag,
  Users,
  CreditCard,
  Settings,
  X,
  Zap,
  Briefcase,
  GitBranch,
  FolderOpen,
  Building2,
} from "lucide-react";
import { cn } from "@/lib/utils";
import { Button } from "@/components/ui/button";

const navItems = [
  { title: "Dashboard",    href: "/dashboard",              icon: LayoutDashboard },
  { title: "Users",        href: "/dashboard/users",        icon: Users },
  { title: "Jobs",         href: "/dashboard/jobs",         icon: Briefcase },
  { title: "Categories",   href: "/dashboard/categories",   icon: FolderOpen },
  { title: "Promotions",   href: "/dashboard/promotions",   icon: Tag },
  { title: "Referrals",    href: "/dashboard/referrals",    icon: GitBranch },
  { title: "Transactions", href: "/dashboard/transactions", icon: CreditCard },
  { title: "Banks",        href: "/dashboard/banks",        icon: Building2 },
  { title: "Settings",     href: "/dashboard/settings",     icon: Settings },
];

interface SidebarProps {
  isOpen: boolean;
  onClose: () => void;
}

export function Sidebar({ isOpen, onClose }: SidebarProps) {
  const pathname = usePathname();

  return (
    <>
      {/* Mobile overlay */}
      {isOpen && (
        <div
          className="fixed inset-0 z-40 bg-black/50 lg:hidden"
          onClick={onClose}
        />
      )}

      {/* Sidebar */}
      <aside
        className={cn(
          "fixed left-0 top-0 z-50 h-full w-64 transform bg-gray-900 text-white transition-transform duration-300 ease-in-out lg:static lg:translate-x-0",
          isOpen ? "translate-x-0" : "-translate-x-full"
        )}
      >
        {/* Logo */}
        <div className="flex items-center justify-between border-b border-gray-700 p-6">
          <div className="flex items-center gap-2">
            <Zap className="h-6 w-6 text-blue-400" />
            <span className="text-xl font-bold">Tasksy Admin</span>
          </div>
          <Button
            variant="ghost"
            size="icon"
            className="lg:hidden text-white hover:bg-gray-700"
            onClick={onClose}
          >
            <X className="h-5 w-5" />
          </Button>
        </div>

        {/* Navigation */}
        <nav className="p-4 space-y-1">
          {navItems.map((item) => {
            const Icon = item.icon;
            const isActive =
              item.href === "/dashboard"
                ? pathname === "/dashboard"
                : pathname.startsWith(item.href);

            return (
              <Link
                key={item.href}
                href={item.href}
                onClick={onClose}
                className={cn(
                  "flex items-center gap-3 rounded-md px-3 py-2.5 text-sm font-medium transition-colors",
                  isActive
                    ? "bg-blue-600 text-white"
                    : "text-gray-300 hover:bg-gray-700 hover:text-white"
                )}
              >
                <Icon className="h-5 w-5 shrink-0" />
                {item.title}
              </Link>
            );
          })}
        </nav>

        {/* Footer */}
        <div className="absolute bottom-0 left-0 right-0 border-t border-gray-700 p-4">
          <p className="text-xs text-gray-500 text-center">
            Tasksy Admin v1.0
          </p>
        </div>
      </aside>
    </>
  );
}
