import { useState } from "react";
import { CodeBlock } from "@astryxdesign/core/CodeBlock";
import { Section } from "@astryxdesign/core/Section";
import { VStack } from "@astryxdesign/core/Stack";
import { Text } from "@astryxdesign/core/Text";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Page, PageHeader } from "../components/Page";
import { useFlags } from "../../app/hooks";

// Admin-side playground. Builds the server-side OFREP call. An eval key must
// never be pasted into a browser, so this page never calls OFREP itself.
export function Playground() {
  const [flagKey, setFlagKey] = useState("");
  const [targetingKey, setTargetingKey] = useState("user-123");
  const { data } = useFlags();
  const knownKeys = (data?.flags ?? []).map((f) => f.key).slice(0, 4);

  const curl = [
    `curl -X POST ${window.location.origin}/ofrep/v1/evaluate/flags/${flagKey || "<key>"} \\`,
    `  -H "Authorization: Bearer <API_KEY>" \\`,
    `  -H "Content-Type: application/json" \\`,
    `  -d '{"context":{"targetingKey":"${targetingKey || "user-123"}"}}'`,
  ].join("\n");

  return (
    <Page>
      <PageHeader
        title="Playground"
        description="Eval keys stay server-side. This page builds the curl, never calls OFREP from the browser."
      />
      <Section variant="muted" padding={4}>
        <VStack gap={3} maxWidth={480}>
          <TextInput
            label="Flag key"
            value={flagKey}
            onChange={setFlagKey}
            width="100%"
            placeholder="new-checkout"
            description={knownKeys.length > 0 ? `Existing: ${knownKeys.join(", ")}` : undefined}
          />
          <TextInput
            label="targetingKey"
            value={targetingKey}
            onChange={setTargetingKey}
            width="100%"
            placeholder="user-123"
          />
        </VStack>
      </Section>
      <VStack gap={2}>
        <CodeBlock code={curl} language="bash" title="Server-side evaluation" />
        <Text type="supporting">
          Bulk eval: drop the flag key from the path and send the same body to /ofrep/v1/evaluate/flags.
        </Text>
      </VStack>
    </Page>
  );
}
