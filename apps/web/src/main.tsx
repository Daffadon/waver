import React from "react";
import ReactDOM from "react-dom/client";
import { Providers } from "./app/providers";
import { AppRoutes } from "./presentation/routes";
import "./globals.css";

ReactDOM.createRoot(document.getElementById("root")!).render(
  <React.StrictMode>
    <Providers>
      <AppRoutes />
    </Providers>
  </React.StrictMode>,
);
