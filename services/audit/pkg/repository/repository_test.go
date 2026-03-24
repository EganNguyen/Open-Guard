package repository_test

import (
	"context"
	"testing"

	"github.com/openguard/audit/pkg/repository"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestNewRepositories(t *testing.T) {
	client, _ := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	
	t.Run("WriteRepository", func(t *testing.T) {
		repo := repository.NewWriteRepository(client)
		assert.NotNil(t, repo)
		assert.NotNil(t, repo.GetCollection())
	})
	
	t.Run("ReadRepository", func(t *testing.T) {
		repo := repository.NewReadRepository(client)
		assert.NotNil(t, repo)
	})
}
