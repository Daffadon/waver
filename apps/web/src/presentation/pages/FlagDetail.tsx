/* eslint-disable react/incompatible-library -- no React Compiler in this app; watch() is the sanctioned RHF subscription */
import { useState } from "react";
import { useNavigate, useParams } from "react-router-dom";
import { useForm } from "react-hook-form";
import { Button } from "@astryxdesign/core/Button";
import { Stack } from "@astryxdesign/core/Stack";
import { Text } from "@astryxdesign/core/Text";
import { Heading } from "@astryxdesign/core/Heading";
import { TextInput } from "@astryxdesign/core/TextInput";
import { TextArea } from "@astryxdesign/core/TextArea";
import { Switch } from "@astryxdesign/core/Switch";
import { Banner } from "@astryxdesign/core/Banner";
import { flagSchema, type Flag } from "../../domain/types";
import { datetimeLocalToUnix, unixToDatetimeLocal } from "../../data/client";
import { useFlagMutations, useFlags } from "../../app/hooks";
import { Page } from "../components/Page";

type Form = {
  key: string;
  name: string;
  description: string;
  enabled: boolean;
  default_value: boolean;
  starts: string;
  ends: string;
  rulesJson: string;
};

// Prefilled for new flags. First match wins. Delete what you do not need.
const exampleRules = [
  { id: "pro-users", attr: "plan", op: "eq", values: ["pro"], value: true, rollout_pct: 100 },
  { id: "free-beta", attr: "plan", op: "eq", values: ["free"], value: true, rollout_pct: 10 },
];

export function FlagDetail() {
  const { key } = useParams();
  const nav = useNavigate();
  const isNew = !key || key === "new";
  const { data } = useFlags();
  const { create, update } = useFlagMutations();
  const existing = data?.flags.find((f) => f.key === key);
  const [err, setErr] = useState("");

  const { register, handleSubmit, setValue, watch, formState } = useForm<Form>({
    defaultValues: {
      key: existing?.key ?? "",
      name: existing?.name ?? "",
      description: existing?.description ?? "",
      enabled: existing?.enabled ?? true,
      default_value: existing?.default_value ?? false,
      starts: unixToDatetimeLocal(existing?.starts_at),
      ends: unixToDatetimeLocal(existing?.ends_at),
      rulesJson: JSON.stringify(existing?.rules ?? (isNew ? exampleRules : []), null, 2),
    },
  });

  async function onSubmit(v: Form) {
    setErr("");
    let rules: Flag["rules"];
    try {
      rules = JSON.parse(v.rulesJson || "[]");
    } catch {
      setErr("rules must be valid JSON array");
      return;
    }
    const parsed = flagSchema.safeParse({
      key: isNew ? v.key : key,
      name: v.name,
      description: v.description,
      enabled: v.enabled,
      default_value: v.default_value,
      rules,
      starts_at: datetimeLocalToUnix(v.starts),
      ends_at: datetimeLocalToUnix(v.ends),
    });
    if (!parsed.success) {
      setErr(parsed.error.errors[0]?.message ?? "validation failed");
      return;
    }
    const payload = {
      ...parsed.data,
      description: parsed.data.description ?? "",
      rules: parsed.data.rules ?? [],
    };
    try {
      if (isNew) await create.mutateAsync(payload as Omit<Flag, "version" | "updated_at">);
      else
        await update.mutateAsync({
          key: key!,
          flag: payload as Omit<Flag, "version" | "updated_at">,
        });
      nav("/");
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <Page>
      <Heading level={1}>{isNew ? "New flag" : `Flag ${key}`}</Heading>
      {err && <Banner status="error" title={err} />}
      <Stack gap={3} maxWidth={560}>
        {isNew && (
          <TextInput
            label="Key"
            value={watch("key")}
            onChange={(s) => setValue("key", s)}
            isRequired
            width="100%"
            placeholder="new-checkout"
          />
        )}
        <TextInput
          label="Name"
          value={watch("name")}
          onChange={(s) => setValue("name", s)}
          isRequired
          width="100%"
        />
        <TextInput
          label="Description"
          value={watch("description")}
          onChange={(s) => setValue("description", s)}
          isOptional
          width="100%"
        />
      </Stack>
      <Stack gap={3} maxWidth={560}>
        <Switch
          label="Enabled (kill switch)"
          value={watch("enabled")}
          onChange={(b) => setValue("enabled", b)}
        />
        <Switch
          label="Default value"
          value={watch("default_value")}
          onChange={(b) => setValue("default_value", b)}
        />
      </Stack>
      <Stack gap={3} maxWidth={560}>
        <TextInput
          label="Starts at UTC (optional)"
          type="text"
          value={watch("starts")}
          onChange={(s) => setValue("starts", s)}
          isOptional
          width="100%"
          placeholder="2026-01-01T00:00"
        />
        <TextInput
          label="Ends at UTC (optional)"
          type="text"
          value={watch("ends")}
          onChange={(s) => setValue("ends", s)}
          isOptional
          width="100%"
          placeholder="2026-12-31T23:59"
        />
      </Stack>
      <Stack gap={2}>
        <TextArea
          label="Rules JSON"
          value={watch("rulesJson")}
          onChange={(s) => setValue("rulesJson", s)}
          rows={14}
        />
        <Text type="supporting">
          First match wins. Keep ids unique. rollout_pct 0-100 is sticky per targetingKey. Ops: eq
          ne in not_in contains starts_with ends_with gt lt semver_eq.
        </Text>
      </Stack>
      <Stack gap={2} maxWidth={560}>
        <Button
          label="Save flag"
          variant="primary"
          width="100%"
          isLoading={create.isPending || update.isPending}
          onClick={handleSubmit(onSubmit)}
        />
      </Stack>
      {formState.errors.name && <Text>{formState.errors.name.message}</Text>}
      <input type="hidden" {...register("name")} />
    </Page>
  );
}
