import type { ReactNode } from "react";
import { BrowserRouter, Navigate, Route, Routes, useLocation, useNavigate } from "react-router-dom";
import { Flag, KeyRound, FlaskConical, LogOut, Moon, Monitor, Plus, Sun } from "lucide-react";
import { AppShell } from "@astryxdesign/core/AppShell";
import { SideNav, SideNavItem, SideNavSection, SideNavHeading } from "@astryxdesign/core/SideNav";
import { TopNav, TopNavHeading } from "@astryxdesign/core/TopNav";
import { Button } from "@astryxdesign/core/Button";
import { IconButton } from "@astryxdesign/core/IconButton";
import { Badge } from "@astryxdesign/core/Badge";
import { EmptyState } from "@astryxdesign/core/EmptyState";
import { NavIcon } from "@astryxdesign/core/NavIcon";
import { SegmentedControl, SegmentedControlItem } from "@astryxdesign/core/SegmentedControl";
import { Text } from "@astryxdesign/core/Text";
import { HStack } from "@astryxdesign/core/Stack";
import { tokenStore } from "../data/client";
import { useFlags } from "../app/hooks";
import { useThemeMode, type ThemeMode } from "../app/providers";
import { Page } from "./components/Page";
import { Login } from "./pages/Login";
import { ChangePassword } from "./pages/ChangePassword";
import { Flags } from "./pages/Flags";
import { FlagDetail } from "./pages/FlagDetail";
import { ApiKeys } from "./pages/ApiKeys";
import { Playground } from "./pages/Playground";

function guard(el: ReactNode) {
  return tokenStore.get() ? el : <Navigate to="/login" replace />;
}

// ThemeSwitch lives in the sidebar footer: one compact icon-only control, so
// the footer row stays a single line next to Sign out.
function ThemeSwitch() {
  const { mode, setMode } = useThemeMode();
  return (
    <SegmentedControl
      value={mode}
      onChange={(value) => setMode(value as ThemeMode)}
      label="Theme"
      size="sm"
    >
      <SegmentedControlItem value="light" label="Light" icon={<Sun size={16} />} isLabelHidden />
      <SegmentedControlItem value="dark" label="Dark" icon={<Moon size={16} />} isLabelHidden />
      <SegmentedControlItem
        value="system"
        label="System"
        icon={<Monitor size={16} />}
        isLabelHidden
      />
    </SegmentedControl>
  );
}

function NotFound() {
  const nav = useNavigate();
  return (
    <Page>
      <EmptyState
        title="Page not found"
        description="That route does not exist."
        actions={
          <Button label="Back to flags" variant="primary" onClick={() => nav("/")} />
        }
      />
    </Page>
  );
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
          header={
            <SideNavHeading
              heading="Waver"
              subheading="Feature flags"
              headingHref="/"
              icon={<NavIcon icon={<Flag size={16} />} />}
            />
          }
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
            <HStack justify="between" align="center">
              <ThemeSwitch />
              <IconButton
                label="Sign out"
                icon={<LogOut size={16} />}
                variant="ghost"
                size="sm"
                tooltip="Sign out"
                onClick={signOut}
              />
            </HStack>
          }
        >
          <SideNavSection title="Operate" isHeaderHidden>
            <SideNavItem
              label="Flags"
              icon={<Flag />}
              href="/"
              isSelected={loc.pathname === "/" || loc.pathname.startsWith("/flags")}
              endContent={
                flags.length > 0 ? (
                  <Badge label={`${live} live`} variant={live > 0 ? "success" : "neutral"} />
                ) : undefined
              }
            />
            <SideNavItem
              label="API keys"
              icon={<KeyRound />}
              href="/keys"
              isSelected={loc.pathname === "/keys"}
            />
            <SideNavItem
              label="Playground"
              icon={<FlaskConical />}
              href="/playground"
              isSelected={loc.pathname === "/playground"}
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
        <Route path="*" element={guard(<NotFound />)} />
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
