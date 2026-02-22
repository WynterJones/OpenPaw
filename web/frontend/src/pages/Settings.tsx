import { useState, useEffect } from "react";
import { generateTheme } from "../lib/theme";
import {
  User,
  Settings2,
  Bell,
  Paintbrush,
  Shield,
  Server,
  Info,
  Save,
  Database,
  HardDrive,
  Download,
  Upload,
  RotateCcw,
  SwatchBook,
  Wrench,
  Plus,
  Search,
  Clock,
  FileText,
  AlertTriangle,
  CheckCircle,
  Trash2,
  Skull,
  Bot,
  Wifi,
  Globe,
  Smartphone,
  ExternalLink,
} from "lucide-react";
import { QRCodeSVG } from "qrcode.react";
import { Toggle } from "../components/Toggle";
import { Header } from "../components/Header";
import { Button } from "../components/Button";
import { Card } from "../components/Card";
import { Input, Select, Textarea } from "../components/Input";
import { Modal } from "../components/Modal";
import { StatusBadge } from "../components/StatusBadge";
import { EmptyState } from "../components/EmptyState";
import { DataTable } from "../components/DataTable";
import { LoadingSpinner } from "../components/LoadingSpinner";
import { useAuth } from "../contexts/AuthContext";
import { useDesign } from "../contexts/DesignContext";
import { api, type SystemInfo } from "../lib/api";
import { useToast } from "../components/Toast";
import {
  isNotificationSoundEnabled,
  setNotificationSoundEnabled,
  playNotificationSound,
  getNotificationVolume,
  setNotificationVolume,
} from "../lib/pushNotifications";

const TABS = [
  { id: "profile", label: "Profile", icon: User },
  { id: "general", label: "General", icon: Settings2 },
  { id: "notifications", label: "Notifications", icon: Bell },
  { id: "network", label: "Network", icon: Wifi },
  { id: "models", label: "AI Models", icon: Bot },
  { id: "design", label: "Design", icon: Paintbrush },
  { id: "security", label: "Security", icon: Shield },
  { id: "system", label: "System", icon: Server },
  { id: "about", label: "About", icon: Info },
  { id: "danger", label: "Danger", icon: Skull },
] as const;

type TabId = (typeof TABS)[number]["id"];

function ProfileTab() {
  const { user, refreshUser } = useAuth();
  const { toast } = useToast();
  const [username, setUsername] = useState(user?.username || "");
  const [savingUsername, setSavingUsername] = useState(false);
  const [currentPassword, setCurrentPassword] = useState("");
  const [newPassword, setNewPassword] = useState("");
  const [confirmPassword, setConfirmPassword] = useState("");
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);

  useEffect(() => {
    if (user?.username) setUsername(user.username);
  }, [user?.username]);

  const usernameChanged =
    username.trim() !== "" && username.trim() !== user?.username;

  const saveUsername = async () => {
    if (!usernameChanged) return;
    setSavingUsername(true);
    try {
      await api.put("/auth/profile", { username: username.trim() });
      await refreshUser();
      toast("success", "Username updated");
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to update username",
      );
    } finally {
      setSavingUsername(false);
    }
  };

  const changePassword = async () => {
    if (newPassword !== confirmPassword) {
      toast("error", "Passwords do not match");
      return;
    }
    if (newPassword.length < 8) {
      toast("error", "Password must be at least 8 characters");
      return;
    }
    setSaving(true);
    try {
      await api.post("/auth/change-password", {
        current_password: currentPassword,
        new_password: newPassword,
      });
      toast("success", "Password changed successfully");
      setCurrentPassword("");
      setNewPassword("");
      setConfirmPassword("");
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to change password",
      );
    } finally {
      setSaving(false);
    }
  };

  const handleAvatarUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (!["image/png", "image/jpeg", "image/webp"].includes(file.type)) {
      toast("error", "Please upload a PNG, JPEG, or WebP image");
      return;
    }
    setUploading(true);
    try {
      const formData = new FormData();
      formData.append("avatar", file);
      const csrfHeaders: Record<string, string> = {};
      const csrf = (await import("../lib/api")).getCSRFToken();
      if (csrf) csrfHeaders["X-CSRF-Token"] = csrf;
      const res = await fetch("/api/v1/auth/avatar", {
        method: "POST",
        headers: csrfHeaders,
        body: formData,
        credentials: "same-origin",
      });
      if (!res.ok) throw new Error("Upload failed");
      await refreshUser();
      toast("success", "Profile photo updated");
    } catch (e) {
      console.warn("uploadProfilePhoto failed:", e);
      toast("error", "Failed to upload photo");
    } finally {
      setUploading(false);
    }
    e.target.value = "";
  };

  return (
    <div className="space-y-6">
      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-4">Account</h3>
        <div className="flex items-center gap-4 mb-6">
          <div className="relative group">
            <div className="w-16 h-16 rounded-full ring-2 ring-accent-primary/30 overflow-hidden flex items-center justify-center bg-accent-muted flex-shrink-0">
              {user?.avatar_path ? (
                <img
                  src={user.avatar_path}
                  alt="Profile"
                  className="w-16 h-16 rounded-full object-cover"
                />
              ) : (
                <User className="w-8 h-8 text-accent-primary" />
              )}
            </div>
            <label className="absolute inset-0 rounded-full bg-black/50 opacity-0 group-hover:opacity-100 transition-opacity flex items-center justify-center cursor-pointer">
              {uploading ? (
                <div className="w-5 h-5 border-2 border-white border-t-transparent rounded-full animate-spin" />
              ) : (
                <Upload className="w-5 h-5 text-white" />
              )}
              <input
                type="file"
                accept="image/png,image/jpeg,image/webp"
                onChange={handleAvatarUpload}
                className="hidden"
                aria-label="Upload profile photo"
                tabIndex={-1}
              />
            </label>
          </div>
          <div>
            <p className="text-lg font-semibold text-text-0">
              {user?.username}
            </p>
            <p className="text-sm text-text-3">Administrator</p>
          </div>
        </div>

        <div className="space-y-3 max-w-sm">
          <Input
            label="Username"
            value={username}
            onChange={(e) => setUsername(e.target.value)}
            placeholder="Min 3 characters"
          />
          <Button
            onClick={saveUsername}
            loading={savingUsername}
            disabled={!usernameChanged}
            icon={<Save className="w-4 h-4" />}
          >
            Update Username
          </Button>
        </div>
      </Card>

      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-4">
          Change Password
        </h3>
        <div className="space-y-3 max-w-sm">
          <Input
            label="Current Password"
            type="password"
            value={currentPassword}
            onChange={(e) => setCurrentPassword(e.target.value)}
            autoComplete="current-password"
          />
          <Input
            label="New Password"
            type="password"
            value={newPassword}
            onChange={(e) => setNewPassword(e.target.value)}
            placeholder="Min 8 characters"
            autoComplete="new-password"
          />
          <Input
            label="Confirm New Password"
            type="password"
            value={confirmPassword}
            onChange={(e) => setConfirmPassword(e.target.value)}
            autoComplete="new-password"
          />
          <Button
            onClick={changePassword}
            loading={saving}
            disabled={!currentPassword || !newPassword || !confirmPassword}
            icon={<Save className="w-4 h-4" />}
          >
            Update Password
          </Button>
        </div>
      </Card>
    </div>
  );
}

function GeneralTab() {
  const { toast } = useToast();
  const [bindAddress, setBindAddress] = useState("127.0.0.1");
  const [port, setPort] = useState("41295");
  const [dataDir, setDataDir] = useState("");
  const [confirmationEnabled, setConfirmationEnabled] = useState(true);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api
      .get<{ bind_address: string; port: number; data_dir: string }>(
        "/settings/general",
      )
      .then((data) => {
        setBindAddress(data.bind_address || "127.0.0.1");
        setPort(String(data.port || 41295));
        setDataDir(data.data_dir || "");
      })
      .catch(() => {});
    api
      .get<Record<string, string>>("/settings")
      .then((data) => {
        if (data.confirmation_enabled === "false")
          setConfirmationEnabled(false);
      })
      .catch(() => {});
  }, []);

  const save = async () => {
    setSaving(true);
    try {
      await api.put("/settings/general", {
        bind_address: bindAddress,
        port: parseInt(port, 10),
      });
      await api.put("/settings", {
        confirmation_enabled: confirmationEnabled ? "true" : "false",
      });
      toast("success", "Settings saved");
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to save settings",
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <div className="space-y-6">
      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-4">
          General Settings
        </h3>
        <div className="space-y-4 max-w-sm">
          <Input
            label="Bind Address"
            value={bindAddress}
            onChange={(e) => setBindAddress(e.target.value)}
          />
          <Input
            label="Port"
            type="number"
            value={port}
            onChange={(e) => setPort(e.target.value)}
          />
          {dataDir && (
            <div className="space-y-1.5">
              <label className="block text-sm font-medium text-text-1">
                Data Directory
              </label>
              <p className="text-sm text-text-2 font-mono bg-surface-2 px-3 py-2 rounded-lg">
                {dataDir}
              </p>
            </div>
          )}
        </div>
      </Card>

      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-4">
          Chat Behavior
        </h3>
        <div className="flex items-center justify-between p-4 rounded-lg bg-surface-2">
          <div>
            <p className="text-sm font-medium text-text-1">
              Build Confirmations
            </p>
            <p className="text-xs text-text-3">
              Ask for confirmation before building tools and dashboards
            </p>
          </div>
          <Toggle
            enabled={confirmationEnabled}
            onChange={setConfirmationEnabled}
            label="Build confirmations"
          />
        </div>
      </Card>

      <Button
        onClick={save}
        loading={saving}
        icon={<Save className="w-4 h-4" />}
      >
        Save Changes
      </Button>
    </div>
  );
}

function NotificationsTab() {
  const { toast } = useToast();
  const [soundEnabled, setSoundEnabled] = useState(isNotificationSoundEnabled);
  const [volume, setVolume] = useState(getNotificationVolume);

  const toggleSound = (enabled: boolean) => {
    setSoundEnabled(enabled);
    setNotificationSoundEnabled(enabled);
    if (enabled) {
      playNotificationSound();
    }
    toast(
      "success",
      enabled ? "Notification sound enabled" : "Notification sound disabled",
    );
  };

  const handleVolumeChange = (e: React.ChangeEvent<HTMLInputElement>) => {
    const val = parseFloat(e.target.value);
    setVolume(val);
    setNotificationVolume(val);
  };

  return (
    <div className="space-y-6">
      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-4">Sound</h3>
        <div className="flex items-center justify-between p-4 rounded-lg bg-surface-2">
          <div>
            <p className="text-sm font-medium text-text-1">
              Notification Sound
            </p>
            <p className="text-xs text-text-3">
              Play a sound when a new notification arrives
            </p>
          </div>
          <Toggle
            enabled={soundEnabled}
            onChange={toggleSound}
            label="Notification sound"
          />
        </div>
        {soundEnabled && (
          <div className="mt-4 p-4 rounded-lg bg-surface-2">
            <div className="flex items-center justify-between mb-2">
              <p className="text-sm font-medium text-text-1">Volume</p>
              <span className="text-xs text-text-3 font-mono">
                {Math.round(volume * 100)}%
              </span>
            </div>
            <input
              type="range"
              min="0"
              max="1"
              step="0.05"
              value={volume}
              onChange={handleVolumeChange}
              onMouseUp={playNotificationSound}
              onTouchEnd={playNotificationSound}
              className="w-full h-1.5 rounded-full appearance-none cursor-pointer bg-surface-3 accent-accent-primary"
            />
          </div>
        )}
        <button
          onClick={playNotificationSound}
          className="mt-3 text-xs text-accent-primary hover:text-accent-hover transition-colors cursor-pointer"
        >
          Preview sound
        </button>
      </Card>

      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-4">
          Browser Notifications
        </h3>
        <div className="flex items-center justify-between p-4 rounded-lg bg-surface-2">
          <div>
            <p className="text-sm font-medium text-text-1">
              Push Notifications
            </p>
            <p className="text-xs text-text-3">
              {typeof Notification !== "undefined"
                ? Notification.permission === "granted"
                  ? "Browser notifications are enabled"
                  : Notification.permission === "denied"
                    ? "Notifications blocked — enable in browser settings"
                    : "Click to enable browser notifications"
                : "Not supported in this browser"}
            </p>
          </div>
          {typeof Notification !== "undefined" &&
            Notification.permission === "default" && (
              <Button
                size="sm"
                variant="secondary"
                onClick={async () => {
                  const granted = await Notification.requestPermission();
                  toast(
                    granted === "granted" ? "success" : "error",
                    granted === "granted"
                      ? "Browser notifications enabled"
                      : "Browser notifications denied",
                  );
                }}
              >
                Enable
              </Button>
            )}
          {typeof Notification !== "undefined" &&
            Notification.permission === "granted" && (
              <CheckCircle className="w-5 h-5 text-green-400 flex-shrink-0" />
            )}
        </div>
      </Card>
    </div>
  );
}

function NetworkTab() {
  const { toast } = useToast();
  const [info, setInfo] = useState<SystemInfo | null>(null);
  const [tailscaleEnabled, setTailscaleEnabled] = useState(false);
  const [saving, setSaving] = useState(false);
  const [enablingLan, setEnablingLan] = useState(false);

  useEffect(() => {
    api
      .get<SystemInfo>("/system/info")
      .then((data) => {
        setInfo(data);
        setTailscaleEnabled(data.tailscale_enabled);
      })
      .catch(() => {});
  }, []);

  const isLocalhostOnly = info?.bind_address === "127.0.0.1" || info?.bind_address === "localhost";

  const setNetworkAccess = async (enable: boolean) => {
    setEnablingLan(true);
    try {
      await api.put("/settings/general", { bind_address: enable ? "0.0.0.0" : "127.0.0.1" });
      toast("success", `Network access ${enable ? "enabled" : "disabled"} — restart OpenPaw to apply`);
      const updated = await api.get<SystemInfo>("/system/info");
      setInfo(updated);
    } catch (err) {
      toast("error", err instanceof Error ? err.message : "Failed to save setting");
    } finally {
      setEnablingLan(false);
    }
  };

  const toggleTailscale = async (enabled: boolean) => {
    setTailscaleEnabled(enabled);
    setSaving(true);
    try {
      await api.put("/settings", {
        tailscale_enabled: enabled ? "true" : "false",
      });
      if (enabled && isLocalhostOnly) {
        await api.put("/settings/general", { bind_address: "0.0.0.0" });
        const updated = await api.get<SystemInfo>("/system/info");
        setInfo(updated);
        toast("success", "Tailscale enabled — restart OpenPaw to apply network binding");
      } else {
        toast("success", `Tailscale ${enabled ? "enabled" : "disabled"}`);
      }
    } catch (err) {
      setTailscaleEnabled(!enabled);
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to save setting",
      );
    } finally {
      setSaving(false);
    }
  };

  const lanUrl = info?.lan_ip ? `http://${info.lan_ip}:${info.port}` : "";
  const tailscaleUrl = info?.tailscale_ip
    ? `http://${info.tailscale_ip}:${info.port}`
    : "";

  return (
    <div className="space-y-6">
      <div className="flex items-center justify-between p-4 rounded-lg bg-surface-2">
        <div>
          <p className="text-sm font-medium text-text-1">Network Access</p>
          <p className="text-xs text-text-3">
            {isLocalhostOnly
              ? "Localhost only — enable to allow LAN and Tailscale connections"
              : "Listening on all interfaces — devices on your network can connect"}
          </p>
          <p className="text-[11px] text-text-3 mt-1">Requires a restart to take effect</p>
        </div>
        <Toggle
          enabled={!isLocalhostOnly}
          onChange={(enabled) => setNetworkAccess(enabled)}
          label="Network access"
          disabled={enablingLan || !info}
        />
      </div>

      <Card>
        <div className="flex items-center gap-3 mb-4">
          <div className="w-8 h-8 rounded-lg bg-accent-muted flex items-center justify-center">
            <Smartphone className="w-4 h-4 text-accent-primary" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-text-1">Local Network</h3>
            <p className="text-xs text-text-3">
              Access OpenPaw from other devices on your network
            </p>
          </div>
        </div>

        {info ? (
          info.lan_ip ? (
            <div className="flex flex-col sm:flex-row gap-6">
              <div className="flex-1 space-y-3">
                <div className="p-3 rounded-lg bg-surface-2">
                  <p className="text-xs text-text-3 mb-1">LAN IP Address</p>
                  <p className="text-sm font-mono font-medium text-text-0">
                    {info.lan_ip}
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-surface-2">
                  <p className="text-xs text-text-3 mb-1">URL</p>
                  <div className="flex items-center gap-2">
                    <p className="text-sm font-mono font-medium text-accent-primary">
                      {lanUrl}
                    </p>
                    <button
                      onClick={() => {
                        navigator.clipboard.writeText(lanUrl);
                        toast("success", "URL copied");
                      }}
                      className="text-text-3 hover:text-text-1 transition-colors cursor-pointer"
                      title="Copy URL"
                    >
                      <ExternalLink className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
                {!isLocalhostOnly ? (
                  <p className="text-xs text-text-3">
                    Scan the QR code with your phone or tablet to open OpenPaw.
                  </p>
                ) : (
                  <div className="flex items-center gap-2 p-2 rounded-lg bg-amber-500/5 border border-amber-500/20">
                    <AlertTriangle className="w-3.5 h-3.5 text-amber-400 flex-shrink-0" />
                    <p className="text-xs text-amber-300/70">
                      Enable network access above to use this URL from other devices.
                    </p>
                  </div>
                )}
              </div>
              {!isLocalhostOnly && (
                <div className="flex-shrink-0 p-4 bg-white rounded-xl self-start">
                  <QRCodeSVG value={lanUrl} size={140} />
                </div>
              )}
            </div>
          ) : (
            <div className="flex items-center gap-3 p-4 rounded-lg bg-surface-2">
              <Wifi className="w-5 h-5 text-text-3" />
              <div>
                <p className="text-sm font-medium text-text-1">
                  No LAN detected
                </p>
                <p className="text-xs text-text-3">
                  Could not find a local network interface. Check your network
                  connection.
                </p>
              </div>
            </div>
          )
        ) : (
          <LoadingSpinner message="Detecting network..." />
        )}
      </Card>

      <Card>
        <div className="flex items-center justify-between mb-4">
          <div className="flex items-center gap-3">
            <div className="w-8 h-8 rounded-lg bg-accent-muted flex items-center justify-center">
              <Globe className="w-4 h-4 text-accent-primary" />
            </div>
            <div>
              <h3 className="text-sm font-semibold text-text-1">Tailscale</h3>
              <p className="text-xs text-text-3">
                Secure remote access via your Tailscale network
              </p>
            </div>
          </div>
          <Toggle
            enabled={tailscaleEnabled}
            onChange={toggleTailscale}
            label="Tailscale remote access"
            disabled={saving}
          />
        </div>

        {tailscaleEnabled ? (
          info?.tailscale_ip ? (
            <div className="flex flex-col sm:flex-row gap-6">
              <div className="flex-1 space-y-3">
                <div className="p-3 rounded-lg bg-surface-2">
                  <p className="text-xs text-text-3 mb-1">
                    Tailscale IP Address
                  </p>
                  <p className="text-sm font-mono font-medium text-text-0">
                    {info.tailscale_ip}
                  </p>
                </div>
                <div className="p-3 rounded-lg bg-surface-2">
                  <p className="text-xs text-text-3 mb-1">URL</p>
                  <div className="flex items-center gap-2">
                    <p className="text-sm font-mono font-medium text-accent-primary">
                      {tailscaleUrl}
                    </p>
                    <button
                      onClick={() => {
                        navigator.clipboard.writeText(tailscaleUrl);
                        toast("success", "URL copied");
                      }}
                      className="text-text-3 hover:text-text-1 transition-colors cursor-pointer"
                      title="Copy URL"
                    >
                      <ExternalLink className="w-3.5 h-3.5" />
                    </button>
                  </div>
                </div>
                {!isLocalhostOnly ? (
                  <div className="flex items-center gap-2 p-3 rounded-lg bg-green-500/5 border border-green-500/20">
                    <CheckCircle className="w-4 h-4 text-green-400 flex-shrink-0" />
                    <p className="text-xs text-green-400">Tailscale connected</p>
                  </div>
                ) : (
                  <div className="flex items-center gap-2 p-3 rounded-lg bg-amber-500/5 border border-amber-500/20">
                    <AlertTriangle className="w-4 h-4 text-amber-400 flex-shrink-0" />
                    <p className="text-xs text-amber-300/70">
                      Enable network access and restart to use Tailscale.
                    </p>
                  </div>
                )}
              </div>
              {!isLocalhostOnly && (
                <div className="flex-shrink-0 p-4 bg-white rounded-xl self-start">
                  <QRCodeSVG value={tailscaleUrl} size={140} />
                </div>
              )}
            </div>
          ) : (
            <div className="flex items-center gap-3 p-4 rounded-lg bg-amber-500/5 border border-amber-500/20">
              <AlertTriangle className="w-5 h-5 text-amber-400 flex-shrink-0" />
              <div>
                <p className="text-sm font-medium text-amber-400">
                  Tailscale not detected
                </p>
                <p className="text-xs text-amber-300/70">
                  Tailscale is enabled but no Tailscale interface was found.
                  Make sure Tailscale is installed and connected.
                </p>
              </div>
            </div>
          )
        ) : (
          <p className="text-xs text-text-3">
            Enable Tailscale to access OpenPaw securely from anywhere. Requires
            Tailscale to be installed and running on this machine.
          </p>
        )}
      </Card>
    </div>
  );
}

function ModelPicker({
  label,
  description,
  value,
  onChange,
}: {
  label: string;
  description: string;
  value: string;
  onChange: (model: string) => void;
}) {
  const [models, setModels] = useState<{ id: string; name: string }[]>([]);
  const [search, setSearch] = useState("");
  const [open, setOpen] = useState(false);

  useEffect(() => {
    api
      .get<{ id: string; name: string }[]>("/settings/available-models")
      .then(setModels)
      .catch(() => {});
  }, []);

  const filtered = models
    .filter(
      (m) =>
        m.id.toLowerCase().includes(search.toLowerCase()) ||
        m.name.toLowerCase().includes(search.toLowerCase()),
    )
    .slice(0, 30);

  const displayName =
    models.find((m) => m.id === value)?.name || value || "Select a model";

  return (
    <Card>
      <h3 className="text-sm font-semibold text-text-1 mb-1">{label}</h3>
      <p className="text-xs text-text-3 mb-4">{description}</p>
      <div className="max-w-sm relative">
        <button
          onClick={() => setOpen(!open)}
          aria-expanded={open}
          aria-haspopup="listbox"
          className="w-full flex items-center justify-between px-3 py-2 rounded-lg border border-border-1 bg-surface-2 text-sm text-text-1 hover:border-border-0 transition-colors cursor-pointer"
        >
          <span className="truncate">{displayName}</span>
          <span className="text-text-3 ml-2 text-xs">
            {open ? "\u25B2" : "\u25BC"}
          </span>
        </button>
        {open && (
          <div className="absolute z-20 mt-1 w-full rounded-lg border border-border-1 bg-surface-1 shadow-xl max-h-64 overflow-hidden flex flex-col">
            <div className="p-2 border-b border-border-0">
              <input
                type="text"
                value={search}
                onChange={(e) => setSearch(e.target.value)}
                placeholder="Search models..."
                className="w-full px-2 py-1.5 rounded-md bg-surface-2 border border-border-1 text-sm text-text-1 placeholder:text-text-3 outline-none focus:border-accent-primary"
                autoFocus
              />
            </div>
            <div className="overflow-y-auto flex-1" role="listbox">
              {filtered.length === 0 ? (
                <p className="p-3 text-xs text-text-3 text-center">
                  No models found
                </p>
              ) : (
                filtered.map((m) => (
                  <button
                    key={m.id}
                    role="option"
                    aria-selected={m.id === value}
                    onClick={() => {
                      onChange(m.id);
                      setOpen(false);
                      setSearch("");
                    }}
                    className={`w-full text-left px-3 py-2 text-sm transition-colors cursor-pointer hover:bg-surface-2 ${
                      m.id === value
                        ? "bg-accent-muted text-accent-text"
                        : "text-text-1"
                    }`}
                  >
                    <span className="block truncate font-medium">{m.name}</span>
                    <span className="block truncate text-xs text-text-3">
                      {m.id}
                    </span>
                  </button>
                ))
              )}
            </div>
          </div>
        )}
      </div>
    </Card>
  );
}

function ModelsTab() {
  const { toast } = useToast();
  const [gatewayModel, setGatewayModel] = useState(
    "anthropic/claude-haiku-4-5",
  );
  const [builderModel, setBuilderModel] = useState(
    "anthropic/claude-sonnet-4-6",
  );
  const [maxTurns, setMaxTurns] = useState(300);
  const [agentTimeoutMin, setAgentTimeoutMin] = useState(60);
  const [saving, setSaving] = useState(false);

  const [apiKeyConfigured, setApiKeyConfigured] = useState(false);
  const [apiKeySource, setApiKeySource] = useState("none");
  const [newApiKey, setNewApiKey] = useState("");
  const [savingKey, setSavingKey] = useState(false);

  useEffect(() => {
    api
      .get<{
        gateway_model: string;
        builder_model: string;
        max_turns: number;
        agent_timeout_min: number;
      }>("/settings/models")
      .then((data) => {
        setGatewayModel(data.gateway_model || "anthropic/claude-haiku-4-5");
        setBuilderModel(data.builder_model || "anthropic/claude-sonnet-4-6");
        if (data.max_turns > 0) setMaxTurns(data.max_turns);
        if (data.agent_timeout_min > 0)
          setAgentTimeoutMin(data.agent_timeout_min);
      })
      .catch(() => {});
    api
      .get<{ configured: boolean; source: string }>("/settings/api-key")
      .then((data) => {
        setApiKeyConfigured(data.configured);
        setApiKeySource(data.source);
      })
      .catch(() => {});
  }, []);

  const save = async () => {
    setSaving(true);
    try {
      await api.put("/settings/models", {
        gateway_model: gatewayModel,
        builder_model: builderModel,
        max_turns: maxTurns,
        agent_timeout_min: agentTimeoutMin,
      });
      toast("success", "Model settings saved");
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to save model settings",
      );
    } finally {
      setSaving(false);
    }
  };

  const saveApiKey = async () => {
    if (!newApiKey.trim()) return;
    setSavingKey(true);
    try {
      await api.put("/settings/api-key", { api_key: newApiKey.trim() });
      setApiKeyConfigured(true);
      setApiKeySource("database");
      setNewApiKey("");
      toast("success", "API key validated and saved");
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to save API key",
      );
    } finally {
      setSavingKey(false);
    }
  };

  return (
    <div className="space-y-6">
      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-1">
          OpenRouter API Key
        </h3>
        <p className="text-xs text-text-3 mb-4">
          Required for all AI features. Set via OPENROUTER_API_KEY env var or
          enter below.
        </p>
        <div
          className={`flex items-center gap-3 p-3 rounded-lg border mb-4 ${
            apiKeyConfigured
              ? "bg-green-500/5 border-green-500/20"
              : "bg-amber-500/5 border-amber-500/20"
          }`}
        >
          <CheckCircle
            className={`w-4 h-4 flex-shrink-0 ${apiKeyConfigured ? "text-green-400" : "text-amber-400"}`}
          />
          <p
            className={`text-sm font-medium ${apiKeyConfigured ? "text-green-400" : "text-amber-400"}`}
          >
            {apiKeyConfigured
              ? `Configured (source: ${apiKeySource})`
              : "Not configured"}
          </p>
        </div>
        {apiKeySource !== "env" && (
          <div className="max-w-sm space-y-3">
            <Input
              label="API Key"
              type="password"
              value={newApiKey}
              onChange={(e) => setNewApiKey(e.target.value)}
              placeholder="sk-or-..."
            />
            <Button
              onClick={saveApiKey}
              loading={savingKey}
              disabled={!newApiKey.trim()}
              icon={<Save className="w-4 h-4" />}
            >
              Validate & Save
            </Button>
          </div>
        )}
        {apiKeySource === "env" && (
          <p className="text-xs text-text-3">
            Key is set via environment variable and cannot be changed from the
            UI.
          </p>
        )}
      </Card>

      <ModelPicker
        label="Gateway Model"
        description="Used to analyze user messages, route requests, and generate summaries. A fast, cheap model is recommended."
        value={gatewayModel}
        onChange={setGatewayModel}
      />

      <ModelPicker
        label="Builder Model"
        description="Used when building tools and dashboards. A balanced model is recommended for speed and quality."
        value={builderModel}
        onChange={setBuilderModel}
      />

      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-1">
          Agent Max Turns
        </h3>
        <p className="text-xs text-text-3 mb-4">
          Maximum number of tool-use turns an agent can take per message. Higher
          values let agents complete larger tasks without stopping.
        </p>
        <div className="max-w-xs">
          <Input
            type="number"
            value={String(maxTurns)}
            onChange={(e) => {
              const v = parseInt(e.target.value, 10);
              if (!isNaN(v) && v > 0) setMaxTurns(v);
            }}
          />
          <p className="text-[11px] text-text-3 mt-1.5">Default: 300</p>
        </div>
      </Card>

      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-1">
          Agent Timeout
        </h3>
        <p className="text-xs text-text-3 mb-4">
          Maximum time (in minutes) an agent can run per message. When hit, the
          agent saves its progress and asks you to say "continue".
        </p>
        <div className="max-w-xs">
          <Input
            type="number"
            value={String(agentTimeoutMin)}
            onChange={(e) => {
              const v = parseInt(e.target.value, 10);
              if (!isNaN(v) && v > 0) setAgentTimeoutMin(v);
            }}
          />
          <p className="text-[11px] text-text-3 mt-1.5">Default: 60 minutes</p>
        </div>
      </Card>

      <Button
        onClick={save}
        loading={saving}
        icon={<Save className="w-4 h-4" />}
      >
        Save Model Settings
      </Button>
    </div>
  );
}

function DesignSystemSection({
  title,
  children,
}: {
  title: string;
  children: React.ReactNode;
}) {
  return (
    <div className="space-y-4">
      <h2 className="text-lg font-semibold text-text-0 border-b border-border-0 pb-2">
        {title}
      </h2>
      {children}
    </div>
  );
}

function ColorSwatch({ name, className }: { name: string; className: string }) {
  return (
    <div className="flex items-center gap-3">
      <div
        className={`w-10 h-10 rounded-lg border border-border-1 ${className}`}
      />
      <span className="text-xs font-mono text-text-2">{name}</span>
    </div>
  );
}

function DesignSystemModal({
  open,
  onClose,
}: {
  open: boolean;
  onClose: () => void;
}) {
  const { toast } = useToast();
  const [inputValue, setInputValue] = useState("");
  const [selectValue, setSelectValue] = useState("");
  const [textareaValue, setTextareaValue] = useState("");

  const sampleData = [
    {
      id: "1",
      name: "Gateway API",
      status: "ready" as const,
      version: "1.2.0",
      date: "2026-02-15",
    },
    {
      id: "2",
      name: "Slack Notifier",
      status: "building" as const,
      version: "0.9.1",
      date: "2026-02-14",
    },
    {
      id: "3",
      name: "DB Backup",
      status: "error" as const,
      version: "2.0.0",
      date: "2026-02-10",
    },
    {
      id: "4",
      name: "Health Check",
      status: "disabled" as const,
      version: "1.0.0",
      date: "2026-01-28",
    },
  ];

  return (
    <Modal open={open} onClose={onClose} title="Design System" size="xl">
      <div className="space-y-10">
        <DesignSystemSection title="Color Palette">
          <Card>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
              Surfaces
            </h4>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
              <ColorSwatch name="surface-0" className="bg-surface-0" />
              <ColorSwatch name="surface-1" className="bg-surface-1" />
              <ColorSwatch name="surface-2" className="bg-surface-2" />
              <ColorSwatch name="surface-3" className="bg-surface-3" />
            </div>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
              Borders
            </h4>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
              <ColorSwatch name="border-0" className="bg-border-0" />
              <ColorSwatch name="border-1" className="bg-border-1" />
            </div>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
              Text
            </h4>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3 mb-6">
              <ColorSwatch name="text-0" className="bg-text-0" />
              <ColorSwatch name="text-1" className="bg-text-1" />
              <ColorSwatch name="text-2" className="bg-text-2" />
              <ColorSwatch name="text-3" className="bg-text-3" />
            </div>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
              Accent & Danger
            </h4>
            <div className="grid grid-cols-2 sm:grid-cols-4 gap-3">
              <ColorSwatch
                name="accent-primary"
                className="bg-accent-primary"
              />
              <ColorSwatch name="accent-hover" className="bg-accent-hover" />
              <ColorSwatch name="accent-muted" className="bg-accent-muted" />
              <ColorSwatch name="danger" className="bg-danger" />
            </div>
          </Card>
        </DesignSystemSection>

        <DesignSystemSection title="Typography">
          <Card>
            <div className="space-y-3">
              <p className="text-3xl font-bold text-text-0">
                Heading 3XL (1.875rem)
              </p>
              <p className="text-2xl font-bold text-text-0">
                Heading 2XL (1.5rem)
              </p>
              <p className="text-xl font-semibold text-text-0">
                Heading XL (1.25rem)
              </p>
              <p className="text-lg font-semibold text-text-0">
                Heading LG (1.125rem)
              </p>
              <p className="text-base text-text-1">
                Body Base (1rem) - Primary text for content and paragraphs.
              </p>
              <p className="text-sm text-text-2">
                Body SM (0.875rem) - Secondary text for descriptions.
              </p>
              <p className="text-xs text-text-3">
                Caption XS (0.75rem) - Muted text for metadata.
              </p>
              <p className="text-sm font-mono text-text-1">
                Monospace - Used for code, IDs, and cron expressions.
              </p>
            </div>
          </Card>
        </DesignSystemSection>

        <DesignSystemSection title="Buttons">
          <Card>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
              Variants
            </h4>
            <div className="flex flex-wrap gap-3 mb-6">
              <Button>Primary</Button>
              <Button variant="secondary">Secondary</Button>
              <Button variant="danger">Danger</Button>
              <Button variant="ghost">Ghost</Button>
            </div>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
              Sizes
            </h4>
            <div className="flex flex-wrap items-center gap-3 mb-6">
              <Button size="sm">Small</Button>
              <Button size="md">Medium</Button>
              <Button size="lg">Large</Button>
            </div>
            <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
              States
            </h4>
            <div className="flex flex-wrap items-center gap-3">
              <Button icon={<Plus className="w-4 h-4" />}>With Icon</Button>
              <Button loading>Loading</Button>
              <Button disabled>Disabled</Button>
            </div>
          </Card>
        </DesignSystemSection>

        <DesignSystemSection title="Form Inputs">
          <Card>
            <div className="space-y-4 max-w-sm">
              <Input
                label="Text Input"
                value={inputValue}
                onChange={(e) => setInputValue(e.target.value)}
                placeholder="Enter some text..."
              />
              <Input
                label="With Error"
                value=""
                onChange={() => {}}
                placeholder="Invalid input"
                error="This field is required"
              />
              <Select
                label="Select"
                value={selectValue}
                onChange={(e) => setSelectValue(e.target.value)}
                options={[
                  { value: "", label: "Choose..." },
                  { value: "a", label: "Option A" },
                  { value: "b", label: "Option B" },
                ]}
              />
              <Textarea
                label="Textarea"
                value={textareaValue}
                onChange={(e) => setTextareaValue(e.target.value)}
                placeholder="Write something..."
                rows={3}
              />
            </div>
          </Card>
        </DesignSystemSection>

        <DesignSystemSection title="Status Badges">
          <Card>
            <div className="flex flex-wrap gap-3">
              <StatusBadge status="ready" />
              <StatusBadge status="building" />
              <StatusBadge status="running" />
              <StatusBadge status="pending" />
              <StatusBadge status="success" />
              <StatusBadge status="error" />
              <StatusBadge status="disabled" />
            </div>
          </Card>
        </DesignSystemSection>

        <DesignSystemSection title="Data Table">
          <Card padding={false}>
            <DataTable
              columns={[
                {
                  key: "name",
                  header: "Name",
                  render: (item: (typeof sampleData)[0]) => (
                    <div className="flex items-center gap-2">
                      <Wrench className="w-4 h-4 text-accent-primary" />
                      <span className="text-sm font-medium text-text-0">
                        {item.name}
                      </span>
                    </div>
                  ),
                },
                {
                  key: "status",
                  header: "Status",
                  render: (item: (typeof sampleData)[0]) => (
                    <StatusBadge status={item.status} />
                  ),
                },
                {
                  key: "version",
                  header: "Version",
                  render: (item: (typeof sampleData)[0]) => (
                    <span className="text-sm text-text-2">v{item.version}</span>
                  ),
                },
              ]}
              data={sampleData}
              keyExtractor={(item) => item.id}
            />
          </Card>
        </DesignSystemSection>

        <DesignSystemSection title="Other Components">
          <Card>
            <div className="space-y-6">
              <div>
                <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
                  Empty State
                </h4>
                <EmptyState
                  icon={<FileText className="w-8 h-8" />}
                  title="Nothing here yet"
                  description="This is how empty states look."
                />
              </div>
              <div>
                <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
                  Loading Spinner
                </h4>
                <LoadingSpinner message="Loading data..." />
              </div>
              <div>
                <h4 className="text-xs font-semibold uppercase tracking-wider text-text-3 mb-3">
                  Toast Notifications
                </h4>
                <div className="flex flex-wrap gap-3">
                  <Button
                    variant="secondary"
                    size="sm"
                    icon={<CheckCircle className="w-4 h-4" />}
                    onClick={() => toast("success", "Success!")}
                  >
                    Success
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    icon={<AlertTriangle className="w-4 h-4" />}
                    onClick={() => toast("error", "Error!")}
                  >
                    Error
                  </Button>
                  <Button
                    variant="secondary"
                    size="sm"
                    icon={<Info className="w-4 h-4" />}
                    onClick={() => toast("warning", "Warning!")}
                  >
                    Warning
                  </Button>
                </div>
              </div>
            </div>
          </Card>
        </DesignSystemSection>

        <DesignSystemSection title="Icon Usage (Lucide React)">
          <Card>
            <p className="text-sm text-text-2 mb-4">
              Standard sizes: w-3 h-3 (inline), w-4 h-4 (buttons), w-5 h-5
              (nav), w-8 h-8 (empty states).
            </p>
            <div className="flex flex-wrap gap-4">
              {[
                { Icon: Wrench, label: "Wrench" },
                { Icon: Search, label: "Search" },
                { Icon: Clock, label: "Clock" },
                { Icon: Plus, label: "Plus" },
                { Icon: FileText, label: "FileText" },
                { Icon: AlertTriangle, label: "Alert" },
                { Icon: CheckCircle, label: "Check" },
              ].map(({ Icon, label }) => (
                <div
                  key={label}
                  className="flex flex-col items-center gap-1 p-2 rounded-lg bg-surface-2 w-20"
                >
                  <Icon className="w-5 h-5 text-text-1" />
                  <span className="text-[10px] text-text-3">{label}</span>
                </div>
              ))}
            </div>
          </Card>
        </DesignSystemSection>
      </div>
    </Modal>
  );
}

const FONT_OPTIONS = [
  { value: "'Vend Sans', system-ui, sans-serif", label: "Vend Sans" },
  { value: "'Inter', system-ui, sans-serif", label: "Inter" },
  { value: "'Nunito', system-ui, sans-serif", label: "Nunito" },
{ value: "'Merriweather', Georgia, serif", label: "Merriweather" },
  { value: "'Fira Code', monospace", label: "Fira Code" },
];

const BG_PRESETS = [
  { url: "/preset-bg/bg-1.webp", name: "Cyber Cat" },
  { url: "/preset-bg/bg-2.webp", name: "Digital Garden" },
  { url: "/preset-bg/bg-3.webp", name: "Peeking Cat" },
  { url: "/preset-bg/bg-4.webp", name: "Cat & Robot" },
  { url: "/preset-bg/bg-5.webp", name: "Garden Gate" },
  { url: "/preset-bg/bg-6.webp", name: "Crystal Path" },
  { url: "/preset-bg/bg-7.webp", name: "Garden Friends" },
  { url: "/preset-bg/bg-8.webp", name: "Shoggoth City" },
  { url: "/preset-bg/bg-9.webp", name: "AI Garden" },
];

function DesignTab() {
  const { accent, config, bgImage, showMascot, saveAll, resetConfig } =
    useDesign();
  const { toast } = useToast();
  const [localAccent, setLocalAccent] = useState(accent);
  const [localFont, setLocalFont] = useState(
    config.font_family || FONT_OPTIONS[0].value,
  );
  const [localFontScale, setLocalFontScale] = useState(
    config.font_scale || "100",
  );
  const [localBg, setLocalBg] = useState(bgImage);
  const [localMascot, setLocalMascot] = useState(showMascot);
  const [saving, setSaving] = useState(false);
  const [uploading, setUploading] = useState(false);
  const [designSystemOpen, setDesignSystemOpen] = useState(false);

  useEffect(() => {
    setLocalAccent(accent);
    setLocalFont(config.font_family || FONT_OPTIONS[0].value);
    setLocalFontScale(config.font_scale || "100");
    setLocalBg(bgImage);
    setLocalMascot(showMascot);
  }, [accent, config.font_family, config.font_scale, bgImage, showMascot]);

  // Live preview as user changes color settings (skip font — handled separately)
  useEffect(() => {
    const theme = generateTheme({ accent: localAccent, mode: "dark" });
    const root = document.documentElement;
    for (const [key, value] of Object.entries(theme)) {
      if (key === "font_family" || key === "bg_image") continue;
      root.style.setProperty(`--op-${key.replace(/_/g, "-")}`, value);
    }
  }, [localAccent]);

  // Live preview font
  useEffect(() => {
    document.documentElement.style.setProperty("--op-font-family", localFont);
  }, [localFont]);

  // Font scale applied on save only — live preview causes layout jitter

  const save = async () => {
    setSaving(true);
    try {
      await saveAll({
        accent: localAccent,
        mode: "dark",
        fontFamily: localFont,
        fontScale: localFontScale,
        bgImage: localBg,
        showMascot: localMascot,
      });
      toast("success", "Design saved");
    } catch (e) {
      console.warn("saveDesign failed:", e);
      toast("error", "Failed to save design");
    } finally {
      setSaving(false);
    }
  };

  const reset = async () => {
    setSaving(true);
    try {
      await resetConfig();
      setLocalBg("");
      setLocalMascot(true);
      toast("success", "Design reset to defaults");
    } catch (e) {
      console.warn("resetDesign failed:", e);
      toast("error", "Failed to reset design");
    } finally {
      setSaving(false);
    }
  };

  const handleBgUpload = async (e: React.ChangeEvent<HTMLInputElement>) => {
    const file = e.target.files?.[0];
    if (!file) return;
    if (!["image/png", "image/jpeg", "image/webp"].includes(file.type)) {
      toast("error", "Please upload a PNG, JPEG, or WebP image");
      return;
    }
    if (file.size > 5 * 1024 * 1024) {
      toast("error", "Image must be under 5MB");
      return;
    }
    setUploading(true);
    try {
      const formData = new FormData();
      formData.append("background", file);
      const csrfHeaders: Record<string, string> = {};
      const csrf = (await import("../lib/api")).getCSRFToken();
      if (csrf) csrfHeaders["X-CSRF-Token"] = csrf;
      const res = await fetch("/api/v1/settings/design/background", {
        method: "POST",
        headers: csrfHeaders,
        body: formData,
        credentials: "same-origin",
      });
      if (!res.ok) throw new Error("Upload failed");
      const data = await res.json();
      setLocalBg(data.url);
      toast("success", "Background uploaded");
    } catch {
      toast("error", "Failed to upload background");
    } finally {
      setUploading(false);
    }
    e.target.value = "";
  };

  const NEON_PRESETS = [
    { color: "#FF2D9B", name: "Neon Pink" },
    { color: "#A855F7", name: "Neon Purple" },
    { color: "#3B82F6", name: "Neon Blue" },
    { color: "#00E5FF", name: "Neon Cyan" },
    { color: "#22D55E", name: "Neon Green" },
    { color: "#FACC15", name: "Neon Yellow" },
    { color: "#FF6B2B", name: "Neon Orange" },
    { color: "#FF3344", name: "Neon Red" },
  ];

  const PASTEL_PRESETS = [
    { color: "#FFB3D9", name: "Pastel Pink" },
    { color: "#C4B5FD", name: "Pastel Purple" },
    { color: "#93C5FD", name: "Pastel Blue" },
    { color: "#A5F3FC", name: "Pastel Cyan" },
    { color: "#A7F3D0", name: "Pastel Green" },
    { color: "#FDE68A", name: "Pastel Yellow" },
    { color: "#FED7AA", name: "Pastel Orange" },
    { color: "#FCA5A5", name: "Pastel Red" },
  ];

  return (
    <div className="space-y-6">
      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-1">Appearance</h3>
        <p className="text-xs text-text-3 mb-5">
          Choose your accent color. All UI colors are generated automatically.
        </p>

        <div>
          <label className="block text-xs font-medium text-text-2 mb-2">
            Neon
          </label>
          <div className="flex flex-wrap gap-2 mb-3">
            {NEON_PRESETS.map((p) => (
              <button
                key={p.color}
                onClick={() => setLocalAccent(p.color)}
                aria-label={p.name}
                aria-pressed={
                  localAccent.toLowerCase() === p.color.toLowerCase()
                }
                className={`group relative w-10 h-10 rounded-xl transition-all cursor-pointer ${
                  localAccent.toLowerCase() === p.color.toLowerCase()
                    ? "ring-2 ring-offset-2 ring-offset-surface-1 scale-110"
                    : "hover:scale-105"
                }`}
                style={
                  {
                    backgroundColor: p.color,
                    "--tw-ring-color": p.color,
                  } as React.CSSProperties
                }
                title={p.name}
              >
                {localAccent.toLowerCase() === p.color.toLowerCase() && (
                  <svg
                    className="w-4 h-4 text-white absolute inset-0 m-auto drop-shadow-sm"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    strokeWidth={3}
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      d="M5 13l4 4L19 7"
                    />
                  </svg>
                )}
              </button>
            ))}
          </div>
          <label className="block text-xs font-medium text-text-2 mb-2">
            Pastel
          </label>
          <div className="flex flex-wrap gap-2 mb-4">
            {PASTEL_PRESETS.map((p) => (
              <button
                key={p.color}
                onClick={() => setLocalAccent(p.color)}
                aria-label={p.name}
                aria-pressed={
                  localAccent.toLowerCase() === p.color.toLowerCase()
                }
                className={`group relative w-10 h-10 rounded-xl transition-all cursor-pointer ${
                  localAccent.toLowerCase() === p.color.toLowerCase()
                    ? "ring-2 ring-offset-2 ring-offset-surface-1 scale-110"
                    : "hover:scale-105"
                }`}
                style={
                  {
                    backgroundColor: p.color,
                    "--tw-ring-color": p.color,
                  } as React.CSSProperties
                }
                title={p.name}
              >
                {localAccent.toLowerCase() === p.color.toLowerCase() && (
                  <svg
                    className="w-4 h-4 text-gray-800 absolute inset-0 m-auto drop-shadow-sm"
                    fill="none"
                    viewBox="0 0 24 24"
                    stroke="currentColor"
                    strokeWidth={3}
                  >
                    <path
                      strokeLinecap="round"
                      strokeLinejoin="round"
                      d="M5 13l4 4L19 7"
                    />
                  </svg>
                )}
              </button>
            ))}
          </div>
          <div className="flex items-center gap-3 max-w-xs">
            <input
              type="color"
              value={localAccent}
              onChange={(e) => setLocalAccent(e.target.value)}
              className="w-10 h-10 rounded-xl border border-border-1 cursor-pointer bg-transparent p-0.5"
            />
            <div className="flex-1">
              <p className="text-xs text-text-3">Custom color</p>
              <p className="text-xs font-mono text-text-2">{localAccent}</p>
            </div>
          </div>
        </div>
      </Card>

      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-1">
          Background Image
        </h3>
        <p className="text-xs text-text-3 mb-4">
          Set a background image for the entire UI. Choose none, a preset, or
          upload your own.
        </p>

        <div className="space-y-4">
          <button
            onClick={() => setLocalBg("")}
            aria-pressed={!localBg}
            className={`px-4 py-2 rounded-xl border-2 text-sm font-medium transition-all cursor-pointer ${
              !localBg
                ? "border-accent-primary bg-accent-muted text-accent-text"
                : "border-border-1 bg-surface-2 text-text-2 hover:border-border-0"
            }`}
          >
            None
          </button>

          <div>
            <label className="block text-xs font-medium text-text-2 mb-2">
              Presets
            </label>
            <div className="flex flex-wrap gap-3">
              {BG_PRESETS.map((p) => (
                <button
                  key={p.url}
                  onClick={() => setLocalBg(p.url)}
                  aria-label={p.name}
                  aria-pressed={localBg === p.url}
                  className={`relative w-20 h-14 rounded-lg overflow-hidden border-2 transition-all cursor-pointer bg-cover bg-center ${
                    localBg === p.url
                      ? "border-accent-primary ring-2 ring-accent-primary/30 scale-105"
                      : "border-border-1 hover:border-border-0 hover:scale-105"
                  }`}
                  style={{ backgroundImage: `url(${p.url})` }}
                  title={p.name}
                >
                  {localBg === p.url && (
                    <div className="absolute inset-0 bg-black/40 flex items-center justify-center">
                      <svg
                        className="w-4 h-4 text-white drop-shadow-sm"
                        fill="none"
                        viewBox="0 0 24 24"
                        stroke="currentColor"
                        strokeWidth={3}
                      >
                        <path
                          strokeLinecap="round"
                          strokeLinejoin="round"
                          d="M5 13l4 4L19 7"
                        />
                      </svg>
                    </div>
                  )}
                </button>
              ))}
            </div>
          </div>

          <div>
            <label className="block text-xs font-medium text-text-2 mb-2">
              Upload Custom
            </label>
            <label
              className={`inline-flex items-center gap-2 px-4 py-2 rounded-xl border-2 border-dashed border-border-1 text-sm text-text-2 cursor-pointer transition-colors hover:border-accent-primary hover:text-accent-text ${uploading ? "opacity-50 pointer-events-none" : ""}`}
            >
              {uploading ? (
                <div className="w-4 h-4 border-2 border-current border-t-transparent rounded-full animate-spin" />
              ) : (
                <Upload className="w-4 h-4" />
              )}
              {uploading ? "Uploading..." : "Choose image"}
              <input
                type="file"
                accept="image/png,image/jpeg,image/webp"
                onChange={handleBgUpload}
                className="hidden"
              />
            </label>
            <p className="text-[11px] text-text-3 mt-1.5">
              PNG, JPEG, or WebP. Max 5MB.
            </p>
            {localBg &&
              !BG_PRESETS.some((p) => p.url === localBg) &&
              localBg.startsWith("/api/") && (
                <div className="mt-2 flex items-center gap-2">
                  <div
                    className="w-20 h-14 rounded-lg border border-border-1 bg-cover bg-center"
                    style={{ backgroundImage: `url(${localBg})` }}
                  />
                  <span className="text-xs text-text-3">
                    Custom image selected
                  </span>
                </div>
              )}
          </div>
        </div>
      </Card>

      <Card>
        <div className="flex items-center justify-between">
          <div>
            <h3 className="text-sm font-semibold text-text-1">
              Toolbar Mascot
            </h3>
            <p className="text-xs text-text-3 mt-0.5">
              Show the cat mascot in the header toolbar.
            </p>
          </div>
          <button
            role="switch"
            aria-checked={localMascot}
            onClick={() => setLocalMascot(!localMascot)}
            className={`relative w-10 h-6 rounded-full transition-colors cursor-pointer ${
              localMascot ? "bg-accent-primary" : "bg-surface-3"
            }`}
          >
            <span
              className={`absolute top-0.5 left-0.5 w-5 h-5 rounded-full bg-white shadow transition-transform ${
                localMascot ? "translate-x-4" : ""
              }`}
            />
          </button>
        </div>
      </Card>

      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-1">Font</h3>
        <p className="text-xs text-text-3 mb-4">
          Choose a typeface for the entire UI.
        </p>
        <div className="flex flex-wrap gap-2">
          {FONT_OPTIONS.map((f) => (
            <button
              key={f.value}
              onClick={() => setLocalFont(f.value)}
              aria-pressed={
                localFont.split(",")[0].trim() === f.value.split(",")[0].trim()
              }
              className={`px-4 py-2.5 rounded-xl border-2 text-sm font-medium transition-all cursor-pointer ${
                localFont.split(",")[0].trim() === f.value.split(",")[0].trim()
                  ? "border-accent-primary bg-accent-muted text-accent-text"
                  : "border-border-1 bg-surface-2 text-text-2 hover:border-border-0"
              }`}
              style={{ fontFamily: f.value }}
            >
              {f.label}
            </button>
          ))}
        </div>
      </Card>

      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-1">Text Size</h3>
        <p className="text-xs text-text-3 mb-4">
          Adjust the overall UI zoom level.
        </p>
        <div className="max-w-sm">
          <div className="flex items-center gap-4">
            <span className="text-xs text-text-3 w-6 shrink-0">A</span>
            <input
              type="range"
              min="75"
              max="125"
              step="5"
              value={localFontScale}
              onChange={(e) => setLocalFontScale(e.target.value)}
              className="flex-1 h-1.5 rounded-full appearance-none cursor-pointer bg-surface-3 accent-accent-primary"
            />
            <span className="text-lg text-text-3 w-6 shrink-0">A</span>
          </div>
          <div className="flex items-center justify-between mt-2">
            <span className="text-[11px] text-text-3">75%</span>
            <button
              onClick={() => setLocalFontScale("100")}
              className={`text-[11px] font-mono cursor-pointer transition-colors ${
                localFontScale === "100"
                  ? "text-accent-text"
                  : "text-text-3 hover:text-text-1"
              }`}
            >
              {localFontScale}%
            </button>
            <span className="text-[11px] text-text-3">125%</span>
          </div>
        </div>
      </Card>

      <div className="flex flex-wrap gap-2">
        <Button
          onClick={save}
          loading={saving}
          icon={<Save className="w-4 h-4" />}
        >
          Save Design
        </Button>
        <Button
          variant="secondary"
          onClick={reset}
          icon={<RotateCcw className="w-4 h-4" />}
        >
          Reset
        </Button>
        <Button
          variant="secondary"
          onClick={() => setDesignSystemOpen(true)}
          icon={<SwatchBook className="w-4 h-4" />}
        >
          Design System
        </Button>
      </div>

      <DesignSystemModal
        open={designSystemOpen}
        onClose={() => setDesignSystemOpen(false)}
      />
    </div>
  );
}

function SecurityTab() {
  const { toast } = useToast();
  const [sessionTimeout, setSessionTimeout] = useState("24");
  const [ipAllowlist, setIpAllowlist] = useState(false);
  const [saving, setSaving] = useState(false);

  useEffect(() => {
    api
      .get<{ session_timeout_hours: number; ip_allowlist_enabled: boolean }>(
        "/settings/security",
      )
      .then((data) => {
        setSessionTimeout(String(data.session_timeout_hours || 24));
        setIpAllowlist(data.ip_allowlist_enabled || false);
      })
      .catch(() => {});
  }, []);

  const save = async () => {
    setSaving(true);
    try {
      await api.put("/settings/security", {
        session_timeout_hours: parseInt(sessionTimeout, 10),
        ip_allowlist_enabled: ipAllowlist,
      });
      toast("success", "Security settings saved");
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to save settings",
      );
    } finally {
      setSaving(false);
    }
  };

  return (
    <Card>
      <h3 className="text-sm font-semibold text-text-1 mb-4">Security</h3>
      <div className="space-y-4 max-w-sm">
        <Input
          label="Session Timeout (hours)"
          type="number"
          value={sessionTimeout}
          onChange={(e) => setSessionTimeout(e.target.value)}
        />
        <div className="flex items-center justify-between p-4 rounded-lg bg-surface-2">
          <div>
            <p className="text-sm font-medium text-text-1">IP Allowlist</p>
            <p className="text-xs text-text-3">
              Restrict access to specific IPs
            </p>
          </div>
          <Toggle
            enabled={ipAllowlist}
            onChange={setIpAllowlist}
            label="IP allowlist"
          />
        </div>
        <Button
          onClick={save}
          loading={saving}
          icon={<Save className="w-4 h-4" />}
        >
          Save Changes
        </Button>
      </div>
    </Card>
  );
}

function SystemTab() {
  const { toast } = useToast();
  const [info, setInfo] = useState<SystemInfo | null>(null);

  useEffect(() => {
    api
      .get<SystemInfo>("/system/info")
      .then(setInfo)
      .catch(() => {});
  }, []);

  const exportData = async () => {
    try {
      const blob = await fetch("/api/v1/system/export", {
        credentials: "same-origin",
      }).then((r) => r.blob());
      const url = URL.createObjectURL(blob);
      const a = document.createElement("a");
      a.href = url;
      a.download = `openpaw-backup-${new Date().toISOString().slice(0, 10)}.json`;
      a.click();
      URL.revokeObjectURL(url);
      toast("success", "Data exported");
    } catch (e) {
      console.warn("exportData failed:", e);
      toast("error", "Failed to export data");
    }
  };

  return (
    <div className="space-y-4">
      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-4">
          System Information
        </h3>
        {info ? (
          <div className="space-y-4">
            <div className="grid grid-cols-1 sm:grid-cols-2 gap-4">
              {[
                { label: "Version", value: info.version, icon: Info },
                { label: "Go Version", value: info.go_version, icon: Server },
                {
                  label: "Platform",
                  value: `${info.os}/${info.arch}`,
                  icon: HardDrive,
                },
                { label: "Uptime", value: info.uptime, icon: Server },
                { label: "Database Size", value: info.db_size, icon: Database },
                {
                  label: "Tools",
                  value: String(info.tool_count),
                  icon: Settings2,
                },
              ].map((item) => (
                <div
                  key={item.label}
                  className="flex items-center gap-3 p-3 rounded-lg bg-surface-2"
                >
                  <item.icon className="w-4 h-4 text-text-3" />
                  <div>
                    <p className="text-xs text-text-3">{item.label}</p>
                    <p className="text-sm font-medium text-text-1">
                      {item.value}
                    </p>
                  </div>
                </div>
              ))}
            </div>
          </div>
        ) : (
          <LoadingSpinner message="Loading system info..." />
        )}
      </Card>

      <Card>
        <h3 className="text-sm font-semibold text-text-1 mb-4">
          Data Management
        </h3>
        <div className="flex flex-wrap gap-2 md:gap-3">
          <Button
            variant="secondary"
            onClick={exportData}
            icon={<Download className="w-4 h-4" />}
          >
            Export Data
          </Button>
          <Button variant="secondary" icon={<Upload className="w-4 h-4" />}>
            Import Data
          </Button>
        </div>
      </Card>
    </div>
  );
}

function AboutTab() {
  const [version, setVersion] = useState("");

  useEffect(() => {
    api
      .get<SystemInfo>("/system/info")
      .then((data) => setVersion(data.version))
      .catch(() => {});
  }, []);

  return (
    <div className="relative rounded-xl overflow-hidden">
      <img
        src="/preset-bg/bg-7.webp"
        alt=""
        className="absolute inset-0 w-full h-full object-cover"
      />
      <div className="absolute inset-0 bg-gradient-to-t from-black/90 via-black/60 to-black/30" />
      <div className="relative text-center py-16 px-6">
        <img
          src="/icon.webp"
          alt="OpenPaw"
          className="w-24 h-24 mx-auto mb-6 drop-shadow-[0_0_30px_rgba(232,75,165,0.5)]"
        />
        <h2 className="text-3xl font-bold text-white">OpenPaw</h2>
        <p className="text-lg text-white/70 mt-2">Your AI-powered assistant</p>
        {version && (
          <p className="text-xs text-white/40 mt-4 font-mono">{version}</p>
        )}
        <div className="mt-8 pt-4 border-t border-white/10">
          <p className="text-sm text-white/50">Created by Wynter Jones</p>
        </div>
      </div>
    </div>
  );
}

function DangerTab() {
  const { toast } = useToast();
  const { logout } = useAuth();
  const [deleteDataConfirm, setDeleteDataConfirm] = useState("");
  const [deleteAccountConfirm, setDeleteAccountConfirm] = useState("");
  const [deletingData, setDeletingData] = useState(false);
  const [deletingAccount, setDeletingAccount] = useState(false);
  const [showDeleteData, setShowDeleteData] = useState(false);
  const [showDeleteAccount, setShowDeleteAccount] = useState(false);

  const handleDeleteAllData = async () => {
    if (deleteDataConfirm !== "DELETE") return;
    setDeletingData(true);
    try {
      await api.delete("/system/data");
      toast("success", "All data has been deleted");
      setShowDeleteData(false);
      setDeleteDataConfirm("");
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to delete data",
      );
    } finally {
      setDeletingData(false);
    }
  };

  const handleDeleteAccount = async () => {
    if (deleteAccountConfirm !== "DELETE") return;
    setDeletingAccount(true);
    try {
      await api.delete("/auth/account");
      toast("success", "Account deleted");
      logout();
    } catch (err) {
      toast(
        "error",
        err instanceof Error ? err.message : "Failed to delete account",
      );
    } finally {
      setDeletingAccount(false);
    }
  };

  return (
    <div className="space-y-6">
      <div className="flex items-center gap-3 p-4 rounded-lg bg-red-500/5 border border-red-500/20">
        <AlertTriangle className="w-5 h-5 text-red-400 flex-shrink-0" />
        <p className="text-sm text-red-400">
          Actions on this page are permanent and cannot be undone. Please
          proceed with caution.
        </p>
      </div>

      <Card>
        <div className="flex items-start gap-3">
          <div className="w-10 h-10 rounded-lg bg-red-500/10 flex items-center justify-center flex-shrink-0 mt-0.5">
            <Trash2 className="w-5 h-5 text-red-400" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-text-1">
              Delete All Data
            </h3>
            <p className="text-xs text-text-3 mt-1">
              Remove all tools, schedules, logs, secrets, chat history, and
              agent roles. Your account will remain intact but all application
              data will be permanently erased.
            </p>
            <Button
              variant="danger"
              size="sm"
              onClick={() => setShowDeleteData(true)}
              className="mt-3"
            >
              Delete Data
            </Button>
          </div>
        </div>
      </Card>

      <Card>
        <div className="flex items-start gap-3">
          <div className="w-10 h-10 rounded-lg bg-red-500/10 flex items-center justify-center flex-shrink-0 mt-0.5">
            <Skull className="w-5 h-5 text-red-400" />
          </div>
          <div>
            <h3 className="text-sm font-semibold text-text-1">
              Delete Account
            </h3>
            <p className="text-xs text-text-3 mt-1">
              Permanently delete your account, all data, and reset the entire
              OpenPaw instance. You will be logged out and redirected to the
              setup page.
            </p>
            <Button
              variant="danger"
              size="sm"
              onClick={() => setShowDeleteAccount(true)}
              className="mt-3"
            >
              Delete Account
            </Button>
          </div>
        </div>
      </Card>

      <Modal
        open={showDeleteData}
        onClose={() => {
          setShowDeleteData(false);
          setDeleteDataConfirm("");
        }}
        title="Delete All Data"
        size="sm"
      >
        <div className="space-y-4">
          <div className="flex items-center gap-3 p-3 rounded-lg bg-red-500/10 border border-red-500/20">
            <AlertTriangle className="w-5 h-5 text-red-400 flex-shrink-0" />
            <p className="text-sm text-red-300">
              This will permanently delete all your tools, schedules, logs,
              secrets, chat history, and agent configurations.
            </p>
          </div>
          <div>
            <p className="text-xs font-medium text-text-1 mb-1.5">
              Type{" "}
              <span className="font-mono font-bold text-red-400">DELETE</span>{" "}
              to confirm
            </p>
            <Input
              value={deleteDataConfirm}
              onChange={(e) => setDeleteDataConfirm(e.target.value)}
              placeholder="DELETE"
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              onClick={() => {
                setShowDeleteData(false);
                setDeleteDataConfirm("");
              }}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              onClick={handleDeleteAllData}
              loading={deletingData}
              disabled={deleteDataConfirm !== "DELETE"}
              icon={<Trash2 className="w-4 h-4" />}
            >
              Delete All Data
            </Button>
          </div>
        </div>
      </Modal>

      <Modal
        open={showDeleteAccount}
        onClose={() => {
          setShowDeleteAccount(false);
          setDeleteAccountConfirm("");
        }}
        title="Delete Account"
        size="sm"
      >
        <div className="space-y-4">
          <div className="flex items-center gap-3 p-3 rounded-lg bg-red-500/10 border border-red-500/20">
            <Skull className="w-5 h-5 text-red-400 flex-shrink-0" />
            <p className="text-sm text-red-300">
              This will permanently delete your account and all associated data.
              The instance will be completely reset.
            </p>
          </div>
          <div>
            <p className="text-xs font-medium text-text-1 mb-1.5">
              Type{" "}
              <span className="font-mono font-bold text-red-400">DELETE</span>{" "}
              to confirm
            </p>
            <Input
              value={deleteAccountConfirm}
              onChange={(e) => setDeleteAccountConfirm(e.target.value)}
              placeholder="DELETE"
            />
          </div>
          <div className="flex justify-end gap-2">
            <Button
              variant="ghost"
              onClick={() => {
                setShowDeleteAccount(false);
                setDeleteAccountConfirm("");
              }}
            >
              Cancel
            </Button>
            <Button
              variant="danger"
              onClick={handleDeleteAccount}
              loading={deletingAccount}
              disabled={deleteAccountConfirm !== "DELETE"}
              icon={<Skull className="w-4 h-4" />}
            >
              Delete Account
            </Button>
          </div>
        </div>
      </Modal>
    </div>
  );
}

export function Settings() {
  const [activeTab, setActiveTab] = useState<TabId>("profile");

  const tabContent: Record<TabId, React.ReactNode> = {
    profile: <ProfileTab />,
    general: <GeneralTab />,
    notifications: <NotificationsTab />,
    network: <NetworkTab />,
    models: <ModelsTab />,
    design: <DesignTab />,
    security: <SecurityTab />,
    system: <SystemTab />,
    about: <AboutTab />,
    danger: <DangerTab />,
  };

  return (
    <div className="flex flex-col h-full">
      <Header title="Settings" />

      <div className="flex-1 overflow-y-auto p-4 md:p-6">
        <div className="flex flex-col sm:flex-row gap-6 max-w-4xl">
          <div className="sm:w-52 flex-shrink-0">
            <nav
              role="tablist"
              aria-orientation="vertical"
              className="flex sm:flex-col gap-0.5 overflow-x-auto sm:overflow-x-visible bg-surface-1 border border-border-0 border-b-border-0 rounded-xl p-2 sm:sticky sm:-top-[25px] shadow-sm"
            >
              {TABS.map((tab, i) => {
                const isDanger = tab.id === "danger";
                const showSeparator = isDanger && i > 0;
                return (
                  <div key={tab.id}>
                    {showSeparator && (
                      <div className="hidden sm:block mx-2 my-1.5 border-b border-border-0" />
                    )}
                    <button
                      role="tab"
                      id={`tab-${tab.id}`}
                      aria-selected={activeTab === tab.id}
                      aria-controls={`tabpanel-${tab.id}`}
                      onClick={() => setActiveTab(tab.id)}
                      className={`w-full flex items-center gap-2.5 px-3 py-2 rounded-lg text-sm font-medium whitespace-nowrap transition-colors cursor-pointer ${
                        isDanger
                          ? activeTab === tab.id
                            ? "bg-red-500/15 text-red-400"
                            : "text-red-400/60 hover:text-red-400 hover:bg-red-500/10"
                          : activeTab === tab.id
                            ? "bg-accent-muted text-accent-text"
                            : "text-text-2 hover:text-text-1 hover:bg-surface-2"
                      }`}
                    >
                      <tab.icon className="w-4 h-4 flex-shrink-0" />
                      {tab.label}
                    </button>
                  </div>
                );
              })}
            </nav>
          </div>

          <div
            role="tabpanel"
            id={`tabpanel-${activeTab}`}
            aria-labelledby={`tab-${activeTab}`}
            className="flex-1 min-w-0"
          >
            {tabContent[activeTab]}
          </div>
        </div>
      </div>
    </div>
  );
}
