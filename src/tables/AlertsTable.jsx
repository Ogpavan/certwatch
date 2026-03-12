import { Table, Tag, Space, Button } from "antd";

export default function AlertsTable({ data, onResolve }) {
  const columns = [
    { title: "Domain", dataIndex: "domain", key: "domain" },
    { title: "Alert Message", dataIndex: "message", key: "message" },
    {
      title: "Severity",
      dataIndex: "severity",
      key: "severity",
      render: (value) => {
        const color = value === "Critical" ? "#EF4444" : value === "Warning" ? "#F59E0B" : "#22C55E";
        return <Tag color={color}>{value}</Tag>;
      }
    },
    { title: "Timestamp", dataIndex: "timestamp", key: "timestamp" },
    { title: "Status", dataIndex: "status", key: "status" },
    {
      title: "Actions",
      key: "actions",
      render: (_, record) => (
        <Space>
          <Button size="small">Acknowledge</Button>
          <Button size="small" type="primary" onClick={() => onResolve?.(record)}>
            Resolve
          </Button>
        </Space>
      )
    }
  ];

  return (
    <Table
      columns={columns}
      dataSource={data}
      pagination={{ pageSize: 6 }}
      className="table-card"
      rowKey="id"
    />
  );
}
