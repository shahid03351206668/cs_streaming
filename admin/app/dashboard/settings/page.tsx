"use client";

import { useEffect, useState } from "react";
import { Loader2, Save } from "lucide-react";
import { Button } from "@/components/ui/button";
import { Input } from "@/components/ui/input";
import { Label } from "@/components/ui/label";
import {
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
} from "@/components/ui/card";
import { useToast } from "@/components/ui/use-toast";
import { getSystemSettings, updateSystemSettings, SystemSettings } from "@/lib/api";

const defaultSettings: Omit<SystemSettings, "id"> = {
  client_commission_percentage: 0,
  freelancer_commission_percentage: 0,
  application_fee_amount: 0,
  app_fee_percentage: 0,
  referral_discount_percentage: 0,
  referral_reward_amount: 0,
};

export default function SettingsPage() {
  const [settings, setSettings] = useState(defaultSettings);
  const [loading, setLoading] = useState(true);
  const [saving, setSaving] = useState(false);
  const { toast } = useToast();

  useEffect(() => {
    getSystemSettings()
      .then((res) => {
        const data = res.data?.data ?? res.data;
        if (data) setSettings(data);
      })
      .catch(() =>
        toast({ variant: "destructive", title: "Error", description: "Failed to load settings." })
      )
      .finally(() => setLoading(false));
  }, []);

  const handleSave = async (e: React.FormEvent) => {
    e.preventDefault();
    setSaving(true);
    try {
      const res = await updateSystemSettings(settings);
      const updated = res.data?.data ?? res.data;
      if (updated) setSettings(updated);
      toast({ title: "Saved", description: "System settings updated successfully." });
    } catch {
      toast({ variant: "destructive", title: "Error", description: "Failed to save settings." });
    } finally {
      setSaving(false);
    }
  };

  const POUND_FIELDS: Array<keyof typeof defaultSettings> = [
    "application_fee_amount",
    "referral_reward_amount",
  ];

  const numField = (
    label: string,
    key: keyof typeof defaultSettings,
    hint?: string
  ) => {
    const isPound = POUND_FIELDS.includes(key);
    return (
      <div className="space-y-2">
        <Label htmlFor={key}>{label}</Label>
        <div className="relative">
          {isPound && (
            <span className="pointer-events-none absolute left-3 top-1/2 -translate-y-1/2 text-sm text-muted-foreground">
              £
            </span>
          )}
          <Input
            id={key}
            type="number"
            min={0}
            step={isPound ? "0.01" : "0.01"}
            value={settings[key]}
            onChange={(e) =>
              setSettings((s) => ({ ...s, [key]: Number(e.target.value) }))
            }
            placeholder={isPound ? "0.00" : "0"}
            className={isPound ? "pl-7" : undefined}
          />
        </div>
        {hint && <p className="text-xs text-muted-foreground">{hint}</p>}
      </div>
    );
  };

  return (
    <div className="space-y-6">
      <div>
        <h2 className="text-2xl font-bold tracking-tight">System Settings</h2>
        <p className="text-muted-foreground">
          Configure platform-wide fee and commission settings.
        </p>
      </div>

      {loading ? (
        <div className="flex items-center justify-center py-20">
          <Loader2 className="h-8 w-8 animate-spin text-muted-foreground" />
        </div>
      ) : (
        <form onSubmit={handleSave} className="space-y-6">
          <Card>
            <CardHeader>
              <CardTitle className="text-base">Commission Settings</CardTitle>
              <CardDescription>
                Percentages charged to clients and freelancers per transaction.
              </CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              {numField(
                "Client Commission (%)",
                "client_commission_percentage",
                "Fee charged to the client on top of the job price."
              )}
              {numField(
                "Freelancer Commission (%)",
                "freelancer_commission_percentage",
                "Fee deducted from the freelancer's payout."
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Application Fee</CardTitle>
              <CardDescription>
                Fixed or percentage-based platform application fee.
              </CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              {numField(
                "App Fee Amount (£)",
                "application_fee_amount",
                "Fixed fee in pounds (e.g. 1.50 = £1.50)."
              )}
              {numField(
                "App Fee Percentage (%)",
                "app_fee_percentage",
                "Percentage-based fee applied per transaction."
              )}
            </CardContent>
          </Card>

          <Card>
            <CardHeader>
              <CardTitle className="text-base">Referral Settings</CardTitle>
              <CardDescription>
                Control referral discounts and rewards issued to users.
              </CardDescription>
            </CardHeader>
            <CardContent className="grid gap-4 sm:grid-cols-2">
              {numField(
                "Referral Discount (%)",
                "referral_discount_percentage",
                "Discount given to the referee on their first transaction."
              )}
              {numField(
                "Referral Reward Amount (£)",
                "referral_reward_amount",
                "Reward credited to the referrer's wallet in pounds (e.g. 5.00 = £5.00)."
              )}
            </CardContent>
          </Card>

          <div className="flex justify-end">
            <Button type="submit" disabled={saving} className="gap-2">
              {saving ? (
                <Loader2 className="h-4 w-4 animate-spin" />
              ) : (
                <Save className="h-4 w-4" />
              )}
              Save Settings
            </Button>
          </div>
        </form>
      )}
    </div>
  );
}
