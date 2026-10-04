import type { ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { Flag, KeyRound, FlaskConical, Plus, LogOut } from "lucide-react";
import { AppShell } from "@astryxdesign/core/AppShell";
import { SideNav, SideNavItem, SideNavSection, SideNavHeading } from "@astryxdesign/core/SideNav";
import { TopNav, TopNavHeading } from "@astryxdesign/core/TopNav";
import { Button } from "@astryxdesign/core/Button";
import { Badge } from "@astryxdesign/core/Badge";
import { Text } from "@astryxdesign/core/Text";
import { tokenStore } from "../data/client";
import { useFlags } from "../app/hooks";
import { Login } from "./pages/Login";
import { ChangePassword } from "./pages/ChangePassword";
import { Flags } from "./pages/Flags";
import { FlagDetail } from "./pages/FlagDetail";
import { ApiKeys } from "./pages/ApiKeys";
import { Playground } from "./pages/Playground";

function guard(el: ReactNode) {
  return tokenStore.get() ? el : <Navigate to="/login" replace />;
}

function Shell() {
  const nav = useNavigate();
  const loc = useLocation();
  const { data } = useFlags();
  const flags = data?.flags ?? [];
  const live = flags.filter((f) => f.enabled).length;

  function signOut() {
    tokenStore.clear();
    nav("/login");
  }

  return (
    <AppShell
      sideNav={
        <SideNav
          header={<SideNavHeading heading="Waver" headingHref="/" icon={<Flag />} />}
          topContent={
            <Button
              label="New flag"
              variant="primary"
              width="100%"
              icon={<Plus />}
              onClick={() => nav("/flags/new")}
            />
          }
          footer={
            <Button
              label="Sign out"
              variant="ghost"
              width="100%"
              icon={<LogOut />}
              onClick={signOut}
            />
          }
        >
          <SideNavSection title="Operate">
            <SideNavItem
              label="Flags"
              icon={<Flag />}
              isSelected={loc.pathname === "/" || loc.pathname.startsWith("/flags")}
              onClick={() => nav("/")}
              endContent={
                flags.length > 0 ? (
                  <Badge label={`${live} live`} variant={live > 0 ? "success" : "neutral"} />
                ) : undefined
              }
            />
            <SideNavItem
              label="API keys"
              icon={<KeyRound />}
              isSelected={loc.pathname === "/keys"}
              onClick={() => nav("/keys")}
            />
            <SideNavItem
              label="Playground"
              icon={<FlaskConical />}
              isSelected={loc.pathname === "/playground"}
              onClick={() => nav("/playground")}
            />
          </SideNavSection>
        </SideNav>
      }
      topNav={
        <TopNav
          heading={<TopNavHeading heading="Waver Flags" />}
          endContent={
            flags.length > 0 ? (
              <Badge
                label={`${live} of ${flags.length} live`}
                variant={live > 0 ? "success" : "neutral"}
              />
            ) : (
              <Text type="supporting">No flags yet</Text>
            )
          }
        />
      }
    >
      <Routes>
        <Route path="/" element={guard(<Flags />)} />
        <Route path="/flags/:key" element={guard(<FlagDetail />)} />
        <Route path="/keys" element={guard(<ApiKeys />)} />
        <Route path="/playground" element={guard(<Playground />)} />
        <Route path="/change-password" element={<ChangePassword />} />
      </Routes>
    </AppShell>
  );
}

export function AppRoutes() {
  return (
    <BrowserRouter>
      <Routes>
        <Route path="/login" element={<Login />} />
        <Route path="/*" element={<Shell />} />
      </Routes>
    </BrowserRouter>
  );
}
