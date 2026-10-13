import { z } from "zod";

export const ruleSchema = z.object({
  id: z.string().min(1).max(64),
  attr: z.string().min(1).max(64),
  op: z.enum([
    "eq",
    "ne",
    "in",
    "not_in",
    "contains",
    "starts_with",
    "ends_with",
    "gt",
    "lt",
    "semver_eq",
  ]),
  values: z.array(z.string().max(256)).min(1).max(20),
  value: z.boolean(),
  rollout_pct: z.number().int().min(0).max(100),
});

export const flagSchema = z
  .object({
    key: z
      .string()
      .regex(/^[a-z0-9][a-z0-9\-_.]*$/, "use [a-z0-9-_.]")
      .max(128),
    name: z.string().min(1).max(120),
    description: z.string().max(500),
    enabled: z.boolean(),
    default_value: z.boolean(),
    rules: z.array(ruleSchema),
    starts_at: z.number().int().nullable().optional(),
    ends_at: z.number().int().nullable().optional(),
  })
  .refine((v) => v.starts_at == null || v.ends_at == null || v.starts_at <= v.ends_at, {
    message: "schedule end must come after its start",
  });

export const loginSchema = z.object({ password: z.string().min(1) });
export const changePasswordSchema = z.object({
  current: z.string().min(1),
  next: z.string().min(12).max(128),
});
export const apiKeySchema = z.object({ name: z.string().min(1).max(64) });

export type Rule = z.infer<typeof ruleSchema>;
export type Flag = z.infer<typeof flagSchema> & { version: number; updated_at: number };
export type ApiKey = {
  id: string;
  prefix: string;
  name: string;
  revoked_at?: number | null;
  created_at: number;
};
