import { useEffect, useMemo, useRef, useState } from "react";
import { Button, Card, Statistic, Spin, message } from "antd";
import { SyncOutlined } from "@ant-design/icons";
import { Pie } from "@ant-design/charts";
import {
  api,
  daysUntil,
  getDomainsWithLatestScan
} from "../services/api.js";

const DEFAULT_SCHEDULE = "03:00";

const buckets = [
  { label: "0-7 days", min: 0, max: 7 },
  { label: "8-14 days", min: 8, max: 14 },
  { label: "15-30 days", min: 15, max: 30 },
  { label: "31-60 days", min: 31, max: 60 },
  { label: "60+ days", min: 61, max: 10000 }
];

function buildDistribution(items, accessor) {
  const counts = buckets.map((bucket) => ({ type: bucket.label, value: 0 }));
  items.forEach((item) => {
    const value = accessor(item);
    if (value === null || value === undefined) return;
    const dayValue = typeof value === "number" ? value : daysUntil(value);
    if (dayValue === null) return;
    const bucket = buckets.find((b) => dayValue >= b.min && dayValue <= b.max);
    if (bucket) {
      const index = buckets.findIndex((b) => b.label === bucket.label);
      counts[index].value += 1;
    }
  });

  const total = counts.reduce((sum, c) => sum + c.value, 0);
  if (total === 0) {
    return [{ type: "No data", value: 1 }];
  }
  return counts;
}

function parseSchedule(value) {
  if (typeof value !== "string" || value.trim() === "") {
    return { hour: 3, minute: 0 };
  }
  const parts = value.split(":");
  if (parts.length !== 2) {
    return { hour: 3, minute: 0 };
  }
  const hour = Number(parts[0]);
  const minute = Number(parts[1]);
  if (!Number.isInteger(hour) || hour < 0 || hour > 23) {
    return { hour: 3, minute: 0 };
  }
  if (!Number.isInteger(minute) || minute < 0 || minute > 59) {
    return { hour: 3, minute: 0 };
  }
  return { hour, minute };
}

function sameDay(a, b) {
  return a.getFullYear() === b.getFullYear() && a.getMonth() === b.getMonth() && a.getDate() === b.getDate();
}

function formatCountdown(ms) {
  if (ms <= 0) return "00:00:00";
  const totalSeconds = Math.floor(ms / 1000);
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;
  const pad = (value) => String(value).padStart(2, "0");
  return `${pad(hours)}:${pad(minutes)}:${pad(seconds)}`;
}

function formatScheduleLabel(date, now) {
  if (!date) return "-";
  const time = date.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit", hour12: true });
  if (sameDay(date, now)) {
    return `Today at ${time}`;
  }
  const tomorrow = new Date(now);
  tomorrow.setDate(now.getDate() + 1);
  if (sameDay(date, tomorrow)) {
    return `Tomorrow at ${time}`;
  }
  return formatDateTime12(date);
}

function computeNextScan(now, scheduleTime, lastRunRaw) {
  const { hour, minute } = parseSchedule(scheduleTime);
  const target = new Date(now);
  target.setHours(hour, minute, 0, 0);
  const lastRun = lastRunRaw ? new Date(lastRunRaw) : null;
  const ranAfterTargetToday = lastRun && sameDay(lastRun, now) && lastRun.getTime() >= target.getTime();

  if (ranAfterTargetToday) {
    const next = new Date(target);
    next.setDate(next.getDate() + 1);
    return { nextAt: next, scheduledAt: next, dueNow: false };
  }
  if (now < target) {
    return { nextAt: target, scheduledAt: target, dueNow: false };
  }
  return { nextAt: now, scheduledAt: target, dueNow: true };
}

function formatDateTime12(value) {
  if (!value) return "-";
  const date = new Date(value);
  if (Number.isNaN(date.getTime())) return "-";
  const datePart = date
    .toLocaleDateString("en-GB", { day: "2-digit", month: "short", year: "numeric" })
    .replace(/ /g, "-");
  const timePart = date.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit", hour12: true });
  return `${datePart} ${timePart}`;
}

function formatScheduleTime12(scheduleTime, now) {
  const { hour, minute } = parseSchedule(scheduleTime);
  const date = new Date(now);
  date.setHours(hour, minute, 0, 0);
  return date.toLocaleTimeString("en-US", { hour: "numeric", minute: "2-digit", hour12: true });
}

const pieConfig = (data, title) => ({
  data,
  angleField: "value",
  colorField: "type",
  radius: 0.9,
  innerRadius: 0.55,
  legend: { position: "bottom" },
  label: { type: "inner", offset: "-30%", content: "{value}" },
  color: ["#EF4444", "#F59E0B", "#38BDF8", "#22C55E", "#64748B"],
  height: 260,
  statistic: {
    title: { formatter: () => title },
    content: { formatter: () => "Distribution" }
  }
});

export default function DashboardPage() {
  const [stats, setStats] = useState([]);
  const [sslData, setSslData] = useState([]);
  const [domainData, setDomainData] = useState([]);
  const [loading, setLoading] = useState(true);
  const [scanLoading, setScanLoading] = useState(false);
  const [scanEvents, setScanEvents] = useState([]);
  const [scanRunId, setScanRunId] = useState(null);
  const [scanRunning, setScanRunning] = useState(false);
  const [scheduleTime, setScheduleTime] = useState(DEFAULT_SCHEDULE);
  const [lastScheduleRun, setLastScheduleRun] = useState(null);
  const [scheduleLoading, setScheduleLoading] = useState(true);
  const [tick, setTick] = useState(Date.now());
  const pollRef = useRef(null);

  const loadDashboard = async () => {
    try {
      setLoading(true);
      const [projects, alerts, domains] = await Promise.all([
        api.getProjects(),
        api.getAlerts(),
        getDomainsWithLatestScan()
      ]);

        const criticalAlerts = (alerts || []).filter((alert) =>
          String(alert.severity).toLowerCase() === "critical" && !alert.resolved
        ).length;

        const sslExpiringSoon = domains.filter((domain) => {
          if (!domain.sslExpiryRaw) return false;
          const days = daysUntil(domain.sslExpiryRaw);
          return days !== null && days < 30;
        }).length;

        const domainExpiringSoon = domains.filter((domain) => {
          if (!domain.domainExpiryRaw) return false;
          const days = daysUntil(domain.domainExpiryRaw);
          return days !== null && days < 30;
        }).length;

        const nextStats = [
          { title: "Total Projects", value: projects.length || 0 },
          { title: "Total Domains", value: domains.length || 0 },
          { title: "SSL Expiring Soon", value: sslExpiringSoon },
          { title: "Domain Expiring Soon", value: domainExpiringSoon },
          { title: "Critical Alerts", value: criticalAlerts }
        ];

        const sslDistribution = buildDistribution(domains, (item) => item.sslExpiryRaw);
        const domainDistribution = buildDistribution(domains, (item) => item.domainExpiryRaw);

      setStats(nextStats);
      setSslData(sslDistribution);
      setDomainData(domainDistribution);
    } catch (err) {
      message.error(err.message || "Failed to load dashboard data");
    } finally {
      setLoading(false);
    }
  };

  const loadSchedule = async () => {
    try {
      setScheduleLoading(true);
      const data = await api.getNotificationSettings();
      const scheduled = typeof data?.schedule_time === "string" && data.schedule_time ? data.schedule_time : DEFAULT_SCHEDULE;
      setScheduleTime(scheduled);
      setLastScheduleRun(data?.last_scanned_at ?? null);
    } catch (err) {
      message.error(err.message || "Failed to load scan schedule");
    } finally {
      setScheduleLoading(false);
    }
  };

  useEffect(() => {
    loadDashboard();
    loadSchedule();
  }, []);

  useEffect(() => () => {
    if (pollRef.current) {
      clearTimeout(pollRef.current);
    }
  }, []);

  const pollScan = async (runId) => {
    try {
      const result = await api.getScanProgress(runId);
      setScanEvents(result.events || []);
      if (result.done) {
        if (pollRef.current) {
          clearTimeout(pollRef.current);
          pollRef.current = null;
        }
        setScanRunning(false);
        setScanRunId(null);
        return;
      }
      pollRef.current = window.setTimeout(() => pollScan(runId), 1200);
    } catch (err) {
      setScanRunning(false);
      setScanRunId(null);
      message.error(err.message || "Failed to load scan progress");
    }
  };

  const handleScanNow = async () => {
    if (scanRunning) return;
    try {
      setScanLoading(true);
      setScanEvents([]);
      const payload = await api.scanNow();
      const runId = payload?.run_id;
      if (!runId) {
        throw new Error("run id missing");
      }
      setScanRunId(runId);
      setScanRunning(true);
      message.success("Scan scheduled");
      await pollScan(runId);
      await loadDashboard();
      await loadSchedule();
    } catch (err) {
      message.error(err.message || "Failed to trigger scan");
    } finally {
      setScanLoading(false);
    }
  };

  const statsContent = useMemo(() => (
    <div className="stat-grid">
      {stats.map((item, index) => (
        <Card key={item.title} className={`card-surface reveal stagger-${index + 1}`}>
          <Statistic title={item.title} value={item.value} />
        </Card>
      ))}
    </div>
  ), [stats]);

  useEffect(() => {
    const id = window.setInterval(() => {
      setTick(Date.now());
    }, 1000);
    return () => clearInterval(id);
  }, []);

  const nextScanInfo = useMemo(() => {
    if (scheduleLoading) {
      return {
        countdown: "Loading...",
        dueNow: false,
        scheduledLabel: "-",
        lastRunLabel: "-",
        scheduleTimeLabel: "-"
      };
    }
    const now = new Date(tick);
    const { nextAt, scheduledAt, dueNow } = computeNextScan(now, scheduleTime, lastScheduleRun);
    const remaining = nextAt.getTime() - now.getTime();
    return {
      countdown: dueNow ? "Due now" : formatCountdown(remaining),
      dueNow,
      scheduledLabel: formatScheduleLabel(scheduledAt, now),
      lastRunLabel: lastScheduleRun ? formatDateTime12(lastScheduleRun) : "Not run yet",
      scheduleTimeLabel: formatScheduleTime12(scheduleTime, now)
    };
  }, [tick, scheduleLoading, scheduleTime, lastScheduleRun]);

  return (
    <div>
      <div className="page-header">
        <div className="section-title">Overview</div>
        <Button
          type="ghost"
          icon={<SyncOutlined />}
          onClick={handleScanNow}
          loading={scanLoading}
          disabled={scanRunning}
        >
          Scan now
        </Button>
      </div>
      <Card className="card-surface next-scan-card">
        <div className="section-title">Next scheduled scan</div>
        <div className="next-scan-body">
          <div className="next-scan-countdown">{nextScanInfo.countdown}</div>
          <div className="next-scan-meta">
            <div>Scheduled for {nextScanInfo.scheduledLabel} (daily at {nextScanInfo.scheduleTimeLabel}).</div>
            <div>Last run: {nextScanInfo.lastRunLabel}.</div>
          </div>
        </div>
      </Card>
      {scanRunning && (
        <div className="scan-progress-card">
          <div className="section-title" style={{ fontSize: 16 }}>
            Scan progress
          </div>
          <div className="scan-progress-list">
            {scanEvents.length === 0 && <div className="scan-progress-item">Preparing scan...</div>}
            {scanEvents.map((event, index) => (
              <div className="scan-progress-item" key={`${event.domain}-${event.timestamp}-${index}`}>
                <span className="scan-progress-domain">{event.domain || "scan"}</span>
                <span className="scan-progress-status">{event.status}</span>
                {event.message && <span className="scan-progress-message">{event.message}</span>}
              </div>
            ))}
          </div>
        </div>
      )}

      {loading ? (
        <Spin />
      ) : (
        <>
          {statsContent}
          <div className="chart-grid">
            <Card className="card-surface reveal stagger-3">
              <div className="section-title">SSL expiry distribution</div>
              <Pie {...pieConfig(sslData, "SSL Expiry")} />
            </Card>
            <Card className="card-surface reveal stagger-4">
              <div className="section-title">Domain expiry distribution</div>
              <Pie {...pieConfig(domainData, "Domain Expiry")} />
            </Card>
          </div>
        </>
      )}
    </div>
  );
}
