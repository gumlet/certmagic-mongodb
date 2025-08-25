package mongoStore

import (
	"context"
	"fmt"
	"os"
	"time"

	"github.com/caddyserver/certmagic"
	"go.mongodb.org/mongo-driver/v2/bson"
	"go.mongodb.org/mongo-driver/v2/mongo"
	"go.mongodb.org/mongo-driver/v2/mongo/options"
)

// MongoStorage implements certmagic.Storage using MongoDB.
type MongoStorage struct {
	Collection *mongo.Collection
	Locks      *mongo.Collection
	LockTTL    time.Duration
	InstanceID string // Unique identifier for this instance
}

// NewMongoStorage initializes the MongoStorage
func NewMongoStorage(mongoClient *mongo.Client, database string) (*MongoStorage, error) {
	collection := mongoClient.Database(database).Collection("certmagic-storage")
	locks := mongoClient.Database(database).Collection("certmagic-locks")

	_, err := locks.Indexes().CreateMany(context.TODO(), []mongo.IndexModel{
		{
			Keys:    bson.M{"key": 1},
			Options: options.Index().SetUnique(true),
		},
		{
			Keys: bson.M{"expires_at": 1},
			Options: options.Index().
				SetExpireAfterSeconds(0),
		},
	})

	if err != nil {
		return nil, err
	}

	hostname, _ := os.Hostname()

	return &MongoStorage{
		Collection: collection,
		Locks:      locks,
		LockTTL:    60 * time.Second,
		InstanceID: hostname,
	}, nil
}

// Store stores data in MongoDB
func (m *MongoStorage) Store(ctx context.Context, key string, value []byte) error {
	_, err := m.Collection.UpdateOne(ctx,
		bson.M{"key": key},
		bson.M{"$set": bson.M{
			"key":   key,
			"value": value,
			"ts":    time.Now(),
		}},
		options.UpdateOne().SetUpsert(true),
	)
	return err
}

// Load retrieves data by key
func (m *MongoStorage) Load(ctx context.Context, key string) ([]byte, error) {
	var result struct {
		Value []byte `bson:"value"`
	}
	err := m.Collection.FindOne(ctx, bson.M{"key": key}).Decode(&result)
	if err == mongo.ErrNoDocuments {
		return nil, fmt.Errorf("MongoStorage.Load key %s not found: %w", key, os.ErrNotExist)
	}
	return result.Value, err
}

// Delete deletes a key
func (m *MongoStorage) Delete(ctx context.Context, key string) error {
	_, err := m.Collection.DeleteOne(ctx, bson.M{"key": key})
	if err != nil {
		return err
	}
	return nil
}

// Exists checks if a key exists
func (m *MongoStorage) Exists(ctx context.Context, key string) bool {
	count, _ := m.Collection.CountDocuments(ctx, bson.M{"key": key})
	return count > 0
}

// List lists keys prefixed by a string
func (m *MongoStorage) List(ctx context.Context, prefix string, recursive bool) ([]string, error) {
	filter := bson.M{"key": bson.M{"$regex": "^" + prefix}}
	cursor, err := m.Collection.Find(ctx, filter)
	if err != nil {
		return nil, err
	}
	defer cursor.Close(ctx)

	var keys []string
	for cursor.Next(ctx) {
		var doc struct{ Key string }
		if err := cursor.Decode(&doc); err == nil {
			keys = append(keys, doc.Key)
		}
	}
	return keys, cursor.Err()
}

// Stat returns file info (size and modified time)
func (m *MongoStorage) Stat(ctx context.Context, key string) (certmagic.KeyInfo, error) {
	var result struct {
		Value []byte    `bson:"value"`
		TS    time.Time `bson:"ts"`
	}
	err := m.Collection.FindOne(ctx, bson.M{"key": key}).Decode(&result)
	if err == mongo.ErrNoDocuments {
		return certmagic.KeyInfo{}, fmt.Errorf("MongoStorage.Stat key %s not found: %w", key, os.ErrNotExist)
	}
	return certmagic.KeyInfo{
		Key:        key,
		Modified:   result.TS,
		Size:       int64(len(result.Value)),
		IsTerminal: true,
	}, err
}

func (m *MongoStorage) Lock(ctx context.Context, key string) error {
	expiration := time.Now().Add(m.LockTTL)

	lock := bson.M{
		"key":        key,
		"instance":   m.InstanceID,
		"expires_at": expiration,
	}

	for {
		select {
		case <-ctx.Done():
			return fmt.Errorf("timeout acquiring lock for key %s: %w", key, ctx.Err())

		default:
			// Try to insert the lock
			_, err := m.Locks.InsertOne(ctx, lock)
			if err == nil {
				// Lock acquired
				return nil
			}

			// If the error is duplicate key, someone else holds the lock
			if mongo.IsDuplicateKeyError(err) {
				time.Sleep(2 * time.Second)
				continue
			}

			// Other errors
			return fmt.Errorf("error trying to acquire lock for key %s: %w", key, err)
		}
	}
}

func (m *MongoStorage) Unlock(ctx context.Context, key string) error {
	_, err := m.Locks.DeleteOne(ctx, bson.M{
		"key":      key,
		"instance": m.InstanceID,
	})
	if err != nil {
		return fmt.Errorf("failed to release lock for key %s: %w", key, err)
	}
	return nil
}
