package dialer

import (
	"context"
	"fmt"
	"log"
	"strings"
	"time"

	"github.com/exelban/EndPoll/types"
	"go.mongodb.org/mongo-driver/bson"
	"go.mongodb.org/mongo-driver/mongo"
	"go.mongodb.org/mongo-driver/mongo/options"
)

// mongoCall makes a mongo request to the host
func (d *Dialer) mongoCall(ctx context.Context, h *types.Host) (response types.HttpResponse) {
	ctx_, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	client, err := mongo.Connect(ctx_, options.Client().ApplyURI(h.URL))
	if err != nil {
		log.Printf("[ERROR] connect mongo %v", err)
		response.Body = err.Error()
		response.Code = 501
		return
	}
	defer func() {
		if err = client.Disconnect(ctx_); err != nil {
			log.Printf("[ERROR] disconnect mongo %v", err)
		}
	}()

	response.Timestamp = time.Now()
	response.OK = true

	if err := client.Ping(ctx_, nil); err != nil {
		log.Printf("[ERROR] ping mongo %v", err)
		response.Body = err.Error()
		response.Code = 501
		return
	}

	type MongoMetaData struct {
		Set     string `bson:"set"`
		RSState int64  `bson:"myState"`
	}
	mongoMetaData := MongoMetaData{}
	db := client.Database("admin")

	err = db.RunCommand(ctx_, bson.D{{"replSetGetStatus", 1}}).Decode(&mongoMetaData)
	if err != nil {
		if strings.Contains(err.Error(), "NoReplicationEnabled") && strings.Contains(h.URL, "replicaSet") {
			response.Code = 502
			response.Body = err.Error()
			return
		} else if !strings.Contains(err.Error(), "NoReplicationEnabled") {
			response.Body = err.Error()
			response.Code = 503
			return
		}
	}

	if mongoMetaData.Set != "" && mongoMetaData.RSState != 1 {
		response.Code = 500
		response.Body = fmt.Sprintf("mongo rs is not in correct state: %s", mongoMetaData.Set)
		return
	}

	response.Code = 200

	return
}
