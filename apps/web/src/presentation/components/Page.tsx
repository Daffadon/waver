import type { ReactNode } from "react";
import { Center } from "@astryxdesign/core/Center";
import { Stack, HStack, VStack } from "@astryxdesign/core/Stack";
import { Heading } from "@astryxdesign/core/Heading";
import { Text } from "@astryxdesign/core/Text";

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

// PageHeader is the one page top: title, one supporting line, actions on the end.
// Wraps so the action drops below the title on narrow screens.
export function PageHeader({
  title,
  description,
  actions,
}: {
  title: string;
  description?: string;
  actions?: ReactNode;
}) {
  return (
    <HStack justify="between" align="end" gap={4} wrap="wrap">
      <VStack gap={1}>
        <Heading level={1}>{title}</Heading>
        {description && <Text type="supporting">{description}</Text>}
      </VStack>
      {actions}
    </HStack>
  );
}
