import { DeleteOutlined } from "@ant-design/icons";
import { Drawer, Descriptions, Divider, Table, Tag, Spin, Button, Popconfirm, Tooltip } from "antd";
import { formatDate, mapStatus } from "../services/api.js";

const statusColor = (status) => {
  if (status === "Healthy") return "#22C55E";
  if (status === "Expiring") return "#F59E0B";
  if (status === "Critical") return "#EF4444";
  return "#64748B";
};

const historyColumns = [
  { title: "Time", dataIndex: "checked_at", key: "checked_at" },
  {
    title: "Status",
    dataIndex: "status",
    key: "status",
    render: (value) => {
      const normalized = mapStatus(value);
      return <Tag color={statusColor(normalized)}>{normalized || "-"}</Tag>;
    }
  },
  { title: "TLS", dataIndex: "tls_version", key: "tls_version" },
  { title: "Issuer", dataIndex: "issuer", key: "issuer" }
];

export default function DomainDetailDrawer({ open, onClose, loading, details, history, onDelete, deleting }) {
  const latest = details?.latest_scan || {};
  const normalizedStatus = mapStatus(latest.status);
  const servers = String(latest.nameservers || "")
    .split(",")
    .map((item) => item.trim())
    .filter(Boolean);

  return (
    <Drawer
      title={details?.domain || "Domain details"}
      placement="right"
      width={420}
      open={open}
      onClose={onClose}
      footer={
        <Popconfirm
          title={`Delete ${details?.domain || "this domain"}?`}
          onConfirm={() => onDelete?.(details)}
          okText="Delete"
          okType="danger"
          placement="top"
        >
          <Button
            type="primary"
            danger
            icon={<DeleteOutlined />}
            block
            loading={deleting}
            disabled={deleting || !details}
          >
            Delete domain
          </Button>
        </Popconfirm>
      }
    >
      {loading ? (
        <Spin />
      ) : (
        <div>
          <Divider orientation="left">General</Divider>
          <Descriptions column={1} size="small">
            <Descriptions.Item label="Project">{details?.project_name || "-"}</Descriptions.Item>
            <Descriptions.Item label="Domain">{details?.domain || "-"}</Descriptions.Item>
            <Descriptions.Item label="Port">{details?.port || "-"}</Descriptions.Item>
            <Descriptions.Item label="Status">
              <Tag color={statusColor(normalizedStatus)}>{normalizedStatus || "-"}</Tag>
            </Descriptions.Item>
            <Descriptions.Item label="Added">{formatDate(details?.created_at)}</Descriptions.Item>
          </Descriptions>

          <Divider orientation="left">SSL Certificate</Divider>
          <Descriptions column={1} size="small">
            <Descriptions.Item label="Issuer">
              <Tooltip title={latest.issuer_dn || latest.issuer || "-"}>
                <span>{latest.issuer || "-"}</span>
              </Tooltip>
            </Descriptions.Item>
            <Descriptions.Item label="Valid From">{formatDate(latest.ssl_valid_from)}</Descriptions.Item>
            <Descriptions.Item label="Expiry Date">{formatDate(latest.ssl_expiry)}</Descriptions.Item>
            <Descriptions.Item label="Fingerprint">{latest.fingerprint || "-"}</Descriptions.Item>
          </Descriptions>

          <Divider orientation="left">TLS Information</Divider>
          <Descriptions column={1} size="small">
            <Descriptions.Item label="TLS Version">{latest.tls_version || "-"}</Descriptions.Item>
          </Descriptions>

          <Divider orientation="left">DNS</Divider>
          <Descriptions column={1} size="small">
            <Descriptions.Item label="IP Address">{latest.ip_address || "-"}</Descriptions.Item>
            <Descriptions.Item label="Nameservers">
              {servers.length
                ? servers.map((server) => (
                    <div key={server}>{server}</div>
                  ))
                : "-"}
            </Descriptions.Item>
          </Descriptions>

          <Divider orientation="left">Scan History</Divider>
          <Table
            columns={historyColumns}
            dataSource={history || []}
            pagination={false}
            size="small"
            rowKey="id"
          />
        </div>
      )}
    </Drawer>
  );
}
