import { useState } from "react";
import { useNavigate } from "react-router-dom";
import { Button } from "@astryxdesign/core/Button";
import { Stack } from "@astryxdesign/core/Stack";
import { Text } from "@astryxdesign/core/Text";
import { Heading } from "@astryxdesign/core/Heading";
import { TextInput } from "@astryxdesign/core/TextInput";
import { Banner } from "@astryxdesign/core/Banner";
import { AuthShell } from "../components/AuthShell";
import { authRepo, tokenStore } from "../../data/client";

export function ChangePassword() {
  const nav = useNavigate();
  const [current, setCurrent] = useState("");
  const [next, setNext] = useState("");
  const [err, setErr] = useState("");

  async function submit() {
    setErr("");
    if (next.length < 12) {
      setErr("next must be 12-128 chars");
      return;
    }
    try {
      await authRepo.changePassword(current, next);
      tokenStore.clear();
      nav("/login");
    } catch (e) {
      setErr((e as Error).message);
    }
  }

  return (
    <AuthShell>
      <Heading level={1}>Change password</Heading>
      <Text type="supporting">Required on first login. New password min 12 chars.</Text>
      {err && <Banner status="error" title={err} />}
      <TextInput
        label="Current (generated) password"
        type="password"
        value={current}
        onChange={setCurrent}
        isRequired
        width="100%"
      />
      <TextInput
        label="New password"
        type="password"
        value={next}
        onChange={setNext}
        onEnter={submit}
        isRequired
        width="100%"
        description="Min 12 chars."
      />
      <Stack gap={2} paddingBlockStart={2}>
        <Button label="Save new password" variant="primary" width="100%" onClick={submit} />
        <Button
          label="Back to sign in"
          variant="ghost"
          width="100%"
          onClick={() => nav("/login")}
        />
      </Stack>
    </AuthShell>
  );
}
