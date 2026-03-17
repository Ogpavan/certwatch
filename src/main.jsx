import React from "react";
import ReactDOM from "react-dom/client";
import { BrowserRouter } from "react-router-dom";
import { ConfigProvider } from "antd";
import App from "./App.jsx";
import "antd/dist/reset.css";
import "./styles/theme.css";

const customTheme = {
  token: {
    colorPrimary: "#3B82F6",
    colorInfo: "#3B82F6",
    colorBgLayout: "#F8FAFC",
    colorBgContainer: "#FFFFFF",
    colorText: "#0F172A",
    colorTextSecondary: "#64748B",
    colorBorder: "#E2E8F0"
  }
};

ReactDOM.createRoot(document.getElementById("root")).render(
  <React.StrictMode>
    <ConfigProvider theme={customTheme}>
      <BrowserRouter>
        <App />
      </BrowserRouter>
    </ConfigProvider>
  </React.StrictMode>
);



