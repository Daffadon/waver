import type { ReactNode } from "react";
import { Card } from "@astryxdesign/core/Card";
import { Center } from "@astryxdesign/core/Center";
import { Stack } from "@astryxdesign/core/Stack";

// AuthShell centers one card on both axes. Form content stays left aligned.
export function AuthShell({ children }: { children: ReactNode }) {
  return (
    <Center axis="both" minHeight="100dvh" padding={4}>
      <Card width="100%" maxWidth={440} padding={6}>
        <Stack gap={4}>{children}</Stack>
      </Card>
    </Center>
  );
}
