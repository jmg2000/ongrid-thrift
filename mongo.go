package main

import (
	"errors"
	"fmt"
	"log"
	"time"

	"gopkg.in/mgo.v2"
	"gopkg.in/mgo.v2/bson"
)

// MongoConfig contains configuration for MongoDB
type MongoConfig struct {
	user     string
	passowrd string
	host     string
	dbName   string
}

type docClient struct {
	ID         bson.ObjectId   `bson:"_id"`
	UserName   string          `bson:"username"`
	Password   string          `bson:"password"`
	FirstName  string          `bson:"firstName"`
	LastName   string          `bson:"lastName"`
	Email      string          `bson:"email"`
	Database   docDatabase     `bson:"database"`
	WorkPlaces []docWorkPlaces `bson:"workPlaces"`
}

type docWorkPlaces struct {
	ID      bson.ObjectId `bson:"_id"`
	WPName  string        `bson:"wpName"`
	MacAddr string        `bson:"macAddr"`
}

type docDatabase struct {
	Host       string `bson:"host"`
	Port       int    `bson:"port"`
	Path       string `bson:"path"`
	User       string `bson:"user"`
	Password   string `bson:"password"`
	DataFile   string `bson:"dataFile"`
	ConfigFile string `bson:"configFile"`
}

type docSession struct {
	id        bson.ObjectId `bson:"_id"`
	login     string        `bson:"login"`
	token     string        `bson:"token"`
	createdAt time.Time     `bson:"created"`
	active    bool
}

// MongoConnection ...
type MongoConnection struct {
	originalSession *mgo.Session
}

// NewMongoConnection ...
func NewMongoConnection(mgoConfig MongoConfig) (conn *MongoConnection) {
	conn = new(MongoConnection)
	conn.createLocalConnection(mgoConfig)
	return
}

func (c *MongoConnection) createLocalConnection(mgoConfig MongoConfig) (err error) {
	log.Println("Connecting to mongo server...")
	c.originalSession, err = mgo.Dial(mgoConfig.host)
	if err == nil {
		log.Println("Connection established to mongo server")
		err = c.originalSession.DB(mgoConfig.dbName).Login(mgoConfig.user, mgoConfig.passowrd)
		if err != nil {
			log.Printf("Error login to mongodb: %s\n", err.Error())
		}
		log.Println("Mongo login passed")
		sessionCollection := c.originalSession.DB("ongrid").C("thrift-sessions")
		if sessionCollection == nil {
			err = errors.New("Collection could not be created, maybe need to create it manually")
		}
	} else {
		log.Printf("Error occured while creating mongodb connection: %s", err.Error())
	}
	return
}

// CloseConnection ...
func (c *MongoConnection) CloseConnection() {
	if c.originalSession != nil {
		c.originalSession.Close()
	}
}

func (c *MongoConnection) getSessionAndCollection() (session *mgo.Session, clientCollection *mgo.Collection, err error) {
	if c.originalSession != nil {
		session = c.originalSession.Copy()
		clientCollection = session.DB("ongrid").C("clients")
	} else {
		err = errors.New("No original session found")
	}
	return
}

// GetUserByMacAddr ...
func (c *MongoConnection) GetUserByMacAddr(macAddr string) (user User, err error) {
	//create an empty document struct
	result := docClient{}
	//get a copy of the original session and a collection
	session, clientCollection, err := c.getSessionAndCollection()
	if err != nil {
		return
	}
	defer session.Close()
	fmt.Printf("MacAddr: %s\n", macAddr)
	err = clientCollection.Find(bson.M{"workPlaces.macAddr": macAddr}).One(&result)
	if err != nil {
		log.Println(err)
		return
	}

	user = fillUserFromResult(result)

	return
}

// GetUserByLogin ...
func (c *MongoConnection) GetUserByLogin(login string) (user User, err error) {
	//create an empty document struct
	result := docClient{}
	//get a copy of the original session and a collection
	session, clientCollection, err := c.getSessionAndCollection()
	if err != nil {
		return
	}
	defer session.Close()

	err = clientCollection.Find(bson.M{"username": login}).One(&result)
	if err != nil {
		log.Println(err)
		return
	}

	user = fillUserFromResult(result)

	return
}

func fillUserFromResult(result docClient) User {
	var user User

	user.ID = result.ID.Hex()
	user.Login = result.UserName
	user.Password = result.Password
	user.FirstName = result.FirstName
	user.LastName = result.LastName
	user.Email = result.Email
	user.DB.host = result.Database.Host
	user.DB.port = result.Database.Port
	user.DB.path = result.Database.Path
	user.DB.user = result.Database.User
	user.DB.password = result.Database.Password
	user.DB.dataDB = result.Database.DataFile
	user.DB.configDB = result.Database.ConfigFile

	return user
}

// ClientAddWorkPlace ...
func (c *MongoConnection) ClientAddWorkPlace(id string, wpName string, macAddr string) error {
	session, clientCollection, err := c.getSessionAndCollection()
	if err != nil {
		return err
	}
	defer session.Close()

	query := bson.M{"_id": bson.ObjectIdHex(id)}
	pushWorkPlace := bson.M{"$push": bson.M{"workPlaces": bson.M{"_id": bson.NewObjectId(), "wpName": wpName, "macAddr": macAddr}}}
	err = clientCollection.Update(query, pushWorkPlace)
	if err != nil {
		log.Println(err)
		return err
	}

	return nil
}
