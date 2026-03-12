import { useState } from "react";
import { Button, Card, Form, Input, message, Tabs } from "antd";
import { useNavigate } from "react-router-dom";
import { api, setToken } from "../services/api.js";

export default function LoginPage() {
  const [loading, setLoading] = useState(false);
  const navigate = useNavigate();

  const handleSubmit = async (values, mode) => {
    setLoading(true);
    try {
      const payload = mode === "register" ? await api.register(values) : await api.login(values);
      if (payload?.token) {
        setToken(payload.token);
        message.success("Authenticated successfully");
        navigate("/dashboard");
      }
    } catch (err) {
      message.error(err.message || "Authentication failed");
    } finally {
      setLoading(false);
    }
  };

  return (
    <div style={{ minHeight: "100vh", display: "grid", placeItems: "center", padding: 24 }}>
      <Card className="card-surface" style={{ width: "min(420px, 90vw)" }}>
        <div className="section-title" style={{ marginBottom: 16 }}>DomainGuard Access</div>
        <Tabs
          defaultActiveKey="login"
          items={[
            {
              key: "login",
              label: "Login",
              children: (
                <Form layout="vertical" onFinish={(values) => handleSubmit(values, "login")}>
                  <Form.Item name="email" label="Email" rules={[{ required: true, type: "email" }]}>
                    <Input placeholder="you@company.com" />
                  </Form.Item>
                  <Form.Item name="password" label="Password" rules={[{ required: true, min: 6 }]}>
                    <Input.Password placeholder="••••••••" />
                  </Form.Item>
                  <Button type="primary" htmlType="submit" block loading={loading}>
                    Login
                  </Button>
                </Form>
              )
            },
            {
              key: "register",
              label: "Register",
              children: (
                <Form layout="vertical" onFinish={(values) => handleSubmit(values, "register")}>
                  <Form.Item name="email" label="Email" rules={[{ required: true, type: "email" }]}>
                    <Input placeholder="you@company.com" />
                  </Form.Item>
                  <Form.Item name="password" label="Password" rules={[{ required: true, min: 6 }]}>
                    <Input.Password placeholder="Create a password" />
                  </Form.Item>
                  <Button type="primary" htmlType="submit" block loading={loading}>
                    Register
                  </Button>
                </Form>
              )
            }
          ]}
        />
      </Card>
    </div>
  );
}
