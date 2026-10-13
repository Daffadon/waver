import { useState } from "react";
import { Button } from "@astryxdesign/core/Button";
import { Badge } from "@astryxdesign/core/Badge";
import { Banner } from "@astryxdesign/core/Banner";
import { Code } from "@astryxdesign/core/CodeBlock";
import { EmptyState } from "@astryxdesign/core/EmptyState";
import { List, ListItem } from "@astryxdesign/core/List";
import { Section } from "@astryxdesign/core/Section";
import { Skeleton } from "@astryxdesign/core/Skeleton";
import { HStack, VStack } from "@astryxdesign/core/Stack";
import { TextInput } from "@astryxdesign/core/TextInput";
import { useImperativeAlertDialog } from "@astryxdesign/core/AlertDialog";
import { useToast } from "@astryxdesign/core/Toast";
import { KeyRound } from "lucide-react";
import { Page, PageHeader } from "../components/Page";
import { useApiKeys } from "../../app/hooks";

const dateFmt = new Intl.DateTimeFormat(undefined, { dateStyle: "medium" });

export function ApiKeys() {
  const { list, create, revoke } = useApiKeys();
  const confirm = useImperativeAlertDialog();
  const showToast = useToast();
  const [name, setName] = useState("");
  const [plain, setPlain] = useState("");
  const [err, setErr] = useState("");

  const keys = list.data?.keys ?? [];

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

  async function copyKey() {
    try {
      await navigator.clipboard.writeText(plain);
      showToast({ body: "Key copied" });
    } catch {
      showToast({ body: "Clipboard blocked — select and copy manually", type: "error" });
    }
  }

  async function revokeKey(id: string, keyName: string) {
    try {
      await revoke.mutateAsync(id);
      showToast({ body: `Revoked ${keyName}` });
    } catch (e) {
      showToast({ body: (e as Error).message, type: "error" });
    }
  }

  return (
    <Page>
      <PageHeader
        title="API keys"
        description="Eval calls use Bearer key. Plain key shows once. Revoke is instant."
      />
      {err && <Banner status="error" title="Key not created" description={err} />}
      {plain && (
        <Banner
          status="success"
          title="Copy this key now"
          description="Shown once. Only the prefix is stored."
          collapsible={false}
          isDismissable
          onDismiss={() => setPlain("")}
          endContent={<Button label="Copy key" onClick={copyKey} />}
        >
          <Code>{plain}</Code>
        </Banner>
      )}
      <Section variant="muted" padding={4}>
        <HStack align="end" gap={3} wrap="wrap">
          <TextInput
            label="Key name"
            value={name}
            onChange={setName}
            isRequired
            width={320}
            placeholder="backend-prod"
          />
          <Button
            label="Create key"
            variant="primary"
            isLoading={create.isPending}
            clickAction={add}
          />
        </HStack>
      </Section>
      {list.isLoading ? (
        <VStack gap={2}>
          {[0, 1].map((i) => (
            <Skeleton key={i} height={44} radius={2} index={i} />
          ))}
        </VStack>
      ) : keys.length === 0 ? (
        <EmptyState
          icon={<KeyRound />}
          title="No API keys yet"
          description="Create a key to evaluate flags from a server. Keys are shown once and can be revoked at any time."
        />
      ) : (
        <List hasDividers density="compact">
          {keys.map((k) => (
            <ListItem
              key={k.id}
              label={k.name}
              description={`${k.prefix}… · created ${dateFmt.format(new Date(k.created_at * 1000))}`}
              endContent={
                <HStack gap={2} align="center">
                  {k.revoked_at ? (
                    <Badge variant="neutral" label="revoked" />
                  ) : (
                    <Badge variant="success" label="active" />
                  )}
                  {!k.revoked_at && (
                    <Button
                      label="Revoke"
                      variant="destructive"
                      size="sm"
                      clickAction={() =>
                        confirm.show({
                          title: `Revoke ${k.name}?`,
                          description:
                            "Evaluation with this key is rejected immediately. This cannot be undone.",
                          actionLabel: "Revoke key",
                          // onAction does not close the dialog; close it, then
                          // report the outcome through the toast.
                          onAction: () => {
                            confirm.hide();
                            void revokeKey(k.id, k.name);
                          },
                        })
                      }
                    />
                  )}
                </HStack>
              }
            />
          ))}
        </List>
      )}
      {confirm.element}
    </Page>
  );
}
