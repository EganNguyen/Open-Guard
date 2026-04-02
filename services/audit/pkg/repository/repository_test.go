package repository_test

import (
	"context"
	"testing"

	"github.com/openguard/audit/pkg/repository"
	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

func TestNewRepository(t *testing.T) {
	client, _ := mongo.Connect(context.TODO(), options.Client().ApplyURI("mongodb://localhost:27017"))
	
	t.Run("Unified Repository", func(t *testing.T) {
		repo := repository.New(client)
		assert.NotNil(t, repo)
		assert.NotNil(t, repo.GetCollection())
	})
}
