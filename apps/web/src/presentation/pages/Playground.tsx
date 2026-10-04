import { useState } from "react";
import { Button } from "@astryxdesign/core/Button";
import { Stack } from "@astryxdesign/core/Stack";
import { Text } from "@astryxdesign/core/Text";
import { Heading } from "@astryxdesign/core/Heading";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Banner } from "@astryxdesign/core/Banner";
import { Page } from "../components/Page";

// Admin-side playground. Proxies one eval through the admin session.
// Never paste a real eval key here on a shared screen.
export function Playground() {
  const [flagKey, setFlagKey] = useState("");
  const [targetingKey, setTargetingKey] = useState("user-123");
  const [out, setOut] = useState("");
  const [err, setErr] = useState("");

  async function run() {
    setErr("");
    setOut("");
    try {
      const res = await fetch(`/api/v1/flags`);
      if (!res.ok) throw new Error("load flags failed");
      setOut(
        "Use a server OFREP call for truth. Example:\n" +
          `curl -X POST /ofrep/v1/evaluate/flags/${flagKey || "{key}"} \\\n` +
          `  -H "Authorization: Bearer <API_KEY>" -H "Content-Type: application/json" \\\n` +
          `  -d '{"context":{"targetingKey":"${targetingKey}"}}'`,
      );
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <Page>
      <Stack gap={2}>
        <Heading level={1}>Playground</Heading>
        <Text type="supporting">
          Eval keys stay server-side. This page builds the curl, never calls OFREP from the browser.
        </Text>
      </Stack>
      {err && <Banner status="error" title={err} />}
      <Stack gap={3} maxWidth={560}>
        <TextInput
          label="Flag key"
          value={flagKey}
          onChange={setFlagKey}
          width="100%"
          placeholder="new-checkout"
        />
        <TextInput
          label="targetingKey"
          value={targetingKey}
          onChange={setTargetingKey}
          width="100%"
        />
        <Button label="Build eval curl" variant="primary" onClick={run} />
      </Stack>
      {out && <Text type="code">{out}</Text>}
    </Page>
  );
}
