import { useEffect, useState } from "react";
import { Button, message, Spin } from "antd";
import { PlusOutlined } from "@ant-design/icons";
import ProjectsTable from "../tables/ProjectsTable.jsx";
import CreateProjectModal from "../forms/CreateProjectModal.jsx";
import { api, getDomainsWithLatestScan, formatDateTime } from "../services/api.js";
import { useNavigate } from "react-router-dom";

export default function ProjectsPage() {
  const [projects, setProjects] = useState([]);
  const [open, setOpen] = useState(false);
  const [loading, setLoading] = useState(true);
  const navigate = useNavigate();

  const loadProjects = async () => {
    try {
      setLoading(true);
      const [projectsData, domains, alerts] = await Promise.all([
        api.getProjects(),
        getDomainsWithLatestScan(),
        api.getAlerts()
      ]);

      const domainsByProject = new Map();
      const lastScanByProject = new Map();
      domains.forEach((domain) => {
        const projectID = domain.project_id;
        if (!domainsByProject.has(projectID)) {
          domainsByProject.set(projectID, []);
        }
        domainsByProject.get(projectID).push(domain);
        if (domain.lastScanRaw) {
          const current = lastScanByProject.get(projectID);
          const nextDate = new Date(domain.lastScanRaw);
          if (!current || nextDate > current) {
            lastScanByProject.set(projectID, nextDate);
          }
        }
      });

      const domainProjectMap = new Map();
      domains.forEach((domain) => {
        domainProjectMap.set(domain.id, domain.project_id);
      });

      const criticalAlertsByProject = new Map();
      (alerts || []).forEach((alert) => {
        if (String(alert.severity).toLowerCase() !== "critical" || alert.resolved) return;
        const projectID = domainProjectMap.get(alert.domain_id);
        if (!projectID) return;
        criticalAlertsByProject.set(projectID, (criticalAlertsByProject.get(projectID) || 0) + 1);
      });

      const hydrated = (projectsData || []).map((project) => {
        const domainList = domainsByProject.get(project.id) || [];
        const lastScan = lastScanByProject.get(project.id);
        return {
          ...project,
          totalDomains: domainList.length,
          criticalAlerts: criticalAlertsByProject.get(project.id) || 0,
          lastScan: lastScan ? formatDateTime(lastScan) : "-"
        };
      });

      setProjects(hydrated);
    } catch (err) {
      message.error(err.message || "Failed to load projects");
    } finally {
      setLoading(false);
    }
  };

  useEffect(() => {
    loadProjects();
  }, []);

  const handleCreate = async (values) => {
    try {
      await api.createProject(values);
      message.success("Project created");
      setOpen(false);
      loadProjects();
    } catch (err) {
      message.error(err.message || "Failed to create project");
    }
  };

  return (
    <div>
      <div className="page-header">
        <div className="section-title">Projects</div>
        <Button type="primary" icon={<PlusOutlined />} onClick={() => setOpen(true)}>
          Create Project
        </Button>
      </div>

      {loading ? (
        <Spin />
      ) : (
        <ProjectsTable
          data={projects}
          onProjectClick={(project) =>
            navigate(
              `/domains?projectId=${project.id}&projectName=${encodeURIComponent(project.name)}`
            )
          }
        />
      )}

      <CreateProjectModal open={open} onCancel={() => setOpen(false)} onCreate={handleCreate} />
    </div>
  );
}
