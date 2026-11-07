package t3

import (
	"context"
	"log"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"

	"github.com/migueel15/dory/internal/service"
)

// projectSyncInterval paces the discovery of T3 projects that have no Dory project yet.
const projectSyncInterval = 2 * time.Minute

func repoKey(raw string) string { return RepoKey(raw) }

// RepoKey reduces a remote or web URL to host/owner/name, so the https, ssh and
// .git spellings of one repository compare equal.
func RepoKey(raw string) string {
	s := strings.ToLower(strings.TrimSpace(raw))
	for _, p := range []string{"https://", "http://", "ssh://", "git://"} {
		s = strings.TrimPrefix(s, p)
	}
	if at := strings.Index(s, "@"); at >= 0 && at < strings.IndexAny(s+"/", "/:") {
		s = s[at+1:]
	}
	s = strings.Replace(s, ":", "/", 1)
	s = strings.TrimSuffix(strings.TrimSuffix(s, "/"), ".git")
	return s
}

var nonSlug = regexp.MustCompile(`[^a-z0-9]+`)

func slugify(s string) string {
	return strings.Trim(nonSlug.ReplaceAllString(strings.ToLower(s), "-"), "-")
}

func (m *Monitor) runProjectSync(ctx context.Context) {
	tick := time.NewTicker(projectSyncInterval)
	defer tick.Stop()
	for {
		m.syncProjects(ctx)
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
		}
	}
}

// syncProjects creates a Dory project, linked to the same repository, for every
// T3 project of any instance whose repository no Dory project has yet. The same
// repository on several machines yields one project.
func (m *Monitor) syncProjects(ctx context.Context) {
	instances, err := m.svc.T3Instances().List(ctx, service.Filter{Limit: 100})
	if err != nil {
		log.Printf("t3: project sync: list instances: %v", err)
		return
	}
	found := map[string]CatalogProject{}
	var mu sync.Mutex
	var wg sync.WaitGroup
	for _, in := range instances {
		wg.Add(1)
		go func() {
			defer wg.Done()
			c := Client{BaseURL: in.BaseUrl, Token: os.Getenv(in.TokenEnv), HTTP: m.client}
			cat, err := c.Catalog(ctx)
			if err != nil {
				return // an unreachable machine is retried on the next round
			}
			mu.Lock()
			defer mu.Unlock()
			for _, p := range cat.Projects {
				if k := repoKey(p.Repo); p.Repo != "" && k != "" {
					if _, ok := found[k]; !ok {
						found[k] = p
					}
				}
			}
		}()
	}
	wg.Wait()
	if len(found) == 0 {
		return
	}
	repos, err := m.svc.Repositories().List(ctx, service.Filter{Limit: 1000})
	if err != nil {
		log.Printf("t3: project sync: list repositories: %v", err)
		return
	}
	for _, r := range repos {
		delete(found, repoKey(r.RemoteUrl))
	}
	for k, p := range found {
		if m.syncFailed[k] {
			continue
		}
		if err := m.createProject(ctx, p); err != nil {
			// Remembered so a conflict (for example an archived repository with this
			// URL) is logged once instead of every round.
			m.syncFailed[k] = true
			log.Printf("t3: project sync: %s: %v", p.Repo, err)
		}
	}
}

func (m *Monitor) createProject(ctx context.Context, p CatalogProject) error {
	name := p.Title
	if name == "" {
		name = p.RepoName
	}
	base := slugify(p.RepoName)
	if base == "" {
		base = slugify(name)
	}
	owner := ""
	if parts := strings.Split(repoKey(p.Repo), "/"); len(parts) >= 3 {
		owner = slugify(parts[1])
	}
	var project service.Project
	var err error
	// Slugs are unique even among archived projects, so fall back to owner-qualified ones.
	for _, slug := range []string{base, base + "-" + owner, base + "-" + owner + "-t3"} {
		project, err = m.svc.Projects().Put(ctx, "t3-sync", service.Project{Slug: slug, Name: name})
		if err == nil {
			break
		}
	}
	if err != nil {
		return err
	}
	if _, err := m.svc.Repositories().Put(ctx, "t3-sync", service.Repository{ProjectID: project.ID, RemoteUrl: p.Repo}); err != nil {
		// Without its repository the project would not be linked: undo it.
		_ = m.svc.Projects().Archive(ctx, "t3-sync", project.ID)
		return err
	}
	log.Printf("t3: project sync: created project %q for %s", name, p.Repo)
	return nil
}
