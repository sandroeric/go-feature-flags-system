package sync

import (
	"context"
	"errors"
	"sync"
	"testing"
	"time"

	"launchdarkly/internal/domain"
	"launchdarkly/internal/store"

	"github.com/jackc/pgx/v5/pgconn"
)

type stubNotificationConnector struct {
	mu    sync.Mutex
	conns []notificationConn
	errs  []error
	calls int
}

func (s *stubNotificationConnector) Connect(ctx context.Context) (notificationConn, error) {
	s.mu.Lock()
	defer s.mu.Unlock()

	idx := s.calls
	s.calls++

	if idx < len(s.errs) && s.errs[idx] != nil {
		return nil, s.errs[idx]
	}
	if idx >= len(s.conns) {
		return nil, errors.New("no connection configured")
	}

	return s.conns[idx], nil
}

type stubNotificationConn struct {
	notifications chan *pgconn.Notification
	errs          chan error
	closeCount    int
	closeErr      error
}

func newStubNotificationConn() *stubNotificationConn {
	return &stubNotificationConn{
		notifications: make(chan *pgconn.Notification, 16),
		errs:          make(chan error, 4),
	}
}

func (s *stubNotificationConn) Exec(ctx context.Context, sql string, args ...any) (pgconn.CommandTag, error) {
	return pgconn.CommandTag{}, nil
}

func (s *stubNotificationConn) WaitForNotification(ctx context.Context) (*pgconn.Notification, error) {
	select {
	case <-ctx.Done():
		return nil, ctx.Err()
	case err := <-s.errs:
		return nil, err
	case notification := <-s.notifications:
		return notification, nil
	}
}

func (s *stubNotificationConn) Close(ctx context.Context) error {
	s.closeCount++
	return s.closeErr
}

func TestRealtimeListenerDebouncesNotifications(t *testing.T) {
	repo := &mockRepository{flags: []domain.Flag{testFlag("debounce")}}
	holder := &mockHolder{current: store.Empty()}
	syncer := NewSyncer(repo, holder)

	conn := newStubNotificationConn()
	connector := &stubNotificationConnector{conns: []notificationConn{conn}}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := startRealtimeListenerWithConnector(ctx, connector, syncer, 25*time.Millisecond, 10*time.Millisecond)

	conn.notifications <- &pgconn.Notification{Channel: flagsUpdatedChannel}
	conn.notifications <- &pgconn.Notification{Channel: flagsUpdatedChannel}
	conn.notifications <- &pgconn.Notification{Channel: flagsUpdatedChannel}

	waitForCondition(t, time.Second, func() bool {
		return holder.Current().Generation() == 1
	})

	time.Sleep(50 * time.Millisecond)
	if holder.Current().Generation() != 1 {
		t.Fatalf("generation = %d, want 1", holder.Current().Generation())
	}

	cancel()
	<-done
}

func TestRealtimeListenerReconnectsAfterDisconnect(t *testing.T) {
	repo := &mockRepository{flags: []domain.Flag{testFlag("reconnect")}}
	holder := &mockHolder{current: store.Empty()}
	syncer := NewSyncer(repo, holder)

	firstConn := newStubNotificationConn()
	secondConn := newStubNotificationConn()
	connector := &stubNotificationConnector{
		conns: []notificationConn{firstConn, secondConn},
	}

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	done := startRealtimeListenerWithConnector(ctx, connector, syncer, 10*time.Millisecond, 10*time.Millisecond)

	firstConn.errs <- errors.New("connection dropped")

	waitForCondition(t, time.Second, func() bool {
		connector.mu.Lock()
		defer connector.mu.Unlock()
		return connector.calls >= 2
	})

	secondConn.notifications <- &pgconn.Notification{Channel: flagsUpdatedChannel}

	waitForCondition(t, time.Second, func() bool {
		return holder.Current().Generation() == 1
	})

	cancel()
	<-done
}

func waitForCondition(t *testing.T, timeout time.Duration, fn func() bool) {
	t.Helper()

	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		if fn() {
			return
		}
		time.Sleep(10 * time.Millisecond)
	}

	t.Fatal("condition not met before timeout")
}

func testFlag(key string) domain.Flag {
	return domain.Flag{
		Key:     key,
		Enabled: true,
		Default: "on",
		Variants: []domain.Variant{
			{Name: "on", Weight: 50},
			{Name: "off", Weight: 50},
		},
	}
}
