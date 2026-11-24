import { get, post, put, resource } from "./http";
import type { Project, Repository, RepositoryLocation } from "./types.gen";

export const projectsApi = {
  projects: resource<Project, Partial<Project>>("/projects"),
  repositories: resource<Repository, Partial<Repository>>("/repositories"),
  repositoryLocations: () => get<RepositoryLocation[]>("/repository-locations"),
  projectFromRepository: (body: { name: string; slug: string; description: string; remote_url: string; default_branch: string }) => post<Repository>("/projects/from-repository", body),
  repositoryPaths: () => get<Record<string, string>>("/repository-paths"),
  setRepositoryPath: (id: string, path: string) => put<void>(`/repositories/${id}/path`, { path }),
};
