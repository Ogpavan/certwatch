import { useMemo, useState, useEffect } from "react";
import { Button, Input, message, Select, Spin, Tag } from "antd";
import { DownloadOutlined, PlusOutlined } from "@ant-design/icons";
import DomainsTable from "../tables/DomainsTable.jsx";
import AddDomainModal from "../forms/AddDomainModal.jsx";
import DomainDetailDrawer from "../components/DomainDetailDrawer.jsx";
import { api, getDomainsWithLatestScan, formatDateTime } from "../services/api.js";
import { useSearchParams } from "react-router-dom";

export default function DomainsPage() {
  const [domains, setDomains] = useState([]);
  const [projects, setProjects] = useState([]);
  const [selected, setSelected] = useState(null);
  const [history, setHistory] = useState([]);
  const [openDrawer, setOpenDrawer] = useState(false);
  const [openModal, setOpenModal] = useState(false);
  const [search, setSearch] = useState("");
  const [loading, setLoading] = useState(true);
  const [detailLoading, setDetailLoading] = useState(false);
  const [deletingDomain, setDeletingDomain] = useState(false);
  const [searchParams, setSearchParams] = useSearchParams();
  const [projectFilter, setProjectFilter] = useState(null);
  const [projectFilterName, setProjectFilterName] = useState("");

  const loadDomains = async () => {
    try {
      setLoading(true);
      const [domainData, projectData] = await Promise.all([
        getDomainsWithLatestScan(),
        api.getProjects()
      ]);
      setProjects(projectData || []);
      const projectNameMap = new Map(
        (projectData || []).map((project) => [project.id, project.name])
      );
      const normalizedDomains = (domainData || []).map((domain) => ({
        ...domain,
        project_name: domain.project_name || projectNameMap.get(domain.project_id) || "-"
      }));
      setDomains(normalizedDomains);
    } catch (err) {
      message.error(err.message || "Failed to load domains");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadDomains();
  }, []);

  useEffect(() => {
    const idParam = searchParams.get("projectId");
    const parsed = idParam ? Number(idParam) : null;
    if (idParam && Number.isNaN(parsed)) {
      setProjectFilter(null);
      setProjectFilterName("");
      return;
    }
    setProjectFilter(parsed || null);
    const fallbackName =
      parsed && projects.length > 0
        ? projects.find((project) => project.id === parsed)?.name
        : "";
    setProjectFilterName(searchParams.get("projectName") || fallbackName || "");
  }, [searchParams, projects]);

  const filtered = useMemo(() => {
    if (!search) return domains;
    const value = search.toLowerCase();
    return domains.filter((item) => item.domain.toLowerCase().includes(value));
  }, [domains, search]);

  const projectFiltered = useMemo(() => {
    if (!projectFilter) return filtered;
    return filtered.filter((item) => item.project_id === projectFilter);
  }, [filtered, projectFilter]);

  const handleAddDomain = async (values) => {
    try {
      const bulkLines = values.bulk
        ? values.bulk
            .split(/\s+/)
            .map((line) => line.replace(/\r/g, "").trim())
            .filter((entry) => entry !== "")
        : [];

      const failures = [];
      if (bulkLines.length > 0) {
        for (const entry of bulkLines) {
          try {
            await api.createDomain({
              project_id: values.project,
              domain: entry,
              port: values.port || 443
            });
          } catch (err) {
            failures.push({ domain: entry, message: err.message });
          }
        }
      } else {
        try {
          await api.createDomain({
            project_id: values.project,
            domain: values.domain,
            port: values.port || 443
          });
        } catch (err) {
          failures.push({ domain: values.domain, message: err.message });
        }
      }

      if (failures.length) {
        const first = failures[0];
        message.error(`Failed to add ${first.domain}: ${first.message}`);
      } else {
        message.success("Domain(s) added");
      }
      setOpenModal(false);
      loadDomains();
    } catch (err) {
      message.error(err.message || "Failed to add domain");
    }
  };

  const handleRowClick = async (record) => {
    setOpenDrawer(true);
    setDetailLoading(true);
    try {
      const [details, historyData] = await Promise.all([
        api.getDomain(record.id),
        api.getDomainHistory(record.id)
      ]);
      const formattedHistory = (historyData || []).map((item) => ({
        ...item,
        checked_at: formatDateTime(item.checked_at)
      }));
      setSelected({
        ...details,
        project_name: details.project_name || record.project_name
      });
      setHistory(formattedHistory);
    } catch (err) {
      message.error(err.message || "Failed to load domain details");
    } finally {
      setDetailLoading(false);
    }
  };

  const handleDeleteDomain = async (domain) => {
    if (!domain) return;
    try {
      setDeletingDomain(true);
      await api.deleteDomain(domain.id);
      message.success(`Deleted ${domain.domain}`);
      setOpenDrawer(false);
      setSelected(null);
      loadDomains();
    } catch (err) {
      message.error(err.message || "Failed to delete domain");
    } finally {
      setDeletingDomain(false);
    }
  };

  const handleExport = () => {
    if (!projectFiltered.length) {
      message.warning("No domains to export");
      return;
    }
    const headers = [
      "Project",
      "Domain",
      "Status",
      "SSL Expiry",
      "Days Left",
      "Domain Expiry",
      "TLS Version",
      "IP Address",
      "Last Scan"
    ];
    const rows = projectFiltered.map((domain) => [
      domain.project_name || "-",
      domain.domain,
      domain.status,
      domain.sslExpiry,
      domain.daysLeft,
      domain.domainExpiry,
      domain.tls,
      domain.ip,
      domain.lastScan
    ]);

    const escape = (value) =>
      `"${String(value ?? "").replace(/"/g, '""')}"`;
    const csv = [headers, ...rows]
      .map((row) => row.map(escape).join(","))
      .join("\n");

    const blob = new Blob([csv], { type: "text/csv;charset=utf-8;" });
    const url = URL.createObjectURL(blob);
    const anchor = document.createElement("a");
    anchor.href = url;
    anchor.download = `domains-${new Date().toISOString().slice(0, 19).replace(/[:T]/g, "-")}.csv`;
    anchor.click();
    setTimeout(() => URL.revokeObjectURL(url), 1000);
  };

  return (
    <div>
      <div className="page-header">
        <div className="section-title">Domains</div>
        <div style={{ display: "flex", gap: 12, alignItems: "center" }}>
          <Select
            allowClear
            placeholder="Filter by project"
            style={{ minWidth: 160 }}
            value={projectFilter ? projectFilter.toString() : undefined}
            onChange={(value) => {
              if (!value) {
                setSearchParams({});
                return;
              }
              const id = Number(value);
              const project = projects.find((item) => item.id === id);
              const params = { projectId: value };
              if (project) {
                params.projectName = project.name;
              }
              setSearchParams(params);
            }}
            options={projects.map((project) => ({
              label: project.name,
              value: project.id.toString()
            }))}
          />
          <div className="ghost-input">
            <Input
              placeholder="Search domains"
              allowClear
              value={search}
              onChange={(event) => setSearch(event.target.value)}
            />
          </div>
          <Button icon={<DownloadOutlined />} onClick={handleExport}>
            Export CSV
          </Button>
          <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpenModal(true)}>
            Add Domain
          </Button>
        </div>
      </div>

      {projectFilterName && (
        <div style={{ marginBottom: 12 }}>
          <Tag
            closable
            onClose={() => setSearchParams({})}
            color="#3B82F6"
            style={{ cursor: "pointer" }}
          >
            Filtering by {projectFilterName}
          </Tag>
        </div>
      )}

      {loading ? (
        <Spin />
      ) : (
        <DomainsTable data={projectFiltered} onDomainClick={handleRowClick} />
      )}

      <DomainDetailDrawer
        open={openDrawer}
        onClose={() => setOpenDrawer(false)}
        loading={detailLoading}
        details={selected}
        history={history}
        onDelete={handleDeleteDomain}
        deleting={deletingDomain}
      />

      <AddDomainModal
        open={openModal}
        onCancel={() => setOpenModal(false)}
        onCreate={handleAddDomain}
        projects={projects}
      />
    </div>
  );
}
