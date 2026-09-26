package module

import (
	"context"
	"database/sql"
	"time"

	"go.uber.org/zap"
)

const leaderLockID int64 = 0x4b75626550696c6f // "KubePilo"; one lock per platform database

// runLeader holds a dedicated PostgreSQL session lock while singleton modules run.
// A standby retries and takes over when the leader connection closes.
func (r *Registry) runLeader(ctx context.Context, db *sql.DB, mods []Module, host *Host) {
	defer close(r.leaderDone)
	for ctx.Err() == nil {
		conn, err := db.Conn(ctx)
		if err == nil {
			var acquired bool
			err = conn.QueryRowContext(ctx, "SELECT pg_try_advisory_lock($1)", leaderLockID).Scan(&acquired)
			if err == nil && acquired {
				r.serveLeader(ctx, conn, mods, host)
			} else if err != nil && ctx.Err() == nil {
				r.logger.Warn("leader election failed", zap.Error(err))
			}
			_ = conn.Close()
		} else if ctx.Err() == nil {
			r.logger.Warn("leader connection failed", zap.Error(err))
		}
		select {
		case <-ctx.Done():
			return
		case <-time.After(3 * time.Second):
		}
	}
}

func (r *Registry) serveLeader(ctx context.Context, conn *sql.Conn, mods []Module, host *Host) {
	started := make([]Module, 0, len(mods))
	defer func() {
		r.leaderActive.Store(false)
		stopCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		for i := len(started) - 1; i >= 0; i-- {
			if err := started[i].Stop(stopCtx); err != nil {
				r.logger.Error("leader module stop failed", zap.String("module", started[i].Meta().Name), zap.Error(err))
			}
		}
		_, _ = conn.ExecContext(stopCtx, "SELECT pg_advisory_unlock($1)", leaderLockID)
	}()
	for _, m := range mods {
		if err := m.Start(ctx, host); err != nil {
			r.logger.Error("leader module start failed", zap.String("module", m.Meta().Name), zap.Error(err))
			return
		}
		started = append(started, m)
	}
	r.leaderActive.Store(true)
	r.logger.Info("singleton modules active on leader")
	ticker := time.NewTicker(time.Second)
	defer ticker.Stop()
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if err := conn.PingContext(ctx); err != nil {
				r.logger.Error("leader connection lost", zap.Error(err))
				return
			}
		}
	}
}
