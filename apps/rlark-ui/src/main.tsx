import React from "react";
import ReactDOM from "react-dom/client";
import App from "./App";
import "./styles.css";
import "./astribot/styles.css";
import { isMockMode } from "./config";
import { installMockBackend } from "./mockBackend";
import {
  enterIamPortal,
  iamPortalEnabled,
  type PortalAuthState,
} from "./astribot/iam/portal";

if (isMockMode) installMockBackend();

const root = ReactDOM.createRoot(document.getElementById("root")!);

function render(iamState?: PortalAuthState) {
  root.render(
    <React.StrictMode>
      <App iamState={iamState} />
    </React.StrictMode>,
  );
}

if (iamPortalEnabled()) {
  // Nothing but a status placeholder exists until the session check answers,
  // so no native sign-in form can flash before the portal decides.
  render({ status: "loading" });
  void enterIamPortal().then(render);
} else {
  render();
}
