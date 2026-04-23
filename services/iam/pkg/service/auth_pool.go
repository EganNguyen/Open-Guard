package service

import (
	"context"

	"golang.org/x/crypto/bcrypt"
)

type bcryptCompareJob struct {
	password string
	hash     string
	result   chan error
}

type bcryptGenerateJob struct {
	password string
	result   chan struct {
		hash []byte
		err  error
	}
}

// AuthWorkerPool manages a bounded set of goroutines for CPU-intensive bcrypt operations.
type AuthWorkerPool struct {
	compareJobs  chan bcryptCompareJob
	generateJobs chan bcryptGenerateJob
	workers      int
}

// NewAuthWorkerPool initializes the pool with the specified number of workers.
func NewAuthWorkerPool(workers int) *AuthWorkerPool {
	p := &AuthWorkerPool{
		compareJobs:  make(chan bcryptCompareJob, 100),
		generateJobs: make(chan bcryptGenerateJob, 100),
		workers:      workers,
	}
	for i := 0; i < workers; i++ {
		go p.worker()
	}
	return p
}

func (p *AuthWorkerPool) worker() {
	for {
		select {
		case job := <-p.compareJobs:
			job.result <- bcrypt.CompareHashAndPassword([]byte(job.hash), []byte(job.password))
		case job := <-p.generateJobs:
			hash, err := bcrypt.GenerateFromPassword([]byte(job.password), 12)
			job.result <- struct {
				hash []byte
				err  error
			}{hash, err}
		}
	}
}

// Compare schedules a bcrypt comparison and waits for the result.
func (p *AuthWorkerPool) Compare(ctx context.Context, password, hash string) error {
	result := make(chan error, 1)
	select {
	case p.compareJobs <- bcryptCompareJob{password, hash, result}:
	case <-ctx.Done():
		return ctx.Err()
	}

	select {
	case err := <-result:
		return err
	case <-ctx.Done():
		return ctx.Err()
	}
}

// Generate schedules a bcrypt hash generation and waits for the result.
func (p *AuthWorkerPool) Generate(ctx context.Context, password string) ([]byte, error) {
	result := make(chan struct {
		hash []byte
		err  error
	}, 1)

	select {
	case p.generateJobs <- bcryptGenerateJob{password, result}:
	case <-ctx.Done():
		return nil, ctx.Err()
	}

	select {
	case res := <-result:
		return res.hash, res.err
	case <-ctx.Done():
		return nil, ctx.Err()
	}
}
