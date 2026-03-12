import { useMemo, useState } from "react";
import { Pagination, Select, Table, Tag } from "antd";

const statusColor = (status) => {
  if (status === "Healthy") return "#22C55E";
  if (status === "Expiring") return "#F59E0B";
  if (status === "Critical") return "#EF4444";
  return "#64748B";
};

export default function DomainsTable({ data, onDomainClick }) {
  const columns = [
    {
      title: "Domain",
      dataIndex: "domain",
      key: "domain",
      sorter: (a, b) => a.domain.localeCompare(b.domain),
      render: (value, record) => (
        <a
          className="domain-link"
          onClick={(event) => {
            event.stopPropagation();
            onDomainClick?.(record);
          }}
        >
          {value}
        </a>
      )
    },
    {
      title: "Project",
      dataIndex: "project_name",
      key: "project_name",
      sorter: (a, b) => (a.project_name || "").localeCompare(b.project_name || ""),
      render: (value) => value || "-"
    },
    {
      title: "Status",
      dataIndex: "status",
      key: "status",
      filters: [
        { text: "Healthy", value: "Healthy" },
        { text: "Expiring", value: "Expiring" },
        { text: "Critical", value: "Critical" },
        { text: "Unknown", value: "Unknown" }
      ],
      onFilter: (value, record) => record.status === value,
      render: (value) => <Tag color={statusColor(value)}>{value}</Tag>
    },
    {
      title: "SSL Expiry",
      dataIndex: "sslExpiry",
      key: "sslExpiry",
      sorter: (a, b) => new Date(a.sslExpiry) - new Date(b.sslExpiry)
    },
    {
      title: "Days Left",
      dataIndex: "daysLeft",
      key: "daysLeft",
      sorter: (a, b) => a.daysLeft - b.daysLeft
    },
    {
      title: "Domain Expiry",
      dataIndex: "domainExpiry",
      key: "domainExpiry",
      sorter: (a, b) => new Date(a.domainExpiry) - new Date(b.domainExpiry)
    },
    { title: "TLS Version", dataIndex: "tls", key: "tls" },
    { title: "IP Address", dataIndex: "ip", key: "ip" },
    { title: "Last Scan", dataIndex: "lastScan", key: "lastScan" }
  ];

  const [pageSize, setPageSize] = useState(10);
  const [page, setPage] = useState(1);

  const paginated = useMemo(() => {
    const start = (page - 1) * pageSize;
    return data.slice(start, start + pageSize);
  }, [data, pageSize, page]);

  const total = data.length;

  return (
    <div className="table-card">
      <Table
        columns={columns}
        dataSource={paginated}
        pagination={false}
        rowKey="id"
      />
      <div className="table-footer">
        <Select
          value={pageSize.toString()}
          onChange={(value) => {
            const size = Number(value);
            setPageSize(size);
            setPage(1);
          }}
          options={[
            { label: "10 / page", value: "10" },
            { label: "50 / page", value: "50" },
            { label: "100 / page", value: "100" }
          ]}
        />
        <Pagination
          current={page}
          total={total}
          pageSize={pageSize}
          showSizeChanger={false}
          onChange={(value) => setPage(value)}
          showTotal={(total, range) => `${range[0]}-${range[1]} of ${total}`}
        />
      </div>
    </div>
  );
}
