import { useNavigate } from "react-router-dom";
import { Button } from "@astryxdesign/core/Button";
import { Card } from "@astryxdesign/core/Card";
import { Stack } from "@astryxdesign/core/Stack";
import { Text } from "@astryxdesign/core/Text";
import { Heading } from "@astryxdesign/core/Heading";
import { Badge } from "@astryxdesign/core/Badge";
import { Page } from "../components/Page";
import { useFlagMutations, useFlags } from "../../app/hooks";

export function Flags() {
  const nav = useNavigate();
  const { data, isLoading, error } = useFlags();
  const { remove } = useFlagMutations();

  return (
    <Page>
      <Stack gap={2}>
        <Heading level={1}>Flags</Heading>
        <Text type="supporting">Boolean only. Kill switch, UTC schedule, attr rules, rollout.</Text>
      </Stack>
      <Button label="New flag" variant="primary" onClick={() => nav("/flags/new")} />
      {isLoading && <Text>Loading.</Text>}
      {error && <Text>Load failed: {(error as Error).message}</Text>}
      {(data?.flags ?? []).map((f) => (
        <Card key={f.key} width="100%" padding={5}>
          <Stack gap={3}>
            <Text size="lg" weight="semibold">
              {f.key}
            </Text>
            <Text type="supporting">
              {f.name}, {f.rules.length} rules
            </Text>
            <Stack direction="horizontal" gap={2}>
              <Badge label={f.enabled ? "enabled" : "disabled"} />
              <Badge label={f.default_value ? "default on" : "default off"} />
            </Stack>
            <Stack direction="horizontal" gap={2} paddingBlockStart={2}>
              <Button label="Open" onClick={() => nav(`/flags/${f.key}`)} />
              <Button
                label="Delete"
                variant="destructive"
                clickAction={() => {
                  void remove.mutateAsync(f.key);
                }}
              />
            </Stack>
          </Stack>
        </Card>
      ))}
    </Page>
  );
}
