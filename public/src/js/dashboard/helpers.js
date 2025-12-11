export function formatBytes(bytes = 0) {
    if (!Number.isFinite(bytes) || bytes <= 0) {
        return "0 B";
    }
    const units = ["B", "KB", "MB", "GB"];
    const index = Math.min(Math.floor(Math.log(bytes) / Math.log(1024)), units.length - 1);
    const converted = bytes / 1024 ** index;
    return `${converted.toFixed(index === 0 ? 0 : 1)} ${units[index]}`;
}

export function formatRelativeTime(timestamp) {
    if (!timestamp) {
        return "—";
    }
    const parsed = new Date(timestamp);
    if (Number.isNaN(parsed.getTime())) {
        return "—";
    }

    const diff = Date.now() - parsed.getTime();
    if (diff < 30 * 1000) {
        return "just now";
    }
    const minutes = Math.floor(diff / (60 * 1000));
    if (minutes < 60) {
        return `${minutes}m ago`;
    }
    const hours = Math.floor(minutes / 60);
    if (hours < 24) {
        return `${hours}h ago`;
    }
    const days = Math.floor(hours / 24);
    return `${days}d ago`;
}

export function describePowerStatus(status = "") {
    const normalized = String(status || "").toLowerCase();
    switch (normalized) {
        case "running":
            return { label: "Running", tone: "running" };
        case "stopped":
            return { label: "Stopped", tone: "stopped" };
        case "services-down":
            return { label: "Services down", tone: "degraded" };
        default:
            return { label: normalized ? normalized : "Unknown", tone: "unknown" };
    }
}
