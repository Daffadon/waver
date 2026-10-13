import { useMemo, useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { Button } from "@astryxdesign/core/Button";
import { Banner } from "@astryxdesign/core/Banner";
import { EmptyState } from "@astryxdesign/core/EmptyState";
import { Heading } from "@astryxdesign/core/Heading";
import { MetadataList, MetadataListItem } from "@astryxdesign/core/MetadataList";
import { Skeleton } from "@astryxdesign/core/Skeleton";
import { Switch } from "@astryxdesign/core/Switch";
import { Text } from "@astryxdesign/core/Text";
import { TextArea } from "@astryxdesign/core/TextArea";
import { TextInput } from "@astryxdesign/core/TextInput";
import { VStack } from "@astryxdesign/core/Stack";
import { useToast } from "@astryxdesign/core/Toast";
import { flagSchema, type Flag } from "../../domain/types";
import { datetimeLocalToUnix, unixToDatetimeLocal } from "../../data/client";
import { useFlagMutations, useFlags } from "../../app/hooks";
import { Page, PageHeader } from "../components/Page";

type Draft = {
  key: string;
  name: string;
  description: string;
  enabled: boolean;
  default_value: boolean;
  starts: string;
  ends: string;
  rulesJson: string;
};

type DraftField = keyof Draft;

// Prefilled for new flags. First match wins. Delete what you do not need.
const exampleRules = [
  { id: "pro-users", attr: "plan", op: "eq", values: ["pro"], value: true, rollout_pct: 100 },
  { id: "free-beta", attr: "plan", op: "eq", values: ["free"], value: true, rollout_pct: 10 },
];

const timeZone = Intl.DateTimeFormat().resolvedOptions().timeZone;
const updatedFmt = new Intl.DateTimeFormat(undefined, { dateStyle: "medium", timeStyle: "short" });

function draftFrom(flag: Flag): Draft {
  return {
    key: flag.key,
    name: flag.name,
    description: flag.description,
    enabled: flag.enabled,
    default_value: flag.default_value,
    starts: unixToDatetimeLocal(flag.starts_at),
    ends: unixToDatetimeLocal(flag.ends_at),
    rulesJson: JSON.stringify(flag.rules, null, 2),
  };
}

const newDraft: Draft = {
  key: "",
  name: "",
  description: "",
  enabled: true,
  default_value: false,
  starts: "",
  ends: "",
  rulesJson: JSON.stringify(exampleRules, null, 2),
};

// validate is the single source of truth for the form: field errors plus the
// parsed payload, so Save stays a two-branch call.
function validate(draft: Draft, routeKey: string | undefined) {
  const errors: Partial<Record<DraftField, string>> = {};
  let payload: Omit<Flag, "version" | "updated_at"> | null = null;

  let rules: unknown = [];
  try {
    rules = JSON.parse(draft.rulesJson || "[]");
  } catch {
    errors.rulesJson = "Not valid JSON";
  }
  if (draft.starts && datetimeLocalToUnix(draft.starts) == null) {
    errors.starts = "Use YYYY-MM-DDTHH:MM";
  }
  if (draft.ends && datetimeLocalToUnix(draft.ends) == null) {
    errors.ends = "Use YYYY-MM-DDTHH:MM";
  }

  const parsed = flagSchema.safeParse({
    key: routeKey ?? draft.key,
    name: draft.name,
    description: draft.description,
    enabled: draft.enabled,
    default_value: draft.default_value,
    rules: errors.rulesJson ? [] : rules,
    starts_at: datetimeLocalToUnix(draft.starts),
    ends_at: datetimeLocalToUnix(draft.ends),
  });
  if (parsed.success) payload = parsed.data;

  // Root-level refinements (schedule order) have no field to attach to.
  let formError = "";
  if (!parsed.success) {
    for (const issue of parsed.error.issues) {
      const path = issue.path[0];
      if (path === "rules") {
        errors.rulesJson = errors.rulesJson ?? issue.message;
      } else if (typeof path === "string" && path in draft) {
        errors[path as DraftField] = errors[path as DraftField] ?? issue.message;
      } else {
        formError = issue.message;
      }
    }
  }
  return { errors, payload, formError };
}

function FlagForm({
  routeKey,
  existing,
}: {
  routeKey?: string;
  existing?: Flag;
}) {
  const nav = useNavigate();
  const showToast = useToast();
  const { create, update } = useFlagMutations();
  const isNew = !existing;
  const [draft, setDraft] = useState<Draft>(() => (existing ? draftFrom(existing) : newDraft));
  const [submitted, setSubmitted] = useState(false);

  const { errors, payload, formError } = useMemo(
    () => validate(draft, existing ? routeKey : undefined),
    [draft, existing, routeKey],
  );

  function set<K extends DraftField>(field: K, value: Draft[K]) {
    setDraft((d) => ({ ...d, [field]: value }));
  }

  const status = (field: DraftField) =>
    submitted && errors[field] ? { type: "error" as const, message: errors[field]! } : undefined;

  async function save() {
    setSubmitted(true);
    if (!payload) return;
    try {
      if (isNew) await create.mutateAsync(payload);
      else await update.mutateAsync({ key: routeKey!, flag: payload });
      showToast({ body: isNew ? `Created ${payload.key}` : `Saved ${routeKey}` });
      nav("/");
    } catch (e) {
      showToast({ body: (e as Error).message, type: "error" });
    }
  }

  return (
    <Page>
      <PageHeader
        title={isNew ? "New flag" : `Flag ${routeKey}`}
        description={
          existing
            ? `v${existing.version} · updated ${updatedFmt.format(new Date(existing.updated_at * 1000))}`
            : "Boolean plus optional attr rules, schedule, and rollout."
        }
        actions={<Button label="Back to flags" onClick={() => nav("/")} />}
      />
      {submitted && formError && (
        <Banner status="error" title="Flag not saved" description={formError} />
      )}
      {existing && (
        <MetadataList columns={2} label={{ position: "start", width: 96 }}>
          <MetadataListItem label="Key">{existing.key}</MetadataListItem>
          <MetadataListItem label="Rules">{String(existing.rules.length)}</MetadataListItem>
          <MetadataListItem label="Starts">
            {existing.starts_at ? updatedFmt.format(new Date(existing.starts_at * 1000)) : "—"}
          </MetadataListItem>
          <MetadataListItem label="Ends">
            {existing.ends_at ? updatedFmt.format(new Date(existing.ends_at * 1000)) : "—"}
          </MetadataListItem>
        </MetadataList>
      )}

      <VStack gap={3} maxWidth={560}>
        <Heading level={2}>Basics</Heading>
        {isNew && (
          <TextInput
            label="Key"
            value={draft.key}
            onChange={(v) => set("key", v)}
            isRequired
            width="100%"
            placeholder="new-checkout"
            description="Lowercase, [a-z0-9-_.], immutable once created."
            status={status("key")}
          />
        )}
        <TextInput
          label="Name"
          value={draft.name}
          onChange={(v) => set("name", v)}
          isRequired
          width="100%"
          status={status("name")}
        />
        <TextInput
          label="Description"
          value={draft.description}
          onChange={(v) => set("description", v)}
          isOptional
          width="100%"
          status={status("description")}
        />
      </VStack>

      <VStack gap={3} maxWidth={560}>
        <Heading level={2}>Switch</Heading>
        <Switch
          label="Enabled (kill switch)"
          value={draft.enabled}
          onChange={(v) => set("enabled", v)}
        />
        <Switch
          label="Default value when no rule matches"
          value={draft.default_value}
          onChange={(v) => set("default_value", v)}
        />
      </VStack>

      <VStack gap={3} maxWidth={560}>
        <Heading level={2}>Schedule</Heading>
        <Text type="supporting">
          Your local time ({timeZone}); stored as UTC. Outside this window the flag serves its
          default.
        </Text>
        <TextInput
          label="Starts at"
          value={draft.starts}
          onChange={(v) => set("starts", v)}
          isOptional
          width="100%"
          placeholder="2026-01-01T00:00"
          status={status("starts")}
        />
        <TextInput
          label="Ends at"
          value={draft.ends}
          onChange={(v) => set("ends", v)}
          isOptional
          width="100%"
          placeholder="2026-12-31T23:59"
          status={status("ends")}
        />
      </VStack>

      <VStack gap={2}>
        <Heading level={2}>Rules</Heading>
        <TextArea
          label="Rules JSON"
          value={draft.rulesJson}
          onChange={(v) => set("rulesJson", v)}
          rows={14}
          status={status("rulesJson")}
        />
        <Text type="supporting">
          First match wins. Keep ids unique. rollout_pct 0-100 is sticky per targetingKey. Ops: eq
          ne in not_in contains starts_with ends_with gt lt semver_eq.
        </Text>
      </VStack>

      <VStack gap={2} maxWidth={560}>
        <Button
          label="Save flag"
          variant="primary"
          width="100%"
          isLoading={create.isPending || update.isPending}
          clickAction={save}
        />
        <Button label="Cancel" variant="ghost" width="100%" onClick={() => nav("/")} />
      </VStack>
    </Page>
  );
}

export function FlagDetail() {
  const { key } = useParams();
  const nav = useNavigate();
  const { data, isLoading } = useFlags();
  const existing = data?.flags.find((f) => f.key === key);
  const routeKey = key === "new" ? undefined : key;

  if (!routeKey) return <FlagForm />;

  if (!existing && isLoading) {
    return (
      <Page>
        <Skeleton height={32} width={280} radius={2} />
        <Skeleton height={220} radius={2} />
      </Page>
    );
  }

  if (!existing) {
    return (
      <Page>
        <EmptyState
          title={`No flag named ${routeKey}`}
          description="It may have been deleted. Pick a flag from the list."
          actions={<Button label="Back to flags" variant="primary" onClick={() => nav("/")} />}
        />
      </Page>
    );
  }

  // Remount on load and on every server-side change, so the form never holds
  // pre-load defaults over real data.
  return (
    <FlagForm key={`${existing.key}:${existing.updated_at}`} routeKey={routeKey} existing={existing} />
  );
}
