import React, { lazy, Suspense } from "react";
import { createRoot } from "react-dom/client";
import "./i18n";
import "./entry.css";
const Landing = lazy(() => import("./marketing/Landing"));
const Workspace = lazy(() => import("./Workspace"));
const workspace = location.pathname !== "/" || /^#invite=/.test(location.hash);
createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <Suspense fallback={<div className="entry-loading" aria-busy="true" />}>
      {workspace ? <Workspace /> : <Landing />}
    </Suspense>
  </React.StrictMode>,
);
