import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./styles.css";
import "./astribot/styles.css";
import { isMockMode } from "./config";
import { installMockBackend } from "./mockBackend";
import { enterIamPortal } from "./astribot/iam/portal";

if (isMockMode) installMockBackend();

enterIamPortal().then(() => {
  ReactDOM.createRoot(document.getElementById("root")!).render(
    <React.StrictMode>
      <App />
    </React.StrictMode>,
  );
});
