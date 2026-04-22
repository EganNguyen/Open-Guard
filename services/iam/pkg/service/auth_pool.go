package service

import (
	"context"

	"golang.org/x/crypto/bcrypt"
)

type bcryptJob struct {
	password string
	hash     string
	result   chan error
}

// AuthWorkerPool manages a bounded set of goroutines for CPU-intensive bcrypt operations.
type AuthWorkerPool struct {
	jobs    chan bcryptJob
	workers int
}

// NewAuthWorkerPool initializes the pool with the specified number of workers.
func NewAuthWorkerPool(workers int) *AuthWorkerPool {
	p := &AuthWorkerPool{
		jobs:    make(chan bcryptJob, 100),
		workers: workers,
	}
	for i := 0; i < workers; i++ {
		go p.worker()
	}
	return p
}

func (p *AuthWorkerPool) worker() {
	for job := range p.jobs {
		job.result <- bcrypt.CompareHashAndPassword([]byte(job.hash), []byte(job.password))
	}
}

// Compare schedules a bcrypt comparison and waits for the result.
func (p *AuthWorkerPool) Compare(ctx context.Context, password, hash string) error {
	result := make(chan error, 1)
	select {
	case p.jobs <- bcryptJob{password, hash, result}:
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
