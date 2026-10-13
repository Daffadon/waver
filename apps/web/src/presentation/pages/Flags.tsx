import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@astryxdesign/core/Button";
import { IconButton } from "@astryxdesign/core/IconButton";
import { Badge } from "@astryxdesign/core/Badge";
import { Banner } from "@astryxdesign/core/Banner";
import { EmptyState } from "@astryxdesign/core/EmptyState";
import { Skeleton } from "@astryxdesign/core/Skeleton";
import { Link } from "@astryxdesign/core/Link";
import { Code } from "@astryxdesign/core/CodeBlock";
import { Table, proportional, pixel, type TableColumn } from "@astryxdesign/core/Table";
import { HStack, VStack } from "@astryxdesign/core/Stack";
import { Text } from "@astryxdesign/core/Text";
import { TextInput } from "@astryxdesign/core/TextInput";
import { useImperativeAlertDialog } from "@astryxdesign/core/AlertDialog";
import { useToast } from "@astryxdesign/core/Toast";
import { Flag, Pencil, Plus, Trash2 } from "lucide-react";
import { Page, PageHeader } from "../components/Page";
import { useFlagMutations, useFlags } from "../../app/hooks";
import type { Flag as FlagModel } from "../../domain/types";

const dateFmt = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

function FlagBadges({ flag }: { flag: FlagModel }) {
  return (
    <HStack gap={1} wrap="wrap">
      <Badge variant={flag.enabled ? "success" : "neutral"} label={flag.enabled ? "live" : "off"} />
      <Badge
        variant={flag.default_value ? "info" : "neutral"}
        label={flag.default_value ? "default on" : "default off"}
      />
      {(flag.starts_at || flag.ends_at) && <Badge variant="warning" label="scheduled" />}
    </HStack>
  );
}

export function Flags() {
  const nav = useNavigate();
  const { data, isLoading, error } = useFlags();
  const { remove } = useFlagMutations();
  const confirm = useImperativeAlertDialog();
  const showToast = useToast();
  const [query, setQuery] = useState("");

  const flags = data?.flags ?? [];
  const needle = query.trim().toLowerCase();
  const rows = needle
    ? flags.filter(
        (f) =>
          f.key.toLowerCase().includes(needle) || f.name.toLowerCase().includes(needle),
      )
    : flags;

  async function deleteFlag(key: string) {
    try {
      await remove.mutateAsync(key);
      showToast({ body: `Deleted ${key}` });
    } catch (e) {
      showToast({ body: (e as Error).message, type: "error" });
    }
  }

  const columns: TableColumn<FlagModel>[] = [
    {
      key: "key",
      header: "Key",
      width: proportional(2),
      renderCell: (f) => (
        <Link href={`/flags/${f.key}`}>
          <Code>{f.key}</Code>
        </Link>
      ),
    },
    {
      key: "name",
      header: "Name",
      width: proportional(2),
      renderCell: (f) => (
        <VStack gap={0.5}>
          <Text>{f.name}</Text>
          {f.description && <Text type="supporting">{f.description}</Text>}
        </VStack>
      ),
    },
    {
      key: "state",
      header: "State",
      width: proportional(2),
      renderCell: (f) => <FlagBadges flag={f} />,
    },
    {
      key: "rules",
      header: "Rules",
      width: pixel(72),
      align: "end",
      renderCell: (f) => <Text type="supporting">{f.rules.length}</Text>,
    },
    {
      key: "updated_at",
      header: "Updated",
      width: pixel(160),
      renderCell: (f) => (
        <Text type="supporting">{dateFmt.format(new Date(f.updated_at * 1000))}</Text>
      ),
    },
    {
      key: "actions",
      header: "",
      width: pixel(88),
      align: "end",
      renderCell: (f) => (
        <HStack gap={1} justify="end">
          <IconButton
            label={`Open ${f.key}`}
            tooltip="Open"
            size="sm"
            variant="ghost"
            icon={<Pencil />}
            onClick={() => nav(`/flags/${f.key}`)}
          />
          <IconButton
            label={`Delete ${f.key}`}
            tooltip="Delete"
            size="sm"
            variant="ghost"
            icon={<Trash2 />}
            clickAction={() =>
              confirm.show({
                title: `Delete ${f.key}?`,
                description:
                  "Evaluation falls back to the default for this key. This cannot be undone.",
                actionLabel: "Delete flag",
                // onAction does not close the dialog; close it, then report
                // the outcome through the toast.
                onAction: () => {
                  confirm.hide();
                  void deleteFlag(f.key);
                },
              })
            }
          />
        </HStack>
      ),
    },
  ];

  return (
    <Page>
      <PageHeader
        title="Flags"
        description="Boolean only. Kill switch, UTC schedule, attr rules, rollout."
        actions={
          <Button
            label="New flag"
            variant="primary"
            icon={<Plus />}
            onClick={() => nav("/flags/new")}
          />
        }
      />
      {error && (
        <Banner
          status="error"
          title="Could not load flags"
          description={(error as Error).message}
        />
      )}
      {!error && flags.length > 0 && (
        <HStack justify="between" align="end" gap={3} wrap="wrap">
          <TextInput
            label="Search flags"
            isLabelHidden
            value={query}
            onChange={setQuery}
            placeholder="Search key or name"
            width={260}
          />
          <Text type="supporting">
            {rows.length} of {flags.length}
          </Text>
        </HStack>
      )}
      {isLoading ? (
        <VStack gap={2}>
          {[0, 1, 2, 3].map((i) => (
            <Skeleton key={i} height={44} radius={2} index={i} />
          ))}
        </VStack>
      ) : rows.length === 0 ? (
        <EmptyState
          icon={<Flag />}
          title={needle ? "No flag matches that search" : "No flags yet"}
          description={
            needle
              ? "Clear the search to see every flag."
              : "A flag is a boolean plus optional rules, schedule, and rollout. Create the first one to start evaluating."
          }
          actions={
            needle ? (
              <Button label="Clear search" onClick={() => setQuery("")} />
            ) : (
              <Button
                label="New flag"
                variant="primary"
                icon={<Plus />}
                onClick={() => nav("/flags/new")}
              />
            )
          }
        />
      ) : (
        <Table<FlagModel>
          data={rows}
          columns={columns}
          idKey="key"
          density="compact"
          dividers="rows"
          hasHover
        />
      )}
      {confirm.element}
    </Page>
  );
}
