import type { ReactNode } from "react";
import { Center } from "@astryxdesign/core/Center";
import { Stack } from "@astryxdesign/core/Stack";

// Page is the single rhythm for authed views: centered column,
// generous outer padding, one gap scale between sections.
export function Page({ children }: { children: ReactNode }) {
  return (
    <Center axis="horizontal" padding={4}>
      <Stack gap={5} paddingBlock={6} width="100%" maxWidth={880}>
        {children}
      </Stack>
    </Center>
  );
}
