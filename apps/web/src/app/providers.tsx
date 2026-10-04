import { Link } from "react-router-dom";
import { QueryClientProvider } from "@tanstack/react-query";
import { Theme } from "@astryxdesign/core/theme";
import { LinkProvider } from "@astryxdesign/core/Link";
import { neutralTheme } from "@astryxdesign/theme-neutral/built";
import { queryClient } from "./hooks";

export function Providers({ children }: { children: React.ReactNode }) {
  return (
    <Theme theme={neutralTheme}>
      <LinkProvider component={Link}>
        <QueryClientProvider client={queryClient}>{children}</QueryClientProvider>
      </LinkProvider>
    </Theme>
  );
}
