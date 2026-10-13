import { createContext, useContext, useState } from "react";
import { Link } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { Theme } from "@astryxdesign/core/theme";
import { LinkProvider } from "@astryxdesign/core/Link";
import { neutralTheme } from "@astryxdesign/theme-neutral/built";
import { queryClient } from "./hooks";

export type ThemeMode = "light" | "dark" | "system";

const MODE_KEY = "waver.theme";
const ThemeModeContext = createContext<{
  mode: ThemeMode;
  setMode: (mode: ThemeMode) => void;
}>({ mode: "system", setMode: () => {} });

export function useThemeMode() {
  return useContext(ThemeModeContext);
}

function readStoredMode(): ThemeMode {
  const saved = localStorage.getItem(MODE_KEY);
  return saved === "light" || saved === "dark" ? saved : "system";
}

export function Providers({ children }: { children: React.ReactNode }) {
  const [mode, setModeState] = useState<ThemeMode>(readStoredMode);

  function setMode(next: ThemeMode) {
    localStorage.setItem(MODE_KEY, next);
    setModeState(next);
  }

  return (
    <Theme theme={neutralTheme} mode={mode}>
      <ThemeModeContext value={{ mode, setMode }}>
        <LinkProvider component={Link}>
          <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
        </LinkProvider>
      </ThemeModeContext>
    </Theme>
  );
}
