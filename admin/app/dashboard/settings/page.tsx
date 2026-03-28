"use client";

import { Settings, Construction } from "lucide-react";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";

export default function SettingsPage() {
  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold tracking-tight">Settings</h2>
        <p className="text-muted-foreground">
          Configure system preferences and administration settings.
        </p>
      </div>

      <Card className="border-dashed">
        <CardHeader className="text-center">
          <div className="flex justify-center mb-4">
            <div className="flex h-16 w-16 items-center justify-center rounded-full bg-muted">
              <Construction className="h-8 w-8 text-muted-foreground" />
            </div>
          </div>
          <CardTitle>System Settings Coming Soon</CardTitle>
          <CardDescription>
            We&apos;re building out the settings panel. Check back soon for
            configuration options including:
          </CardDescription>
        </CardHeader>
        <CardContent>
          <ul className="space-y-3 text-sm text-muted-foreground max-w-md mx-auto">
            {[
              "Platform fee configuration",
              "Email notification templates",
              "Escrow release conditions",
              "User verification settings",
              "Payment gateway configuration",
              "Webhook endpoints management",
              "API rate limiting controls",
            ].map((item) => (
              <li key={item} className="flex items-center gap-2">
                <Settings className="h-4 w-4 shrink-0 text-muted-foreground/60" />
                {item}
              </li>
            ))}
          </ul>
        </CardContent>
      </Card>
    </div>
  );
}
