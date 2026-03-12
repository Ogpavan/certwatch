import { useEffect, useMemo, useState } from "react";
import { Card, Table, Tag, Button, Space, message } from "antd";
import { api, formatDateTime } from "../services/api.js";

const levelColors = {
  INFO: "blue",
  WARN: "orange",
  ERROR: "red"
};

export default function LogsPage() {
  const [logs, setLogs] = useState([]);
  const [loading, setLoading] = useState(false);

  const loadLogs = async () => {
    setLoading(true);
    try {
      const data = await api.getLogs();
      setLogs(Array.isArray(data) ? data : []);
    } catch (err) {
      message.error(err?.message || "Failed to load logs");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadLogs();
  }, []);

  const columns = useMemo(
    () => [
      {
        title: "Time",
        dataIndex: "created_at",
        key: "created_at",
        width: 180,
        render: (value) => formatDateTime(value)
      },
      {
        title: "Level",
        dataIndex: "level",
        key: "level",
        width: 90,
        render: (value) => <Tag color={levelColors[value] || "default"}>{value}</Tag>
      },
      {
        title: "Domain",
        dataIndex: "domain",
        key: "domain",
        width: 220,
        render: (value) => value || "-"
      },
      {
        title: "Message",
        dataIndex: "message",
        key: "message"
      }
    ],
    []
  );

  return (
    <div>
      <div className="page-header">
        <div className="section-title">Logs</div>
      </div>

      <Card className="card-surface">
        <Space style={{ marginBottom: 16 }}>
          <Button onClick={loadLogs} loading={loading}>
            Refresh
          </Button>
        </Space>
        <Table
          rowKey={(record) => record.id}
          dataSource={logs}
          columns={columns}
          loading={loading}
          pagination={{ pageSize: 20 }}
        />
      </Card>
    </div>
  );
}
