import { useState } from "react";
import { Button } from "@astryxdesign/core/Button";
import { Stack } from "@astryxdesign/core/Stack";
import { Text } from "@astryxdesign/core/Text";
import { Heading } from "@astryxdesign/core/Heading";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Banner } from "@astryxdesign/core/Banner";
import { Page } from "../components/Page";
import { useApiKeys } from "../../app/hooks";

export function ApiKeys() {
  const { list, create, revoke } = useApiKeys();
  const [name, setName] = useState("");
  const [plain, setPlain] = useState("");
  const [err, setErr] = useState("");

  async function add() {
    setErr("");
    setPlain("");
    if (!name.trim()) {
      setErr("name 1-64 chars");
      return;
    }
    try {
      const r = await create.mutateAsync(name.trim());
      setPlain(r.key);
      setName("");
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <Page>
      <Stack gap={2}>
        <Heading level={1}>API keys</Heading>
        <Text type="supporting">
          Eval calls use Bearer key. Plain key shows once. Revoke is instant.
        </Text>
      </Stack>
      {err && <Banner status="error" title={err} />}
      {plain && <Banner status="success" title={`Copy now: ${plain}`} />}
      <Stack gap={3} maxWidth={480}>
        <TextInput
          label="Key name"
          value={name}
          onChange={setName}
          isRequired
          width="100%"
          placeholder="backend-prod"
        />
        <Button label="Create key" variant="primary" isLoading={create.isPending} onClick={add} />
      </Stack>
      {(list.data?.keys ?? []).map((k) => (
        <Stack key={k.id} gap={2} paddingBlock={4}>
          <Text weight="semibold">{k.name}</Text>
          <Text type="supporting">
            {k.prefix}…, {k.revoked_at ? "revoked" : "active"}
          </Text>
          {!k.revoked_at && (
            <Button
              label="Revoke"
              variant="destructive"
              clickAction={() => {
                void revoke.mutateAsync(k.id);
              }}
            />
          )}
        </Stack>
      ))}
    </Page>
  );
}
