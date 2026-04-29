package sync

import (
	"context"
	"errors"
	"log/slog"
	"time"

	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgconn"
)

const (
	flagsUpdatedChannel     = "flags_updated"
	defaultRealtimeDebounce = 100 * time.Millisecond
	defaultReconnectDelay   = time.Second
)

type notificationConn interface {
	Exec(context.Context, string, ...any) (pgconn.CommandTag, error)
	WaitForNotification(context.Context) (*pgconn.Notification, error)
	Close(context.Context) error
}

type notificationConnector interface {
	Connect(context.Context) (notificationConn, error)
}

type pgxNotificationConnector struct {
	databaseURL string
}

func (c pgxNotificationConnector) Connect(ctx context.Context) (notificationConn, error) {
	cfg, err := pgx.ParseConfig(c.databaseURL)
	if err != nil {
		return nil, err
	}

	return pgx.ConnectConfig(ctx, cfg)
}

// StartRealtimeListener listens for Postgres notifications and refreshes the
// in-memory store while polling remains active as a fallback.
func StartRealtimeListener(ctx context.Context, databaseURL string, syncer *Syncer) <-chan struct{} {
	return startRealtimeListenerWithConnector(ctx, pgxNotificationConnector{databaseURL: databaseURL}, syncer, defaultRealtimeDebounce, defaultReconnectDelay)
}

func startRealtimeListenerWithConnector(ctx context.Context, connector notificationConnector, syncer *Syncer, debounce, reconnectDelay time.Duration) <-chan struct{} {
	done := make(chan struct{})

	if debounce <= 0 {
		debounce = defaultRealtimeDebounce
	}
	if reconnectDelay <= 0 {
		reconnectDelay = defaultReconnectDelay
	}

	go func() {
		defer close(done)

		runner := realtimeListener{
			connector:      connector,
			syncer:         syncer,
			debounce:       debounce,
			reconnectDelay: reconnectDelay,
			channel:        flagsUpdatedChannel,
		}
		runner.run(ctx)
	}()

	return done
}

type realtimeListener struct {
	connector      notificationConnector
	syncer         *Syncer
	debounce       time.Duration
	reconnectDelay time.Duration
	channel        string
}

func (l realtimeListener) run(ctx context.Context) {
	var timer *time.Timer
	var timerC <-chan time.Time

	stopTimer := func() {
		if timer == nil {
			return
		}
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		timer = nil
		timerC = nil
	}

	for {
		if ctx.Err() != nil {
			stopTimer()
			return
		}

		conn, err := l.connectAndListen(ctx)
		if err != nil {
			slog.Error("realtime listener connection failed", "error", err)
			if !sleepContext(ctx, l.reconnectDelay) {
				stopTimer()
				return
			}
			continue
		}

		slog.Info("realtime listener connected", "channel", l.channel)

		notifications := make(chan struct{}, 1)
		errs := make(chan error, 1)
		go waitForNotifications(ctx, conn, l.channel, notifications, errs)

	connected:
		for {
			select {
			case <-ctx.Done():
				stopTimer()
				closeNotificationConn(conn)
				return
			case <-notifications:
				if timer != nil {
					continue
				}
				timer = time.NewTimer(l.debounce)
				timerC = timer.C
			case <-timerC:
				stopTimer()
				if err := l.syncer.Sync(ctx); err != nil {
					slog.Error("realtime refresh failed", "error", err)
				}
			case err := <-errs:
				if ctx.Err() != nil {
					stopTimer()
					closeNotificationConn(conn)
					return
				}
				slog.Warn("realtime listener disconnected, reconnecting", "error", err)
				closeNotificationConn(conn)
				break connected
			}
		}

		if !sleepContext(ctx, l.reconnectDelay) {
			stopTimer()
			return
		}
	}
}

func (l realtimeListener) connectAndListen(ctx context.Context) (notificationConn, error) {
	conn, err := l.connector.Connect(ctx)
	if err != nil {
		return nil, err
	}

	if _, err := conn.Exec(ctx, "listen "+l.channel); err != nil {
		closeNotificationConn(conn)
		return nil, err
	}

	return conn, nil
}

func waitForNotifications(ctx context.Context, conn notificationConn, channel string, notifications chan<- struct{}, errs chan<- error) {
	for {
		notification, err := conn.WaitForNotification(ctx)
		if err != nil {
			if ctx.Err() != nil || errors.Is(err, context.Canceled) {
				return
			}

			select {
			case errs <- err:
			default:
			}
			return
		}

		if notification == nil || notification.Channel != channel {
			continue
		}

		select {
		case notifications <- struct{}{}:
		default:
		}
	}
}

func closeNotificationConn(conn notificationConn) {
	if conn == nil {
		return
	}

	closeCtx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	if err := conn.Close(closeCtx); err != nil {
		slog.Warn("failed to close realtime listener connection", "error", err)
	}
}

func sleepContext(ctx context.Context, delay time.Duration) bool {
	timer := time.NewTimer(delay)
	defer timer.Stop()

	select {
	case <-ctx.Done():
		return false
	case <-timer.C:
		return true
	}
}
