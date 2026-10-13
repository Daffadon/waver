import { QueryClient, useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
import { flagRepo, keyRepo } from "../data/client";
import type { FlagInput } from "../domain/types";

export const queryClient = new QueryClient({
  defaultOptions: { queries: { retry: 1, staleTime: 30_000 } },
});

export function useFlags() {
  return useQuery({ queryKey: ["flags"], queryFn: () => flagRepo.list() });
}

export function useFlagMutations() {
  const qc = useQueryClient();
  const invalidate = () => qc.invalidateQueries({ queryKey: ["flags"] });
  const create = useMutation({ mutationFn: flagRepo.create, onSuccess: invalidate });
  const update = useMutation({
    mutationFn: ({ key, flag }: { key: string; flag: FlagInput }) =>
      flagRepo.update(key, flag),
    onSuccess: invalidate,
  });
  const remove = useMutation({ mutationFn: flagRepo.remove, onSuccess: invalidate });
  return { create, update, remove };
}

export function useApiKeys() {
  const qc = useQueryClient();
  const list = useQuery({ queryKey: ["apikeys"], queryFn: () => keyRepo.list() });
  const create = useMutation({
    mutationFn: keyRepo.create,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["apikeys"] }),
  });
  const revoke = useMutation({
    mutationFn: keyRepo.revoke,
    onSuccess: () => qc.invalidateQueries({ queryKey: ["apikeys"] }),
  });
  return { list, create, revoke };
}
