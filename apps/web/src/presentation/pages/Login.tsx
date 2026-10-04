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

export function Login() {
  const nav = useNavigate();
  const [pw, setPw] = useState("");
  const [err, setErr] = useState("");
  const [busy, setBusy] = useState(false);

  async function submit() {
    setBusy(true);
    setErr("");
    try {
      const r = await authRepo.login(pw);
      tokenStore.set(r.token);
      nav("/");
    } catch (e) {
      const msg = (e as Error).message;
      if (msg.includes("MUST_CHANGE")) nav("/change-password");
      else setErr(msg);
    } finally {
      setBusy(false);
    }
  }

  return (
    <AuthShell>
      <Heading level={1}>Sign in</Heading>
      <Text type="supporting">Single admin. First login uses the generated password.</Text>
      {err && <Banner status="error" title={err} />}
      <TextInput
        label="Password"
        type="password"
        value={pw}
        onChange={setPw}
        onEnter={submit}
        isRequired
        width="100%"
      />
      <Stack gap={2} paddingBlockStart={2}>
        <Button label="Sign in" variant="primary" width="100%" isLoading={busy} onClick={submit} />
      </Stack>
    </AuthShell>
  );
}
