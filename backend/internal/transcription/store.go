package transcription

import (
	"context"
	"errors"
	"fmt"
	"time"

	"github.com/example/web-utils/backend/internal/diarization"
	"github.com/google/uuid"
	"github.com/jackc/pgx/v5"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Job struct {
	ID, Status, ErrorCode, ErrorMessage string
	SourceURL                           string
	Attempts                            int
}

type Segment struct {
	ID             string `json:"id,omitempty"`
	StartMS        int    `json:"start_ms"`
	EndMS          int    `json:"end_ms"`
	Text           string `json:"text"`
	Speaker        string `json:"speaker"`
}

type FormattedVersion struct {
	ID        string    `json:"id"`
	JobID     string    `json:"job_id,omitempty"`
	Preset    string    `json:"preset"`
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type Store struct {
	DB     *pgxpool.Pool
	Events *EventBroker
}

func (s Store) publish(jobID, status string) {
	if s.Events != nil {
		s.Events.Publish(jobID, status)
	}
}

func (s Store) Create(ctx context.Context, sourceURL, key string) (Job, error) {
	if key == "" {
		key = uuid.NewString()
	}
	job := Job{ID: uuid.NewString(), Status: "queued", SourceURL: sourceURL}
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	runID := uuid.NewString()
	_, err = tx.Exec(ctx, `INSERT INTO utility_runs (id,idempotency_key,source_url) VALUES ($1,$2,$3)`, runID, key, sourceURL)
	if err != nil {
		_ = tx.Rollback(ctx)
		var existing Job
		qErr := s.DB.QueryRow(ctx, `SELECT j.id,j.status,COALESCE(j.error_code,''),COALESCE(j.error_message,''),r.source_url,j.attempts FROM utility_runs r JOIN transcription_jobs j ON j.run_id=r.id WHERE r.idempotency_key=$1`, key).
			Scan(&existing.ID, &existing.Status, &existing.ErrorCode, &existing.ErrorMessage, &existing.SourceURL, &existing.Attempts)
		if qErr == nil {
			return existing, nil
		}
		return Job{}, err
	}
	_, err = tx.Exec(ctx, `INSERT INTO transcription_jobs (id,run_id,status) VALUES ($1,$2,'queued')`, job.ID, runID)
	if err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	s.publish(job.ID, "queued")
	return job, nil
}

func (s Store) Get(ctx context.Context, id string) (Job, []Segment, error) {
	var j Job
	err := s.DB.QueryRow(ctx, `SELECT j.id,j.status,COALESCE(j.error_code,''),COALESCE(j.error_message,''),r.source_url,j.attempts FROM transcription_jobs j JOIN utility_runs r ON r.id=j.run_id WHERE j.id=$1`, id).
		Scan(&j.ID, &j.Status, &j.ErrorCode, &j.ErrorMessage, &j.SourceURL, &j.Attempts)
	if err != nil {
		return Job{}, nil, err
	}
	rows, err := s.DB.Query(ctx, `SELECT id::text,start_ms,end_ms,text,COALESCE(speaker,'Unknown') FROM transcript_segments WHERE job_id=$1 ORDER BY start_ms, id`, id)
	if err != nil {
		return Job{}, nil, err
	}
	defer rows.Close()
	segments := make([]Segment, 0)
	for rows.Next() {
		var v Segment
		if err := rows.Scan(&v.ID, &v.StartMS, &v.EndMS, &v.Text, &v.Speaker); err != nil {
			return Job{}, nil, err
		}
		segments = append(segments, v)
	}
	return j, segments, rows.Err()
}

func (s Store) ListFormatted(ctx context.Context, jobID string) ([]FormattedVersion, error) {
	rows, err := s.DB.Query(ctx, `SELECT id::text,preset,content,created_at FROM formatted_transcripts WHERE job_id=$1 ORDER BY created_at ASC`, jobID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	out := make([]FormattedVersion, 0)
	for rows.Next() {
		var f FormattedVersion
		f.JobID = jobID
		if err := rows.Scan(&f.ID, &f.Preset, &f.Content, &f.CreatedAt); err != nil {
			return nil, err
		}
		out = append(out, f)
	}
	return out, rows.Err()
}

func (s Store) SaveFormatted(ctx context.Context, jobID, preset, content string) (FormattedVersion, error) {
	id := uuid.NewString()
	var createdAt time.Time
	err := s.DB.QueryRow(ctx, `INSERT INTO formatted_transcripts(id,job_id,preset,content) VALUES($1,$2,$3,$4) RETURNING created_at`, id, jobID, preset, content).Scan(&createdAt)
	if err != nil {
		return FormattedVersion{}, err
	}
	return FormattedVersion{
		ID:        id,
		JobID:     jobID,
		Preset:    preset,
		Content:   content,
		CreatedAt: createdAt,
	}, nil
}

func (s Store) Claim(ctx context.Context) (Job, error) {
	tx, err := s.DB.BeginTx(ctx, pgx.TxOptions{})
	if err != nil {
		return Job{}, err
	}
	defer tx.Rollback(ctx)
	var j Job
	err = tx.QueryRow(ctx, `WITH candidate AS (SELECT j.id FROM transcription_jobs j WHERE j.status='queued' AND j.next_attempt_at<=now() AND (j.lease_expires_at IS NULL OR j.lease_expires_at<now()) ORDER BY j.created_at FOR UPDATE SKIP LOCKED LIMIT 1) UPDATE transcription_jobs j SET status='resolving',attempts=attempts+1,lease_expires_at=now()+interval '30 minutes',updated_at=now() FROM candidate c,utility_runs r WHERE j.id=c.id AND r.id=j.run_id RETURNING j.id,j.status,'','',r.source_url,j.attempts`).
		Scan(&j.ID, &j.Status, &j.ErrorCode, &j.ErrorMessage, &j.SourceURL, &j.Attempts)
	if errors.Is(err, pgx.ErrNoRows) {
		return Job{}, tx.Commit(ctx)
	}
	if err != nil {
		return Job{}, err
	}
	if err := tx.Commit(ctx); err != nil {
		return Job{}, err
	}
	s.publish(j.ID, "resolving")
	return j, nil
}

func (s Store) Complete(ctx context.Context, id string, segments []Segment) error {
	return s.CompleteWithTurns(ctx, id, segments, nil)
}

func (s Store) CompleteWithTurns(ctx context.Context, id string, segments []Segment, turns []diarization.Turn) error {
	tx, err := s.DB.Begin(ctx)
	if err != nil {
		return err
	}
	defer tx.Rollback(ctx)
	var currentStatus string
	if err := tx.QueryRow(ctx, `SELECT status FROM transcription_jobs WHERE id=$1 FOR UPDATE`, id).Scan(&currentStatus); err != nil {
		return err
	}
	if currentStatus == "cancelled" {
		return tx.Commit(ctx)
	}
	_, _ = tx.Exec(ctx, `DELETE FROM transcript_segments WHERE job_id=$1`, id)
	_, _ = tx.Exec(ctx, `DELETE FROM speaker_turns WHERE job_id=$1`, id)
	for _, t := range turns {
		if _, err = tx.Exec(ctx, `INSERT INTO speaker_turns(job_id,start_ms,end_ms,speaker) VALUES($1,$2,$3,$4)`, id, t.StartMS, t.EndMS, t.Speaker); err != nil {
			return err
		}
	}
	for _, v := range segments {
		if _, err = tx.Exec(ctx, `INSERT INTO transcript_segments(job_id,start_ms,end_ms,text,speaker) VALUES($1,$2,$3,$4,$5)`, id, v.StartMS, v.EndMS, v.Text, v.Speaker); err != nil {
			return err
		}
	}
	_, err = tx.Exec(ctx, `UPDATE transcription_jobs SET status='completed',error_code=NULL,error_message=NULL,lease_expires_at=NULL,updated_at=now() WHERE id=$1`, id)
	if err != nil {
		return err
	}
	if err := tx.Commit(ctx); err != nil {
		return err
	}
	s.publish(id, "completed")
	return nil
}

func (s Store) Fail(ctx context.Context, id, code string) error {
	msg := SafeErrorMessage(code)
	_, err := s.DB.Exec(ctx, `UPDATE transcription_jobs SET status='failed',error_code=$2,error_message=$3,lease_expires_at=NULL,updated_at=now() WHERE id=$1 AND status!='cancelled'`, id, code, msg)
	if err == nil {
		s.publish(id, "failed")
	}
	return err
}

func (s Store) Cancel(ctx context.Context, id string) error {
	tag, err := s.DB.Exec(ctx, `UPDATE transcription_jobs SET status='cancelled',lease_expires_at=NULL,updated_at=now() WHERE id=$1 AND status NOT IN ('completed','cancelled')`, id)
	if err != nil {
		return err
	}
	if tag.RowsAffected() > 0 {
		s.publish(id, "cancelled")
	}
	return nil
}

func (s Store) Retry(ctx context.Context, id string) (Job, error) {
	var j Job
	err := s.DB.QueryRow(ctx, `UPDATE transcription_jobs j SET status='queued',error_code=NULL,error_message=NULL,lease_expires_at=NULL,next_attempt_at=now(),updated_at=now() FROM utility_runs r WHERE j.id=$1 AND r.id=j.run_id AND j.status IN ('failed','cancelled') RETURNING j.id,j.status,'','',r.source_url,j.attempts`, id).
		Scan(&j.ID, &j.Status, &j.ErrorCode, &j.ErrorMessage, &j.SourceURL, &j.Attempts)
	if err != nil {
		return Job{}, fmt.Errorf("job cannot be retried: %w", err)
	}
	s.publish(id, "queued")
	return j, nil
}

func (s Store) Touch(ctx context.Context, id, status string) error {
	tag, err := s.DB.Exec(ctx, `UPDATE transcription_jobs SET status=$2,lease_expires_at=now()+interval '30 minutes',updated_at=now() WHERE id=$1 AND status NOT IN ('cancelled','completed')`, id, status)
	if err != nil {
		return err
	}
	if tag.RowsAffected() == 0 {
		return fmt.Errorf("job %s is no longer active", id)
	}
	s.publish(id, status)
	return nil
}

func (s Store) CleanupExpired(ctx context.Context, retention time.Duration) (int64, error) {
	if retention <= 0 {
		retention = 7 * 24 * time.Hour
	}
	cutoff := time.Now().UTC().Add(-retention)
	tag, err := s.DB.Exec(ctx, `DELETE FROM utility_runs WHERE id IN (SELECT r.id FROM utility_runs r JOIN transcription_jobs j ON j.run_id=r.id WHERE j.status IN ('completed','failed','cancelled') AND j.updated_at < $1 LIMIT 100)`, cutoff)
	if err != nil {
		return 0, err
	}
	return tag.RowsAffected(), nil
}

func (s Store) LeaseDuration() time.Duration { return 30 * time.Minute }
