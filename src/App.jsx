import { Navigate, Routes, Route } from "react-router-dom";
import AppLayout from "./layout/AppLayout.jsx";
import DashboardPage from "./dashboard/DashboardPage.jsx";
import ProjectsPage from "./projects/ProjectsPage.jsx";
import DomainsPage from "./domains/DomainsPage.jsx";
import AlertsPage from "./alerts/AlertsPage.jsx";
import LogsPage from "./logs/LogsPage.jsx";
import SettingsPage from "./settings/SettingsPage.jsx";

export default function App() {
  return (
    <Routes>
      <Route element={<AppLayout />}>
        <Route path="/" element={<Navigate to="/dashboard" replace />} />
        <Route path="/dashboard" element={<DashboardPage />} />
        <Route path="/projects" element={<ProjectsPage />} />
        <Route path="/domains" element={<DomainsPage />} />
        <Route path="/alerts" element={<AlertsPage />} />
        <Route path="/logs" element={<LogsPage />} />
        <Route path="/settings" element={<SettingsPage />} />
      </Route>
      <Route path="*" element={<Navigate to="/dashboard" replace />} />
    </Routes>
  );
}
