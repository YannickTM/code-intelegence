import React, { useState, Fragment } from "react";
import { BrowserRouter, Route, Switch } from "react-router-dom";
import Header from "./components/Header";
import Sidebar from "./components/Sidebar";
import Dashboard from "../pages/Dashboard";
import Settings from "../pages/Settings";

/**
 * Navigation item used in the sidebar.
 */
const NAV_ITEMS = [
  { path: "/", label: "Dashboard", icon: "home" },
  { path: "/settings", label: "Settings", icon: "gear" },
];

/**
 * Main application shell with routing and layout.
 */
export default function App() {
  const [sidebarOpen, setSidebarOpen] = useState(true);
  const [theme, setTheme] = useState("light");

  const toggleSidebar = () => setSidebarOpen((prev) => !prev);

  return (
    <BrowserRouter>
      <div className={`app-container theme-${theme}`}>
        <Header onMenuClick={toggleSidebar} title="My App" />
        <div className="app-body">
          <Sidebar open={sidebarOpen} items={NAV_ITEMS} />
          <main className="app-content">
            <Switch>
              <Route exact path="/">
                <Dashboard />
              </Route>
              <Route path="/settings">
                <Settings theme={theme} onThemeChange={setTheme} />
              </Route>
            </Switch>
          </main>
        </div>
      </div>
    </BrowserRouter>
  );
}

/**
 * Simple status badge component.
 */
export const StatusBadge = ({ status, label }) => {
  const colorMap = {
    success: "green",
    warning: "orange",
    error: "red",
  };

  return (
    <Fragment>
      <span
        className="status-badge"
        style={{ backgroundColor: colorMap[status] || "gray" }}
      >
        {label || status}
      </span>
    </Fragment>
  );
};

/**
 * Empty state placeholder for lists.
 */
export function EmptyState({ message }) {
  return (
    <>
      <div className="empty-state">
        <img src="/images/empty.svg" alt="No data" />
        <p>{message}</p>
      </div>
    </>
  );
}
