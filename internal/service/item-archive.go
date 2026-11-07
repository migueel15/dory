package service

import (
	"context"
	"log"
)

// WithItemThreadArchiving archives linked T3 threads after an item archive is
// committed. archive resolves the instance and dispatches the remote command.
// Remote failures are logged and never change the result of the item archive.
func WithItemThreadArchiving(svc Service, archive func(ctx context.Context, instanceID, threadID string) error) Service {
	s := archivingService{Service: svc}
	s.afterArchive = func(ctx context.Context, id string) {
		const pageSize = 1000
		for offset := int32(0); ; offset += pageSize {
			links, err := svc.ItemThreads().List(ctx, Filter{
				Fields: map[string]any{"item_id": id}, Limit: pageSize, Offset: offset,
			})
			if err != nil {
				log.Printf("archive item %s: list threads: %v", id, err)
				return
			}
			for _, link := range links {
				if err := archive(ctx, link.InstanceID, link.ThreadID); err != nil {
					log.Printf("archive item %s: instance %s thread %s: %v", id, link.InstanceID, link.ThreadID, err)
				}
			}
			if len(links) < pageSize {
				return
			}
		}
	}
	return s
}

type archivingService struct {
	Service
	afterArchive func(context.Context, string)
}

func (s archivingService) Items() Resource[Item] {
	return archivingItems{Resource: s.Service.Items(), afterArchive: s.afterArchive}
}

// InTx defers remote effects until the outer transaction has committed, so a
// rollback (including a failed commit) cannot hide threads in T3.
func (s archivingService) InTx(ctx context.Context, fn func(Service) error) error {
	var archived []string
	err := s.Service.InTx(ctx, func(tx Service) error {
		return fn(archivingService{Service: tx, afterArchive: func(_ context.Context, id string) {
			archived = append(archived, id)
		}})
	})
	if err == nil {
		for _, id := range archived {
			s.afterArchive(ctx, id)
		}
	}
	return err
}

type archivingItems struct {
	Resource[Item]
	afterArchive func(context.Context, string)
}

func (r archivingItems) Archive(ctx context.Context, actor, id string) error {
	if err := r.Resource.Archive(ctx, actor, id); err != nil {
		return err
	}
	r.afterArchive(ctx, id)
	return nil
}
