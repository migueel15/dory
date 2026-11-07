package service

import (
	"context"
	"errors"
	"fmt"
	"testing"
)

type archiveTestService struct {
	Service
	items   archiveTestItems
	threads archiveTestThreads
	txError error
	inTx    bool
}

func (s *archiveTestService) Items() Resource[Item]             { return &s.items }
func (s *archiveTestService) ItemThreads() Resource[ItemThread] { return &s.threads }
func (s *archiveTestService) InTx(ctx context.Context, fn func(Service) error) error {
	s.inTx = true
	err := fn(s)
	s.inTx = false
	if err != nil {
		return err
	}
	return s.txError
}

type archiveTestItems struct {
	Resource[Item]
	archived bool
	err      error
}

func (r *archiveTestItems) Archive(context.Context, string, string) error {
	r.archived = r.err == nil
	return r.err
}

type archiveTestThreads struct {
	Resource[ItemThread]
	links []ItemThread
	err   error
	calls int
}

func (r *archiveTestThreads) List(_ context.Context, f Filter) ([]ItemThread, error) {
	r.calls++
	if f.Fields["item_id"] != "item" || f.IncludeArchived || f.Limit != 1000 {
		return nil, fmt.Errorf("unexpected thread filter: %+v", f)
	}
	if r.err != nil {
		return nil, r.err
	}
	end := min(int(f.Offset+f.Limit), len(r.links))
	return r.links[int(f.Offset):end], nil
}

func TestArchiveItemThreads(t *testing.T) {
	for _, remoteFails := range []bool{false, true} {
		t.Run(fmt.Sprint("remote failure=", remoteFails), func(t *testing.T) {
			base := &archiveTestService{threads: archiveTestThreads{links: []ItemThread{
				{InstanceID: "one", ThreadID: "first"},
				{InstanceID: "two", ThreadID: "second"},
			}}}
			var seen []string
			svc := WithItemThreadArchiving(base, func(_ context.Context, instanceID, threadID string) error {
				if !base.items.archived {
					t.Fatal("remote call before successful item archive")
				}
				seen = append(seen, instanceID+"/"+threadID)
				if remoteFails && threadID == "first" {
					return errors.New("instance offline")
				}
				return nil
			})
			if err := svc.Items().Archive(context.Background(), "actor", "item"); err != nil {
				t.Fatal(err)
			}
			if !base.items.archived || fmt.Sprint(seen) != "[one/first two/second]" {
				t.Fatalf("archived=%v, remote calls=%v", base.items.archived, seen)
			}
		})
	}
}

func TestArchiveItemFailures(t *testing.T) {
	boom := errors.New("database error")
	for _, failArchive := range []bool{false, true} {
		t.Run(fmt.Sprint("archive failure=", failArchive), func(t *testing.T) {
			base := &archiveTestService{}
			if failArchive {
				base.items.err = boom
			} else {
				base.threads.err = boom
			}
			svc := WithItemThreadArchiving(base, func(context.Context, string, string) error {
				t.Fatal("unexpected remote call")
				return nil
			})
			err := svc.Items().Archive(context.Background(), "actor", "item")
			if failArchive && (!errors.Is(err, boom) || base.threads.calls != 0) {
				t.Fatalf("failed archive: err=%v, list calls=%d", err, base.threads.calls)
			}
			if !failArchive && (err != nil || !base.items.archived) {
				t.Fatalf("list failure prevented archive: %v", err)
			}
		})
	}
}

func TestArchiveItemThreadsPagination(t *testing.T) {
	base := &archiveTestService{threads: archiveTestThreads{links: make([]ItemThread, 1001)}}
	calls := 0
	svc := WithItemThreadArchiving(base, func(context.Context, string, string) error {
		calls++
		return nil
	})
	if err := svc.Items().Archive(context.Background(), "actor", "item"); err != nil {
		t.Fatal(err)
	}
	if calls != 1001 || base.threads.calls != 2 {
		t.Fatalf("remote calls=%d, list calls=%d", calls, base.threads.calls)
	}
}

func TestArchiveItemThreadsAfterCommit(t *testing.T) {
	boom := errors.New("rollback or commit failure")
	for _, outcome := range []string{"commit", "rollback", "commit failure"} {
		t.Run(outcome, func(t *testing.T) {
			base := &archiveTestService{threads: archiveTestThreads{links: []ItemThread{{ThreadID: "thread"}}}}
			if outcome == "commit failure" {
				base.txError = boom
			}
			calls := 0
			svc := WithItemThreadArchiving(base, func(context.Context, string, string) error {
				if base.inTx {
					t.Fatal("remote archive before transaction commit")
				}
				calls++
				return nil
			})
			err := svc.InTx(context.Background(), func(tx Service) error {
				if err := tx.Items().Archive(context.Background(), "actor", "item"); err != nil {
					return err
				}
				if outcome == "rollback" {
					return boom
				}
				return nil
			})
			if outcome == "commit" {
				if err != nil || calls != 1 {
					t.Fatalf("committed: err=%v, calls=%d", err, calls)
				}
			} else if !errors.Is(err, boom) || calls != 0 || base.threads.calls != 0 {
				t.Fatalf("failed transaction: err=%v, remote calls=%d, list calls=%d", err, calls, base.threads.calls)
			}
		})
	}
}
