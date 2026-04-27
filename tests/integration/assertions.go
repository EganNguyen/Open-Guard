package integration

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"go.mongodb.org/mongo-driver/bson"
)

func AssertPostgresRowExists(t *testing.T, query string, args ...interface{}) {
	var exists bool
	err := testDB.QueryRow(context.Background(), "SELECT EXISTS("+query+")", args...).Scan(&exists)
	assert.NoError(t, err)
	assert.True(t, exists, "Expected row to exist in PostgreSQL")
}

func AssertMongoEventCaptured(t *testing.T, orgID, subject string) {
	ctx := context.Background()
	Eventually(t, func() bool {
		collection := testMongo.Database("openguard_audit").Collection("events")
		count, err := collection.CountDocuments(ctx, bson.M{"org_id": orgID, "subject": subject})
		return err == nil && count > 0
	}, 15*time.Second, 1*time.Second)
}

func AssertClickHouseLogIndexed(t *testing.T, orgID, action string) {
	ctx := context.Background()
	Eventually(t, func() bool {
		var count uint64
		err := testCH.QueryRow(ctx, "SELECT count() FROM access_logs WHERE org_id = ? AND action = ?", orgID, action).Scan(&count)
		return err == nil && count > 0
	}, 15*time.Second, 1*time.Second)
}
