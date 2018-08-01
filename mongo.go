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

// Ongrid client that works with the desktop application
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
	ID        bson.ObjectId `bson:"_id"`
	Login     string        `bson:"login"`
	Token     string        `bson:"token"`
	CreatedAt time.Time     `bson:"created"`
	Active    bool
}

type docCustomer struct {
	ID       bson.ObjectId `bson:"_id"`
	Owner    bson.ObjectId `bson:"owner"`
	Name     string        `bson:"username"`
	Email    string        `bson:"email"`
	Passowrd string        `bson:"password"`
	Phone    string        `bson:"phone"`
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
		// sessionCollection := c.originalSession.DB("ongrid").C("thrift-sessions")
		// if sessionCollection == nil {
		// 	err = errors.New("Collection could not be created, maybe need to create it manually")
		// }
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

func (c *MongoConnection) getSessionAndCollection(collectionName string) (session *mgo.Session, clientCollection *mgo.Collection, err error) {
	if c.originalSession != nil {
		session = c.originalSession.Copy()
		clientCollection = session.DB("ongrid").C(collectionName)
	} else {
		err = errors.New("No original session found")
	}
	return
}

// GetUserByMacAddr ...
func (c *MongoConnection) GetUserByMacAddr(login string, macAddr string) (user User, err error) {
	//create an empty document struct
	result := docClient{}
	//get a copy of the original session and a collection
	session, clientCollection, err := c.getSessionAndCollection("clients")
	if err != nil {
		return
	}
	defer session.Close()
	fmt.Printf("MacAddr: %s\n", macAddr)
	err = clientCollection.Find(bson.M{"email": login, "workPlaces.macAddr": macAddr}).One(&result)
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
	session, clientCollection, err := c.getSessionAndCollection("clients")
	if err != nil {
		return
	}
	defer session.Close()

	err = clientCollection.Find(bson.M{"email": login}).One(&result)
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
	session, clientCollection, err := c.getSessionAndCollection("clients")
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

// CreateCustomer ...
func (c *MongoConnection) CreateCustomer(owner string, name string, email string, phone string, password string) (string, error) {
	session, customerCollection, err := c.getSessionAndCollection("clients")
	if err != nil {
		return "", err
	}
	defer session.Close()

	index := mgo.Index{
		Key:      []string{"$text:email"},
		Unique:   true,
		DropDups: true,
	}

	customerCollection.EnsureIndex(index)

	customerID := bson.NewObjectId()

	err = customerCollection.Insert(
		&docCustomer{
			ID:       customerID,
			Owner:    bson.ObjectIdHex(owner),
			Name:     name,
			Email:    email,
			Phone:    phone,
			Passowrd: password,
		},
	)
	if err != nil {
		if mgo.IsDup(err) {
			err = errors.New("Duplicate email exists")
		}
		return "", err
	}

	return customerID.Hex(), nil
}
