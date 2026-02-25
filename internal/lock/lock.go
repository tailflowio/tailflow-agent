package lock

import "context"

type Locker interface {
	Acquire(ctx context.Context, key string) (release func(), acquired bool, err error)
}

type Deduplicator interface {
	IsDuplicate(ctx context.Context, eventID string) (bool, error)
}
