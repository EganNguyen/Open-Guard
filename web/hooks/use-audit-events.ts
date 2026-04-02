"use client";

import { useInfiniteQuery } from "@tanstack/react-query";
import { getAuditEvents, AuditEvent } from "@/lib/api";

export function useAuditEvents() {
  return useInfiniteQuery({
    queryKey: ["audit", "events"],
    queryFn: ({ pageParam }) => getAuditEvents({ cursor: pageParam as string }),
    initialPageParam: undefined as string | undefined,
    getNextPageParam: (lastPage) => lastPage.meta.next_cursor || undefined,
  });
}
