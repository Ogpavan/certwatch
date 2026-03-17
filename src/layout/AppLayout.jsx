import { useMemo, useState } from "react";
import { useLocation, useNavigate, Outlet } from "react-router-dom";
import {
  Layout,
  Menu,
  Input,
  Badge,
  Avatar,
  Dropdown,
  Space,
  Button
} from "antd";
import {
  DashboardOutlined,
  ProjectOutlined,
  GlobalOutlined,
  AlertOutlined,
  FileTextOutlined,
  SettingOutlined,
  BellOutlined,
  UserOutlined,
  MenuFoldOutlined,
  MenuUnfoldOutlined
} from "@ant-design/icons";

const { Header, Sider, Content } = Layout;

const menuItems = [
  {
    key: "/dashboard",
    icon: <DashboardOutlined />,
    label: "Dashboard"
  },
  {
    key: "/projects",
    icon: <ProjectOutlined />,
    label: "Projects"
  },
  {
    key: "/domains",
    icon: <GlobalOutlined />,
    label: "Domains"
  },
  {
    key: "/alerts",
    icon: <AlertOutlined />,
    label: "Alerts"
  },
  {
    key: "/logs",
    icon: <FileTextOutlined />,
    label: "Logs"
  },
  {
    key: "/settings",
    icon: <SettingOutlined />,
    label: "Settings"
  }
];

export default function AppLayout() {
  const [collapsed, setCollapsed] = useState(false);
  const location = useLocation();
  const navigate = useNavigate();

  const selectedKey = useMemo(() => {
    const match = menuItems.find((item) => location.pathname.startsWith(item.key));
    return match ? [match.key] : ["/dashboard"];
  }, [location.pathname]);

  const userMenu = {
    items: [
      { key: "profile", label: "Profile" },
      { key: "settings", label: "Settings" }
    ],
    onClick: ({ key }) => {
      if (key === "settings") {
        navigate("/settings");
      }
    }
  };

  return (
    <Layout className="app-shell">
      <Header className="app-header">
        <div className="header-left">
          <Space size={16} align="center">
            <Button
              type="text"
              onClick={() => setCollapsed((prev) => !prev)}
              icon={collapsed ? <MenuUnfoldOutlined /> : <MenuFoldOutlined />}
            />
            <div className="logo">
              <span className="logo-badge">C</span>
              <span>CertWatch</span>
            </div>
          </Space>
        </div>
        <div className="header-right">
          <div className="header-actions">
            <Badge count={5} size="small" offset={[-2, 2]}>
              <Button type="text" icon={<BellOutlined />} />
            </Badge>
            <Dropdown menu={userMenu} placement="bottomRight">
              <Space>
                <Avatar icon={<UserOutlined />} />
              </Space>
            </Dropdown>
          </div>
        </div>
      </Header>
      <Layout>
        <Sider
          width={220}
          collapsible
          collapsed={collapsed}
          trigger={null}
          breakpoint="lg"
          className="app-sider"
        >
          <Menu
            mode="inline"
            theme="light"
            selectedKeys={selectedKey}
            items={menuItems}
            onClick={(info) => navigate(info.key)}
          />
        </Sider>
        <Content className="app-content">
          <Outlet />
        </Content>
      </Layout>
    </Layout>
  );
}
