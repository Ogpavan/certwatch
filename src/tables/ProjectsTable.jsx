import { Table, Tag } from "antd";

export default function ProjectsTable({ data, onProjectClick }) {
  const columns = [
    {
      title: "Project Name",
      dataIndex: "name",
      key: "name",
      render: (value, record) => (
        <span
          className="project-link"
          role="button"
          tabIndex={0}
          onClick={(event) => {
            event.stopPropagation();
            onProjectClick?.(record);
          }}
          onKeyDown={(event) => {
            if (event.key === "Enter") {
              event.preventDefault();
              onProjectClick?.(record);
            }
          }}
        >
          {value}
        </span>
      )
    },
    {
      title: "Total Domains",
      dataIndex: "totalDomains",
      key: "totalDomains",
      sorter: (a, b) => a.totalDomains - b.totalDomains
    },
    {
      title: "Critical Alerts",
      dataIndex: "criticalAlerts",
      key: "criticalAlerts",
      sorter: (a, b) => a.criticalAlerts - b.criticalAlerts,
      render: (value) => (
        <Tag color={value > 0 ? "#EF4444" : "#22C55E"}>{value}</Tag>
      )
    },
    { title: "Last Scan", dataIndex: "lastScan", key: "lastScan" }
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
