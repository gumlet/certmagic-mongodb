# Certmagic Storage Backend for MongoDB

[![Go Reference](https://pkg.go.dev/badge/github.com/gumlet/certmagic-mongodb.svg)](https://pkg.go.dev/github.com/gumlet/certmagic-mongodb)

This library allows you to use MongoDB as key/certificate storage backend for your Certmagic-enabled HTTPS server.

Package documentation: https://pkg.go.dev/github.com/gumlet/certmagic-mongodb

## How to use

Install this package

```sh
go get github.com/gumlet/certmagic-mongodb

```

Use it as your [certmagic](https://github.com/caddyserver/certmagic) default storage backend.

```go
import (
    mongoStore "github.com/gumlet/certmagic-mongodb"
    // other dependencies....
)

database := "companydb"

// Connect to MongoDB
clientOptions := options.Client().ApplyURI(config.MongoURI)
mongoClient, err := mongo.Connect(context.TODO(), clientOptions)

// pass MongoDB client and database name to storage interface.
storage, err := mongoStore.NewMongoStorage(mongoClient, database)
if err != nil {
    log.Fatal(err)
}
// tell certmagic to use this storage.
certmagic.Default.Storage = storage

```

This library will create 2 new collections `certmagic-storage` and `certmagic-locks` in your database and store all the required data in those collections. It will also create all required indexes.

