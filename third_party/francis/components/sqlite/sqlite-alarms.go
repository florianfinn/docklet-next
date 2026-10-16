// Source: italypaleale/francis/components/sqlite/sqlite-alarms.go
// Revision: 23cd46831a543a73eec05ad034afd46245225876
// Retrieved: 2026-10-16
// License: MIT
// Deviations: Pinned provider; alarm fetching split into a separate file to meet the size limit; whitespace normalized.

package sqlite

import (
	"context"
	"database/sql"
	"errors"
	"fmt"
	"github.com/italypaleale/francis/components"
	"github.com/italypaleale/francis/internal/ref"
	sqltransactions "github.com/italypaleale/go-sql-utils/transactions/sql"
	"strings"
	"time"
	"uuid"
)

func (s *SQLiteProvider) GetAlarm(ctx context.Context, req ref.AlarmRef) (res components.GetAlarmRes, err error) {
	queryCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var (
		dueTime   int64
		interval  *string
		cron      *string
		jobMethod *string
		kind      string
		ttlTime   *int64
	)
	// #nosec G202 -- the only concatenated value is the static table prefix, not user input
	err = s.db.
		QueryRowContext(queryCtx, `
			SELECT
				alarm_due_time, alarm_interval, alarm_cron, alarm_data, alarm_ttl_time, alarm_kind, job_method
			FROM `+s.tablePrefix+`alarms
			WHERE
				actor_type = ?
				AND actor_id = ?
				AND alarm_name = ?`,
			req.ActorType, req.ActorID, req.Name,
		).
		Scan(&dueTime, &interval, &cron, &res.Data, &ttlTime, &kind, &jobMethod)

	if errors.Is(err, sql.ErrNoRows) {
		return res, components.ErrNoAlarm
	} else if err != nil {
		return res, fmt.Errorf("error executing query: %w", err)
	}

	res.DueTime = time.UnixMilli(dueTime)
	if interval != nil {
		res.Interval = *interval
	}
	if cron != nil {
		res.Cron = *cron
	}
	res.Kind = components.AlarmKind(kind)
	if jobMethod != nil {
		res.JobMethod = *jobMethod
	}
	if ttlTime != nil {
		res.TTL = new(time.UnixMilli(*ttlTime))
	}

	return res, nil
}

type setAlarmResult struct {
	alarmID             string
	dueTime             int64
	leaseID             *string
	leaseExpirationTime *int64
}

func (s *SQLiteProvider) SetAlarm(ctx context.Context, alarmRef ref.AlarmRef, req components.SetAlarmReq) (*ref.AlarmLease, error) {
	var (
		interval *string
		ttlTime  *int64
	)
	if req.Interval != "" {
		interval = &req.Interval
	}
	if req.TTL != nil {
		ttlTime = new(req.TTL.UnixMilli())
	}

	if req.Data != nil && len(req.Data) == 0 {
		req.Data = nil
	}

	alarmID := uuid.NewV7().String()

	queryCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Trying to acquire a lease requires using the slower path
	// We skip that when there are no allowed hosts or when the local clock places the alarm outside of the fetch-ahead interval
	if len(req.LeaseImmediate) > 0 && !req.DueTime.After(s.clock.Now().Add(s.cfg.AlarmsFetchAheadInterval)) {
		return s.setAndLeaseAlarm(queryCtx, alarmRef, req, alarmID, interval, ttlTime)
	}

	// The fast path uses the same upsert but ignores the returned row
	_, _, err := s.setAlarm(queryCtx, s.db, alarmRef, req, alarmID, interval, ttlTime)
	if err != nil {
		return nil, err
	}
	return nil, nil
}

// setAlarm stores or replaces an alarm through either a database handle or an existing transaction
func (s *SQLiteProvider) setAlarm(ctx context.Context, q querier, alarmRef ref.AlarmRef, req components.SetAlarmReq, alarmID string, interval *string, ttlTime *int64) (res setAlarmResult, returned bool, err error) {
	// Store or replace the alarm while preserving a lease when the properties are identical
	// Return the resulting row directly so inserts and replacements need no follow-up query
	// #nosec G202 -- the only concatenated value is the static table prefix, not user input
	err = q.
		QueryRowContext(ctx,
			`INSERT INTO `+s.tablePrefix+`alarms
				(alarm_id, actor_type, actor_id, alarm_name,
				alarm_due_time, alarm_interval, alarm_ttl_time, alarm_data,
				alarm_lease_id, alarm_lease_expiration_time)
			VALUES
				(?, ?, ?, ?, ?, ?, ?, ?, NULL, NULL)
			ON CONFLICT (actor_type, actor_id, alarm_name) DO UPDATE SET
				alarm_id = EXCLUDED.alarm_id,
				alarm_due_time = EXCLUDED.alarm_due_time,
				alarm_interval = EXCLUDED.alarm_interval,
				alarm_ttl_time = EXCLUDED.alarm_ttl_time,
				alarm_data = EXCLUDED.alarm_data,
				alarm_lease_id = NULL,
				alarm_lease_expiration_time = NULL
			WHERE
				`+s.tablePrefix+`alarms.alarm_due_time != EXCLUDED.alarm_due_time
				OR `+s.tablePrefix+`alarms.alarm_interval IS NOT EXCLUDED.alarm_interval
				OR `+s.tablePrefix+`alarms.alarm_ttl_time IS NOT EXCLUDED.alarm_ttl_time
				OR `+s.tablePrefix+`alarms.alarm_data IS NOT EXCLUDED.alarm_data
			RETURNING alarm_id, alarm_due_time, alarm_lease_id, alarm_lease_expiration_time`,
			alarmID, alarmRef.ActorType, alarmRef.ActorID, alarmRef.Name,
			req.DueTime.UnixMilli(), interval, ttlTime, req.Data,
		).
		Scan(&res.alarmID, &res.dueTime, &res.leaseID, &res.leaseExpirationTime)
	if errors.Is(err, sql.ErrNoRows) {
		return res, false, nil
	} else if err != nil {
		return res, false, fmt.Errorf("failed to create alarm: %w", err)
	}
	return res, true, nil
}

// setAndLeaseAlarm atomically stores, places, and leases an alarm
func (s *SQLiteProvider) setAndLeaseAlarm(ctx context.Context, alarmRef ref.AlarmRef, req components.SetAlarmReq, alarmID string, interval *string, ttlTime *int64) (*ref.AlarmLease, error) {
	return sqltransactions.ExecuteInTransaction(ctx, s.log, s.db, func(ctx context.Context, tx *sql.Tx) (*ref.AlarmLease, error) {
		// Store the alarm and capture its current lease state in the same statement
		stored, returned, err := s.setAlarm(ctx, tx, alarmRef, req, alarmID, interval, ttlTime)
		if err != nil {
			return nil, err
		}
		if !returned {
			// SQLite suppresses RETURNING for an identical upsert, so inspect the unchanged row while still holding the write transaction
			// #nosec G202 -- the only concatenated value is the static table prefix, not user input
			err = tx.
				QueryRowContext(ctx,
					`SELECT alarm_id, alarm_due_time, alarm_lease_id, alarm_lease_expiration_time
					FROM `+s.tablePrefix+`alarms
					WHERE actor_type = ? AND actor_id = ? AND alarm_name = ?`,
					alarmRef.ActorType, alarmRef.ActorID, alarmRef.Name,
				).
				Scan(&stored.alarmID, &stored.dueTime, &stored.leaseID, &stored.leaseExpirationTime)
			if err != nil {
				return nil, fmt.Errorf("failed to inspect unchanged alarm: %w", err)
			}
		}

		// Reject rows that became ineligible before actor placement
		now := s.clock.Now()
		if stored.dueTime > now.Add(s.cfg.AlarmsFetchAheadInterval).UnixMilli() {
			// Alarm is not due within the lookahead interval
			return nil, nil
		}
		if stored.leaseID != nil && stored.leaseExpirationTime != nil && *stored.leaseExpirationTime >= now.UnixMilli() {
			// Alarm is already leased
			return nil, nil
		}

		// Place the actor only on one of the hosts connected to the requesting runtime
		_, err = s.lookupActorInTransaction(ctx, tx, alarmRef.ActorRef(), req.LeaseImmediate, time.UnixMilli(stored.dueTime))
		if errors.Is(err, components.ErrNoHost) {
			return nil, nil
		} else if err != nil {
			return nil, fmt.Errorf("failed to place alarm actor: %w", err)
		}

		// Acquire the lease before committing the alarm and actor placement
		storedLeaseIDValue := uuid.NewV7().String()
		// #nosec G202 -- the only concatenated value is the static table prefix, not user input
		err = tx.
			QueryRowContext(ctx,
				`UPDATE `+s.tablePrefix+`alarms
				SET alarm_lease_id = ?, alarm_lease_expiration_time = ?
				WHERE
					alarm_id = ?
					AND (
						alarm_lease_id IS NULL
						OR alarm_lease_expiration_time IS NULL
						OR alarm_lease_expiration_time < ?
					)
				RETURNING alarm_lease_id`,
				storedLeaseIDValue, now.Add(s.cfg.AlarmsLeaseDuration).UnixMilli(),
				stored.alarmID, now.UnixMilli(),
			).
			Scan(&storedLeaseIDValue)
		if errors.Is(err, sql.ErrNoRows) {
			return nil, nil
		} else if err != nil {
			return nil, fmt.Errorf("failed to lease alarm: %w", err)
		}

		return ref.NewAlarmLease(alarmRef, stored.alarmID, time.UnixMilli(stored.dueTime), storedLeaseIDValue), nil
	})
}

func (s *SQLiteProvider) DeleteAlarm(ctx context.Context, ref ref.AlarmRef) error {
	queryCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()
	// #nosec G202 -- the only concatenated value is the static table prefix, not user input
	res, err := s.db.ExecContext(queryCtx,
		`DELETE FROM `+s.tablePrefix+`alarms
		WHERE
			actor_type = ?
			AND actor_id = ?
			AND alarm_name = ?`,
		ref.ActorType, ref.ActorID, ref.Name,
	)
	if err != nil {
		return fmt.Errorf("error executing query: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("error counting affected rows: %w", err)
	}
	if affected == 0 {
		return components.ErrNoAlarm
	}

	return nil
}

func (s *SQLiteProvider) FetchAndLeaseUpcomingAlarms(ctx context.Context, req components.FetchAndLeaseUpcomingAlarmsReq) ([]*ref.AlarmLease, error) {
	// If there's no host, return an empty list
	if len(req.Hosts) == 0 {
		return nil, nil
	}

	return sqltransactions.ExecuteInTransaction(ctx, s.log, s.db, func(ctx context.Context, tx *sql.Tx) ([]*ref.AlarmLease, error) {
		fetcher := newUpcomingAlarmFetcher(tx, s, &req)

		res, err := fetcher.FetchUpcoming(ctx)
		if err != nil {
			return nil, fmt.Errorf("failed to fetch upcoming alarms: %w", err)
		}

		return res, nil
	})
}

func (s *SQLiteProvider) GetLeasedAlarm(ctx context.Context, lease *ref.AlarmLease) (res components.GetLeasedAlarmRes, err error) {
	queryCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	var (
		dueTime   int64
		interval  *string
		cron      *string
		jobMethod *string
		kind      string
		ttlTime   *int64
	)
	// #nosec G202 -- the only concatenated value is the static table prefix, not user input
	err = s.db.
		QueryRowContext(queryCtx, `
			SELECT
				actor_type, actor_id, alarm_name, alarm_data,
				alarm_due_time, alarm_interval, alarm_cron, alarm_ttl_time, alarm_kind, job_method
			FROM `+s.tablePrefix+`alarms
			WHERE
				alarm_id = ?
				AND alarm_lease_id = ?
				AND alarm_lease_expiration_time IS NOT NULL
				AND alarm_lease_expiration_time >= ?`,
			lease.Key(), lease.LeaseID(), s.clock.Now().UnixMilli(),
		).
		Scan(
			&res.ActorType, &res.ActorID, &res.Name, &res.Data,
			&dueTime, &interval, &cron, &ttlTime, &kind, &jobMethod,
		)

	if errors.Is(err, sql.ErrNoRows) {
		return res, components.ErrNoAlarm
	} else if err != nil {
		return res, fmt.Errorf("error executing query: %w", err)
	}

	res.DueTime = time.UnixMilli(dueTime)
	if interval != nil {
		res.Interval = *interval
	}
	if cron != nil {
		res.Cron = *cron
	}
	res.Kind = components.AlarmKind(kind)
	if jobMethod != nil {
		res.JobMethod = *jobMethod
	}
	if ttlTime != nil {
		res.TTL = new(time.UnixMilli(*ttlTime))
	}

	return res, nil
}

func (s *SQLiteProvider) RenewAlarmLeases(ctx context.Context, req components.RenewAlarmLeasesReq) (res components.RenewAlarmLeasesRes, err error) {
	// Renewal is always scoped to a set of hosts
	// With no host there's nothing to renew, so we e must return early here, as otherwise the query would contain an invalid "host_id IN ()" clause
	if len(req.Hosts) == 0 {
		return res, nil
	}

	now := s.clock.Now()
	expTime := now.Add(s.cfg.AlarmsLeaseDuration).UnixMilli()

	var leaseIdCondition string

	args := make([]any, 2+len(req.Leases)+len(req.Hosts))
	args[0] = expTime
	args[1] = now.UnixMilli()

	// If we have a list of leases, we restrict by them too
	if len(req.Leases) > 0 {
		// Add lease conditions
		b := strings.Builder{}
		b.Grow(len(req.Leases)*2 + len(" AND alarm_lease_id IN ()"))
		b.WriteString(" AND alarm_lease_id IN (")
		for i, lease := range req.Leases {
			if i == 0 {
				b.WriteRune('?')
			} else {
				b.WriteString(",?")
			}
			args[2+i] = lease.LeaseID()
		}
		b.WriteRune(')')

		leaseIdCondition = b.String()
	}

	// Add host conditions
	hostPlaceholders := getInPlaceholders(req.Hosts, args, 2+len(req.Leases))

	queryCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Disable the "G202: SQL string concatenation" gosec warning, since there's no risk of SQL injection here
	// #nosec G202
	rows, err := s.db.QueryContext(queryCtx,
		`
		UPDATE `+s.tablePrefix+`alarms
		SET alarm_lease_expiration_time = ?
		WHERE
			alarm_lease_expiration_time IS NOT NULL
			AND alarm_lease_expiration_time >= ?
			`+leaseIdCondition+`
			AND alarm_id IN (
				SELECT alarm_id FROM `+s.tablePrefix+`alarms a
				JOIN `+s.tablePrefix+`active_actors aa ON a.actor_type = aa.actor_type AND a.actor_id = aa.actor_id
				WHERE aa.host_id IN (`+hostPlaceholders+`)
			)
		RETURNING actor_type, actor_id, alarm_name, alarm_id, alarm_lease_id, alarm_due_time`,
		args...)
	if err != nil {
		return res, fmt.Errorf("query error: %w", err)
	}
	defer rows.Close()

	var renewedLeases []*ref.AlarmLease
	for rows.Next() {
		var (
			aRef             ref.AlarmRef
			alarmID, leaseID string
			dueTime          int64
		)
		err = rows.Scan(&aRef.ActorType, &aRef.ActorID, &aRef.Name, &alarmID, &leaseID, &dueTime)
		if err != nil {
			return res, fmt.Errorf("error scanning rows: %w", err)
		}

		renewedLeases = append(renewedLeases, ref.NewAlarmLease(
			aRef,
			alarmID,
			time.UnixMilli(dueTime),
			leaseID,
		))
	}

	err = rows.Err()
	if err != nil {
		return res, fmt.Errorf("error scanning rows: %w", err)
	}

	res.Leases = renewedLeases
	return res, nil
}

func (s *SQLiteProvider) ReleaseAlarmLease(ctx context.Context, lease *ref.AlarmLease) error {
	queryCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// #nosec G202 -- the only concatenated value is the static table prefix, not user input
	res, err := s.db.ExecContext(queryCtx, `
		UPDATE `+s.tablePrefix+`alarms
		SET
			alarm_lease_id = NULL,
			alarm_lease_expiration_time = NULL
		WHERE
			alarm_id = ?
			AND alarm_lease_id = ?
			AND alarm_lease_expiration_time IS NOT NULL
			AND alarm_lease_expiration_time >= ?`,
		lease.Key(), lease.LeaseID(), s.clock.Now().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("error executing query: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("error counting affected rows: %w", err)
	}
	if affected == 0 {
		return components.ErrNoAlarm
	}

	return nil
}

func (s *SQLiteProvider) UpdateLeasedAlarm(ctx context.Context, lease *ref.AlarmLease, req components.UpdateLeasedAlarmReq) (err error) {
	queryCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	whereClause := `WHERE
		alarm_id = ?
		AND alarm_lease_id = ?
		AND alarm_lease_expiration_time IS NOT NULL
		AND alarm_lease_expiration_time >= ?`

	now := s.clock.Now()

	// If we want to refresh the lease...
	var res sql.Result
	if req.RefreshLease {
		// Disable the "G202: SQL string concatenation" gosec warning, since there's no risk of SQL injection here
		// #nosec G202
		res, err = s.db.ExecContext(queryCtx, `
			UPDATE `+s.tablePrefix+`alarms
			SET
				alarm_lease_expiration_time = ?,
				alarm_due_time = ?
			`+whereClause,
			now.Add(s.cfg.AlarmsLeaseDuration).UnixMilli(), req.DueTime.UnixMilli(),
			lease.Key(), lease.LeaseID(), now.UnixMilli(),
		)
	} else {
		// Disable the "G202: SQL string concatenation" gosec warning, since there's no risk of SQL injection here
		// #nosec G202
		res, err = s.db.ExecContext(queryCtx, `
			UPDATE `+s.tablePrefix+`alarms
			SET
				alarm_lease_id = NULL,
				alarm_lease_expiration_time = NULL,
				alarm_due_time = ?
			`+whereClause,
			req.DueTime.UnixMilli(),
			lease.Key(), lease.LeaseID(), now.UnixMilli(),
		)
	}
	if err != nil {
		return fmt.Errorf("error executing query: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("error counting affected rows: %w", err)
	}
	if affected == 0 {
		return components.ErrNoAlarm
	}

	return nil
}

func (s *SQLiteProvider) DeleteLeasedAlarm(ctx context.Context, lease *ref.AlarmLease) error {
	queryCtx, cancel := context.WithTimeout(ctx, s.timeout)
	defer cancel()

	// Note on the query:
	// A job handler that halts its own actor is the common case for a worker, and deactivating an actor drops the leases of its alarms so another host can pick them up
	// For the occurrence being finalized right now that release must not undo the finalization, so a lease this execution owns and a lease that was released both count
	// A lease that merely expired keeps its id, and one another replica took holds its own id, so neither is matched here
	// An unleased row is only this execution's occurrence while it still carries the due time this execution leased, since a repeating alarm's next occurrence is always scheduled later
	// #nosec G202 -- the only concatenated value is the static table prefix, not user input
	res, err := s.db.ExecContext(queryCtx, `
		DELETE FROM `+s.tablePrefix+`alarms
		WHERE
			alarm_id = ?
			AND (
				(
					alarm_lease_id = ?
					AND alarm_lease_expiration_time IS NOT NULL
					AND alarm_lease_expiration_time >= ?
				)
				OR (alarm_lease_id IS NULL AND alarm_due_time = ?)
			)`,
		lease.Key(), lease.LeaseID(), s.clock.Now().UnixMilli(), lease.DueTime().UnixMilli(),
	)
	if err != nil {
		return fmt.Errorf("error executing query: %w", err)
	}

	affected, err := res.RowsAffected()
	if err != nil {
		return fmt.Errorf("error counting affected rows: %w", err)
	}
	if affected == 0 {
		return components.ErrNoAlarm
	}

	return nil
}
