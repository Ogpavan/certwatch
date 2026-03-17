const API_BASE = import.meta.env.VITE_API_BASE || "";

async function request(path, options = {}) {
  const headers = new Headers(options.headers || {});
  if (options.body && !headers.has("Content-Type")) {
    headers.set("Content-Type", "application/json");
  }

  const response = await fetch(`${API_BASE}${path}`, {
    ...options,
    headers
  });

  const contentType = response.headers.get("content-type") || "";
  let payload = null;
  if (contentType.includes("application/json")) {
    payload = await response.json();
  } else {
    payload = await response.text();
  }

  if (!response.ok) {
    const message = payload?.error || response.statusText;
    throw new Error(message);
  }

  return payload;
}

export const api = {
  getProjects: () => request("/projects"),
  createProject: (data) => request("/projects", { method: "POST", body: JSON.stringify(data) }),
  deleteProject: (id) => request(`/projects/${id}`, { method: "DELETE" }),
  getDomains: () => request("/domains"),
  createDomain: (data) => request("/domains", { method: "POST", body: JSON.stringify(data) }),
  deleteDomain: (id) => request(`/domains/${id}`, { method: "DELETE" }),
  getDomain: (id) => request(`/domains/${id}`),
  getDomainHistory: (id) => request(`/domains/${id}/history`),
  getAlerts: () => request("/alerts"),
  resolveAlert: (id) => request(`/alerts/${id}/resolve`, { method: "POST" }),
  scanNow: () => request("/scan-now", { method: "POST" }),
  getScanProgress: (runId) => request(`/scan-progress/${runId}`),
  getNotificationSettings: () => request("/settings/notifications"),
  updateNotificationSettings: (data) =>
    request("/settings/notifications", { method: "PUT", body: JSON.stringify(data) }),
  getLogs: (limit = 200) => request(`/logs?limit=${limit}`)
};

function normalizeDate(value) {
  if (!value) return null;
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return null;
  if (date.getFullYear() <= 1) return null;
  return date;
}

export function formatDate(value) {
  const date = normalizeDate(value);
  if (!date) return "-";
  return date
    .toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" })
    .replace(/ /g, "-");
}

export function formatDateTime(value) {
  const date = normalizeDate(value);
  if (!date) return "-";
  return date
    .toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" })
    .replace(/ /g, "-")
    .concat(" ", date.toLocaleTimeString("en-GB", { hour12: false }));
}

export function daysUntil(value) {
  const date = normalizeDate(value);
  if (!date) return null;
  return Math.ceil((date.getTime() - Date.now()) / (1000 * 60 * 60 * 24));
}

export function mapStatus(status) {
  if (!status) return "Unknown";
  if (status === "ExpiringSoon") return "Expiring";
  return status;
}

export async function getDomainsWithLatestScan() {
  const domains = await api.getDomains();
  if (!Array.isArray(domains) || domains.length === 0) {
    return [];
  }

  const detailed = await Promise.all(
    domains.map(async (domain) => {
      try {
        const info = await api.getDomain(domain.id);
        return { ...domain, ...info };
      } catch {
        return domain;
      }
    })
  );

  return detailed.map((domain) => {
    const latest = domain.latest_scan || {};
    const sslExpiry = latest.ssl_expiry || null;
    const domainExpiry = latest.domain_expiry || null;
    const sslDate = normalizeDate(sslExpiry);
    const domainDate = normalizeDate(domainExpiry);
    const daysLeft = daysUntil(sslDate);
    const lastScanRaw = latest.checked_at || null;
    const issuerFriendly = latest.issuer || "-";
    const issuerDN = latest.issuer_dn || latest.issuer || "-";

    return {
      ...domain,
      status: mapStatus(latest.status),
      sslExpiryRaw: sslDate,
      sslExpiry: formatDate(sslDate),
      daysLeft: daysLeft ?? "-",
      domainExpiryRaw: domainDate,
      domainExpiry: formatDate(domainDate),
      issuer: issuerFriendly,
      issuerDN,
      tls: latest.tls_version || "-",
      ip: latest.ip_address || "-",
      lastScan: formatDateTime(latest.checked_at),
      lastScanRaw
    };
  });
}
