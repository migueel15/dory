// Hooks of projects, repositories, folders of a node and memories.
import { useQuery } from "@tanstack/react-query";
import { api } from "../client";
import { useCall,type MOpts,type Of,type Opts } from "./hooks";
import { qk } from "./keys";

export const useProject = (id: string, o?: Opts<Of<typeof api.projects.get>>) =>
  useQuery({ queryKey: qk.project(id), queryFn: () => api.projects.get(id), ...o });
export const useProjectRepositories = (id: string, o?: Opts<Of<typeof api.repositories.list>>) =>
  useQuery({ queryKey: qk.repositoriesOf(id), queryFn: () => api.repositories.list({ project_id: id }), ...o });
export const useRepositoryPaths = (o?: Opts<Of<typeof api.repositoryPaths>>) =>
  useQuery({ queryKey: qk.repositoryPaths(), queryFn: api.repositoryPaths, ...o });
export const useFolders = (node: string | undefined, dir: string, hidden: boolean, o?: Opts<Of<typeof api.folders>>) =>
  useQuery({ queryKey: qk.folders(node, dir, hidden), queryFn: () => api.folders(node!, dir, hidden), ...o });
export const useCheckout = (node: string | undefined, path: string | undefined, o?: Opts<Of<typeof api.checkout>>) =>
  useQuery({ queryKey: qk.checkout(node, path), queryFn: () => api.checkout(node!, path!), ...o });

export const useCreateRepository = (project: string, remote: string, o?: MOpts<Of<typeof api.repositories.create>, void>) =>
  useCall(() => api.repositories.create({ project_id: project, remote_url: remote }), o);
export const useArchiveProject = (id: string, o?: MOpts<Of<typeof api.projects.archive>, void>) => useCall(() => api.projects.archive(id), o);
export const useSetRepositoryPath = (id: string, path: string, o?: MOpts<void, void>) => useCall(() => api.setRepositoryPath(id, path), o);

type ProjectFields = { name: string; slug: string; description: string };
// useAddProject creates a project, from a repository of a node when it has a
// remote (one project per repository, whatever node adds it); it gives the id.
export const useAddProject = (fields: () => ProjectFields & { remote: string; branch: string; node: string; path: string }, o?: MOpts<string, void>) =>
  useCall(async () => {
    const { remote, branch, node, path, ...project } = fields();
    if (!remote) return (await api.projects.create(project)).id;
    const r = await api.projectFromRepository({ ...project, remote_url: remote, default_branch: branch || "main" });
    await api.setNodeRepositoryPath(node, r.id, path);
    return r.project_id;
  }, o);
// useSaveProject creates or updates a project and adds a repository to it.
export const useSaveProject = (id: string | undefined, fields: ProjectFields, remote: string, path: string, o?: MOpts<Of<typeof api.projects.create>, void>) =>
  useCall(async () => {
    const p = id ? await api.projects.update(id, fields) : await api.projects.create(fields);
    if (remote) {
      const r = await api.repositories.create({ project_id: p.id, remote_url: remote });
      if (path) await api.setRepositoryPath(r.id, path);
    }
    return p;
  }, o);

export const useSaveMemory = (id: string | undefined, body: () => Parameters<typeof api.memories.create>[0], o?: MOpts<Of<typeof api.memories.create>, void>) =>
  useCall(() => (id ? api.memories.update(id, body()) : api.memories.create(body())), o);
export const useArchiveMemory = (id: string | undefined, o?: MOpts<Of<typeof api.memories.archive>, void>) => useCall(() => api.memories.archive(id!), o);
