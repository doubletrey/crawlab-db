package mongo

import (
	"github.com/spf13/viper"
	"go.mongodb.org/mongo-driver/mongo"
)

func GetMongoDb(dbName string) (db *mongo.Database) {
	if dbName == "" {
		dbName = viper.GetString("mongo.db")
	}
	return Client.Database(dbName, nil)
}
