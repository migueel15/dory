// Hooks of GitHub: sign-in, the overview, details, diffs and pull request reviews.
import { useInfiniteQuery,useQuery } from "@tanstack/react-query";
import { api } from "../client";
import { useCall,type MOpts,type Of,type Opts } from "./hooks";
import { qk } from "./keys";

export const useGithubAuth = (o?: Opts<Of<typeof api.githubAuth>>) =>
  useQuery({ queryKey: qk.githubAuth(), queryFn: api.githubAuth, ...o });
export const useGithubDevice = (o?: Opts<Of<typeof api.githubDeviceState>>) =>
  useQuery({ queryKey: qk.githubDevice(), queryFn: api.githubDeviceState, ...o });
export const useGithubOverview = (o?: Opts<Of<typeof api.github>>) =>
  useQuery({ queryKey: qk.github(), queryFn: () => api.github(), ...o });
export const useGithubDetail = (repo: string, kind: string, number: number, o?: Opts<Of<typeof api.githubDetail>>) =>
  useQuery({ queryKey: qk.githubDetail(repo, kind, number), queryFn: () => api.githubDetail(repo, kind, number), ...o });
export const useGithubDiff = (repo: string, number: number, o?: Opts<Of<typeof api.githubDiff>>) =>
  useQuery({ queryKey: qk.githubDiff(repo, number), queryFn: () => api.githubDiff(repo, number), ...o });
export const usePullReviews = (repo: string, number: number, o?: Opts<Of<typeof api.pullReviews>>) =>
  useQuery({ queryKey: qk.pullReviews(repo, number), queryFn: () => api.pullReviews(repo, number), ...o });
export const usePullAgent = (repo: string, number: number, o?: Opts<Of<typeof api.pullAgent>>) =>
  useQuery({ queryKey: qk.pullAgent(repo, number), queryFn: () => api.pullAgent(repo, number), ...o });
export const useGithubClosed = (kind: "pulls" | "issues", project: string, o?: { enabled?: boolean; staleTime?: number }) =>
  useInfiniteQuery({
    queryKey: qk.githubClosed(kind, project),
    queryFn: ({ pageParam }) => api.githubClosed(kind, project, pageParam),
    initialPageParam: "",
    getNextPageParam: (last) => last.cursor || undefined,
    ...o,
  });
export const useSearchTasks = (text: string, o?: Opts<Of<typeof api.searchTasks>>) =>
  useQuery({ queryKey: qk.paletteTasks(text), queryFn: ({ signal }) => api.searchTasks(text, signal), ...o });
export const useSearchGithub = (text: string, o?: Opts<Of<typeof api.searchGithub>>) =>
  useQuery({ queryKey: qk.paletteGithub(text), queryFn: ({ signal }) => api.searchGithub(text, signal), ...o });

export const useGithubLogout = (o?: MOpts<Of<typeof api.githubLogout>, void>) => useCall(() => api.githubLogout(), o);
export const useGithubDeviceStart = (o?: MOpts<Of<typeof api.githubDeviceStart>, void>) => useCall(() => api.githubDeviceStart(), o);
export const useGithubAct = <V = Parameters<typeof api.githubAct>[3]>(repo: string, kind: string, number: number, action: (v: V) => Parameters<typeof api.githubAct>[3], o?: MOpts<void, V>) =>
  useCall((v: V) => api.githubAct(repo, kind, number, action(v)), o);
export const useCreatePullReview = (repo: string, number: number, o?: MOpts<Of<typeof api.createPullReview>, Parameters<typeof api.createPullReview>[2]>) =>
  useCall((launch: Parameters<typeof api.createPullReview>[2]) => api.createPullReview(repo, number, launch), o);
// usePullReviewMark marks a file of a pull request as reviewed at its patch, or unmarks it.
export const usePullReviewMark = (repo: string, number: number, hashOf: (path: string) => string, o?: MOpts<void, { path: string; on: boolean }>) =>
  useCall(({ path, on }: { path: string; on: boolean }) => (on ? api.markPullReview(repo, number, path, hashOf(path)) : api.unmarkPullReview(repo, number, path)), o);
export const useAskPullAgent = <V>(repo: string, number: number, body: (v: V) => Parameters<typeof api.askPullAgent>[2], o?: MOpts<Of<typeof api.askPullAgent>, V>) =>
  useCall((v: V) => api.askPullAgent(repo, number, body(v)), o);
