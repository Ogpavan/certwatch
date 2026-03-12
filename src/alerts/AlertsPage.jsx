import { useEffect, useState } from "react";
import { Tabs, message, Spin } from "antd";
import AlertsTable from "../tables/AlertsTable.jsx";
import { api } from "../services/api.js";

export default function AlertsPage() {
  const [alerts, setAlerts] = useState([]);
  const [loading, setLoading] = useState(true);

  const loadAlerts = async () => {
    try {
      setLoading(true);
      const [alertsData, domains] = await Promise.all([
        api.getAlerts(),
        api.getDomains()
      ]);
      const domainMap = new Map();
      (domains || []).forEach((domain) => domainMap.set(domain.id, domain.domain));

      const mapped = (alertsData || []).map((alert) => ({
        id: alert.id,
        domain_id: alert.domain_id,
        domain: domainMap.get(alert.domain_id) || "-",
        message: alert.message,
        severity: alert.severity,
        timestamp: new Date(alert.created_at).toLocaleString(),
        status: alert.resolved ? "Resolved" : "Open"
      }));

      setAlerts(mapped);
    } catch (err) {
      message.error(err.message || "Failed to load alerts");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadAlerts();
  }, []);

  const handleResolve = async (alert) => {
    try {
      await api.resolveAlert(alert.id);
      message.success("Alert resolved");
      loadAlerts();
    } catch (err) {
      message.error(err.message || "Failed to resolve alert");
    }
  };

  const critical = alerts.filter((alert) => alert.severity === "Critical" && alert.status !== "Resolved");
  const warnings = alerts.filter((alert) => alert.severity === "Warning" && alert.status !== "Resolved");
  const resolved = alerts.filter((alert) => alert.status === "Resolved");

  return (
    <div>
      <div className="page-header">
        <div className="section-title">Alerts</div>
      </div>
      {loading ? (
        <Spin />
      ) : (
        <Tabs
          defaultActiveKey="critical"
          items={[
            {
              key: "critical",
              label: "Critical",
              children: <AlertsTable data={critical} onResolve={handleResolve} />
            },
            {
              key: "warning",
              label: "Warnings",
              children: <AlertsTable data={warnings} onResolve={handleResolve} />
            },
            {
              key: "resolved",
              label: "Resolved",
              children: <AlertsTable data={resolved} onResolve={handleResolve} />
            }
          ]}
        />
      )}
    </div>
  );
}
