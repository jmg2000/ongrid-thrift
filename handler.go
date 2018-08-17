package main

/*
 * Licensed to the Apache Software Foundation (ASF) under one
 * or more contributor license agreements. See the NOTICE file
 * distributed with this work for additional information
 * regarding copyright ownership. The ASF licenses this file
 * to you under the Apache License, Version 2.0 (the
 * "License"); you may not use this file except in compliance
 * with the License. You may obtain a copy of the License at
 *
 *   http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing,
 * software distributed under the License is distributed on an
 * "AS IS" BASIS, WITHOUT WARRANTIES OR CONDITIONS OF ANY
 * KIND, either express or implied. See the License for the
 * specific language governing permissions and limitations
 * under the License.
 */

import (
	"crypto/md5"
	"database/sql"
	"errors"
	"fmt"
	"io"
	"log"
	"ongrid-thrift/ongrid2"
	"ongrid-thrift/privileges"
	"strconv"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kylelemons/go-gypsy/yaml"
	_ "github.com/nakagami/firebirdsql"
	"github.com/sethvargo/go-password/password"
	"github.com/twinj/uuid"
	"gopkg.in/gomail.v2"
)

// DBConfig contains configuration for Firebird db
type DBConfig struct {
	user     string
	password string
	host     string
	port     string
	path     string
}

// Database config for User struct
type Database struct {
	host     string
	port     int
	path     string
	user     string
	password string
	dataDB   string
	configDB string
}

// User - Ongrid client that works with the desktop application
type User struct {
	ID        string
	Login     string
	Password  string
	FirstName string
	LastName  string
	Email     string
	DB        Database
}

// Session contains client session data
type Session struct {
	token         string
	user          *User
	queries       map[string][]ongrid2.Query
	transactionID int
	dbData        *sqlx.DB
	dbConfig      *sqlx.DB
	config        *ongrid2.ConfigObject
}

// Sessions is session array
type Sessions map[string]*Session

var sessions Sessions
var dbOnGrid *sqlx.DB
var dbConfig DBConfig
var mgoConfig MongoConfig
var mongoConnection *MongoConnection

func init() {
	config, err := yaml.ReadFile("config/ongrid.conf")
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("Config file readed..")

	dbConfig.user, _ = config.Get("dbuser")
	dbConfig.password, _ = config.Get("dbpass")
	dbConfig.host, _ = config.Get("dbhost")
	dbConfig.port, _ = config.Get("dbport")
	dbConfig.path, _ = config.Get("dbpath")

	mgoConfig.user, _ = config.Get("mgouser")
	mgoConfig.passowrd, _ = config.Get("mgopass")
	mgoConfig.host, _ = config.Get("mgohost")
	mgoConfig.dbName, _ = config.Get("mgodb")

	sessions = make(Sessions)
}

// DBHandler ...
type DBHandler struct {
}

// NewDBHandler ...
func NewDBHandler() *DBHandler {
	return &DBHandler{}
}

// OngridHandler ...
type OngridHandler struct {
}

// NewOngridHandler ...
func NewOngridHandler() *OngridHandler {
	return &OngridHandler{}
}

// Ping ...
func (p *OngridHandler) Ping() (err error) {
	fmt.Print("ping()\n")
	return nil
}

// Connect - авторизация в системе по мак адресу
func (p *OngridHandler) Connect(login string, macAddr string) (token string, err error) {
	dbOnGrid, err = sqlx.Connect("firebirdsql", dbConfig.user+":"+dbConfig.password+"@"+dbConfig.host+":"+dbConfig.port+"/"+dbConfig.path)
	if err != nil {
		log.Fatalln("Connect: ", err)
		return "", err
	}
	log.Println("Connect: System firebird database connection established")

	mongoConnection = NewMongoConnection(mgoConfig)

	token, err = authMac(login, macAddr)
	if err != nil {
		dbOnGrid.Close()
		mongoConnection.CloseConnection()
		log.Println("Connect: Unknown macaddress. Database connection closed")
		return
	}
	log.Printf("Auth.. Token = %s", token)

	return
}

// AddWorkPlace добавляет новое рабочее место в таблицу sys$workplaces
func (p *OngridHandler) AddWorkPlace(wpName, macAddr, login, password string) (token string, err error) {
	var user *User
	dbOnGrid, err = sqlx.Connect("firebirdsql", dbConfig.user+":"+dbConfig.password+"@"+dbConfig.host+":"+dbConfig.port+"/"+dbConfig.path)
	if err != nil {
		log.Fatalln("Connect: ", err)
		return "", err
	}
	log.Println("AddWorkPlace: Firebird db connection established")

	mongoConnection = NewMongoConnection(mgoConfig)

	token, user, err = authLP(login, password)
	if err != nil {
		dbOnGrid.Close()
		mongoConnection.CloseConnection()
		log.Println("AddWorkPlace: User not found. Database connection closed")
		return
	}

	mongoConnection.ClientAddWorkPlace(user.ID, wpName, macAddr)

	log.Printf("AddWorkPlace: Auth.. Token = %s", token)

	return
}

// Disconnect выход из системы
func (p *OngridHandler) Disconnect(authToken string) error {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return err
	}

	//dbOnGrid.MustExec("update sys$sessions set active = 0, closed_at = ? where token = ? and active = 1", time.Now(), authToken)
	sessions[sessionID].dbData.Close()
	sessions[sessionID].dbConfig.Close()
	dbOnGrid.Close()
	mongoConnection.CloseConnection()

	delete(sessions, sessionID)

	log.Println("Logout: Databases connections closed")

	return nil
}

// Login ...
func (p *OngridHandler) Login(login, password string) (userid int64, err error) {
	userid = 1
	return
}

/* SQL function */

// ExecuteSelectQuery выполняет sql запрос и возвращает результат
func (p *DBHandler) ExecuteSelectQuery(authToken string, query *ongrid2.Query) (*ongrid2.DataRowSet, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return nil, err
	}

	var dataRowSet ongrid2.DataRowSet

	start := time.Now()

	rows, err := sessions[sessionID].dbData.NamedQuery(query.Sql, getParams(query))
	if err != nil {
		return nil, fmt.Errorf("ExecuteSelectQuery error: %v", err)
	}
	defer rows.Close()

	columnTypes, err := rows.ColumnTypes()
	if err != nil {
		return nil, fmt.Errorf("rows.ColumnTypes error: %v", err)
	}

	for _, cType := range columnTypes {
		colomnMetadata := ongrid2.ColumnMetadata{}
		colomnMetadata.Name = cType.Name()
		switch cType.ScanType().String() {
		case "int16":
			colomnMetadata.Type = ongrid2.FieldType_INTEGER
		case "int32":
			colomnMetadata.Type = ongrid2.FieldType_INTEGER
		case "string":
			colomnMetadata.Type = ongrid2.FieldType_STRING
		case "float64":
			colomnMetadata.Type = ongrid2.FieldType_DOUBLE
		case "time.Time":
			colomnMetadata.Type = ongrid2.FieldType_DATETIME
		case "[]uint8":
			colomnMetadata.Type = ongrid2.FieldType_BLOB
		}
		if length, ok := cType.Length(); ok {
			colomnMetadata.Length = int32(length)
		}
		log.Printf("Field name: %s, type: %s, scan type: %v\n", cType.Name(), cType.DatabaseTypeName(), cType.ScanType().String())
		dataRowSet.Columns = append(dataRowSet.Columns, &colomnMetadata)
	}

	var rowsCount int

	for rows.Next() {
		dataRow := ongrid2.DataRow{}
		columnValues, err := rows.SliceScan()
		if err != nil {
			return nil, fmt.Errorf("rows.Scan: %v", err)
		}
		for _, val := range columnValues {
			dataField := ongrid2.DataField{}

			switch valT := val.(type) {
			case int16:
				IntVal := int64(valT)
				dataField.IntegerValue = &IntVal
			case int32:
				IntVal := int64(valT)
				dataField.IntegerValue = &IntVal
			case string:
				StrVal := fmt.Sprint(valT)
				dataField.StringValue = &StrVal
			case float64:
				DoubleVal := float64(valT)
				dataField.DoubleValue = &DoubleVal
			case time.Time:
				TimeVal := valT.Unix()
				dataField.DatetimeValue = &TimeVal
			case []uint8:
				BlobVal := valT
				dataField.BlobValue = BlobVal
			}

			dataRow.Fields = append(dataRow.Fields, &dataField)
		}

		dataRowSet.Rows = append(dataRowSet.Rows, &dataRow)

		rowsCount++
	}

	log.Printf("ExecuteSelectQuery complete, selected %d rows, %.2fs elapsed\n", rowsCount, time.Since(start).Seconds())

	return &dataRowSet, nil
}

// ExecuteNonSelectQuery аналог ExecSQL, не возвращает результата запроса
func (p *DBHandler) ExecuteNonSelectQuery(authToken string, query *ongrid2.Query) error {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return err
	}

	_, err = sessions[sessionID].dbData.NamedExec(query.Sql, getParams(query))
	if err != nil {
		log.Printf("ExecuteNonSelectQuery error: %v", err)
	}
	return err
}

// StartBatchExecution возвращяет новый batchId
func (p *DBHandler) StartBatchExecution(authToken string) (string, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return "", err
	}
	sessions[sessionID].transactionID++
	return string(sessions[sessionID].transactionID), nil
}

// AddQuery добавляет запрос в map queries с определенным batchId
func (p *DBHandler) AddQuery(authToken string, batchID string, query *ongrid2.Query) error {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return err
	}
	sessions[sessionID].queries[batchID] = append(sessions[sessionID].queries[batchID], *query)
	return nil
}

// FinishBatchExecution выполняет все запросы из map queries с определенным batchId
func (p *DBHandler) FinishBatchExecution(authToken string, batchID string, condition *ongrid2.Query, onSuccess *ongrid2.Query) (string, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return "", err
	}

	tx := sessions[sessionID].dbData.MustBegin()
	for _, query := range sessions[sessionID].queries[batchID] {
		_, err := tx.NamedExec(query.Sql, getParams(&query))
		if err != nil {
			return "", fmt.Errorf("FinishBatchExecute error: query: %s - %v", query.Sql, err)
		}
	}
	tx.Commit()

	_, err = sessions[sessionID].dbData.NamedExec(onSuccess.Sql, getParams(onSuccess))
	if err != nil {
		return "", fmt.Errorf("FinishBatchExecute, onSuccess error: query: %s - %v", onSuccess.Sql, err)
	}

	return "", nil
}

// BatchExecute выполняет в транзакции все запросы из queries
func (p *DBHandler) BatchExecute(authToken string, queries []*ongrid2.Query, condition *ongrid2.Query, onSuccess *ongrid2.Query) (string, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return "", err
	}
	tx := sessions[sessionID].dbData.MustBegin()
	for _, query := range queries {
		_, err := tx.NamedExec(query.Sql, getParams(query))
		if err != nil {
			return "", fmt.Errorf("BatchExecute error: query: %s - %v", query.Sql, err)
		}
	}
	tx.Commit()

	_, err = sessions[sessionID].dbData.NamedExec(onSuccess.Sql, getParams(onSuccess))
	if err != nil {
		return "", fmt.Errorf("BatchExecute, onSuccess error: query: %s - %v", onSuccess.Sql, err)
	}

	return "", nil

}

/* Other function */

// GetEvents ...
func (p *OngridHandler) GetEvents(authToken string, last int64) (events []*ongrid2.Event, err error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return nil, err
	}

	log.Println("GetEvents start..")

	var rows *sqlx.Rows

	rows, err = sessions[sessionID].dbData.Queryx("select * from igo$events where id > ? and type = 4", last)
	if err != nil {
		log.Printf("GetEvents, select * from igo$events error: %v\n", err)
		return
	}
	defer rows.Close()

	var dbEvent DBEvent
	for rows.Next() {
		err = rows.StructScan(&dbEvent)
		if err != nil {
			log.Printf("GetEvents, StructScan: %v\n", err)
		}

		if dbEvent.EventType == 4 {
			rowsMsg, err := sessions[sessionID].dbData.Queryx("select * from igo$messages where id = ?", dbEvent.ObjectID)
			if err != nil {
				log.Printf("GetEvents: %v\n", err)
				return nil, err
			}
			defer rowsMsg.Close()

			var dbMsg DBMessage

			for rowsMsg.Next() {
				err = rowsMsg.StructScan(&dbMsg)
				if err != nil {
					log.Printf("msg scan: %v\n", err)
					return nil, err
				}

				event := ongrid2.Event{}

				event.ID = int64(dbEvent.ID)
				event.Type = ongrid2.EventType_MESSAGE
				event.Message, err = getMessage(sessions[sessionID].dbData, dbEvent.ObjectID)

				events = append(events, &event)
			}
		}

		// rowsRq, err := dbOnGrid.Queryx("select * from sys$requests where id = ?", dbEvent.ObjectID)
		// if err != nil {
		// 	log.Printf("GetEvents: %v\n", err)
		// 	return nil, err
		// }
		// defer rowsRq.Close()

		// var dbRequest DBRequest
		// for rowsRq.Next() {
		// 	err = rowsRq.StructScan(&dbRequest)
		// 	if err != nil {
		// 		log.Printf("GetEvents, StructScan: %v\n", err)
		// 	}
		// 	event := ongrid2.Event{}

		// 	event.ID = int64(dbEvent.ID)
		// 	event.Type = 1
		// event.Request, err = getRequest(dbEvent.ObjectID)

		// 	events = append(events, &event)
		// }
	}

	log.Printf("events: %v", events)

	return
}

// PostEvent create or update event in backend
func (p *OngridHandler) PostEvent(authToken string, event *ongrid2.Event) (string, error) {
	if _, err := checkToken(authToken); err != nil {
		return "", err
	}

	if event.Type != ongrid2.EventType_REQUEST {
		return "", nil
	}

	request := event.Request

	var objectID int
	err := dbOnGrid.QueryRowx("select id from sys$requests where id = ?", request.ID).Scan(&objectID)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {

		} else {
			log.Printf("PostEvent, select id from sys$requests error: %v", err)
			return "", err
		}
	}

	if objectID == 0 {
		err = dbOnGrid.QueryRowx("select gen_id(gen_sys$requests_id, 1) from rdb$database").Scan(&objectID)
		if err != nil {
			log.Printf("PostEvent, select gen_id error: %v", err)
			return "", err
		}

		_, err := dbOnGrid.NamedExec("insert into sys$requests (id, userid, company, createddatetime, desireddatetime, desiredtimeperiod, phone, email, description, car, status) "+
			"values (:id, :user, :company, :createdat, :desired, :desiredperiod, :phone, :email, :descr, :car, :status)",
			map[string]interface{}{
				"id":            objectID,
				"user":          request.User.ID,
				"company":       request.Company.ID,
				"createdat":     time.Unix(request.CreatedDateTime, 0),
				"desired":       time.Unix(request.DesiredDateTime, 0),
				"desiredperiod": request.DesiredTimePeriod,
				"phone":         request.Phone,
				"email":         request.Email,
				"descr":         request.Description,
				"car":           request.Car.ID,
				"status":        request.Status,
			})
		if err != nil {
			log.Printf("PostEvent, insert into sys$requests error: %v", err)
			return "", err
		}
	} else {
		_, err = dbOnGrid.NamedExec("update sys$requests set userid = :user, company = :comapny, createddatetime = :createdat, desireddatetime = :desired, "+
			"desiredtimeperiod = :desiredperiod, phone = :phone, email = :email, description = :descr, car = :car, status = :status where id = :reqid",
			map[string]interface{}{
				"user":          request.User.ID,
				"company":       request.Company.ID,
				"createdat":     time.Unix(request.CreatedDateTime, 0),
				"desired":       time.Unix(request.DesiredDateTime, 0),
				"desiredperiod": request.DesiredTimePeriod,
				"phone":         request.Phone,
				"email":         request.Email,
				"descr":         request.Description,
				"car":           request.Car.ID,
				"status":        request.Status,
				"reqid":         objectID,
			})
		if err != nil {
			log.Printf("PostEvent, update sys$requests error: %v", err)
			return "", err
		}
	}

	var hexUUID string
	err = dbOnGrid.QueryRowx("select hex_uuid from get_hex_uuid").Scan(&hexUUID)
	if err != nil {
		log.Printf("select hex_uuid from get_hex_uuid error: %v", err)
		return "", err
	}
	log.Printf("New UUID: %s", hexUUID)

	_, err = dbOnGrid.NamedExec("insert into sys$events (id, type, objectid) values (:id, :type, :objid)",
		map[string]interface{}{
			"id":    hexUUID,
			"type":  1,
			"objid": objectID,
		})
	if err != nil {
		log.Printf("PostEvent, insert into sys$events error: %v", err)
		return "", err
	}

	return hexUUID, nil
}

// GetCentrifugoConf ...
func (p *OngridHandler) GetCentrifugoConf(authToken string) (*ongrid2.CentrifugoConf, error) {
	if _, err := checkToken(authToken); err != nil {
		return nil, err
	}

	var centrifugoConf ongrid2.CentrifugoConf

	centrifugoConf.Host = "178.128.207.49"
	centrifugoConf.Port = 8000
	centrifugoConf.Secret = "secret"

	return &centrifugoConf, nil
}

/*
	ObjectType
	0 - Field
	1 - Table
	2 - Prop
	3 - Event
	4 - Template
	5 - Menu
	6 - Toolbar
	7 - Procedure, database query
	10 - Configuration
*/

// dbConfigigObject ...
type dbConfigigObject struct {
	ID        int            `db:"OBJECTID"`
	ObjType   int            `db:"OBJECTTYPE"`
	ParamType int            `db:"PARAMTYPE"`
	Name      string         `db:"OBJECTNAME"`
	Param     sql.NullString `db:"OBJECTPARAM"`
	Value     sql.NullString `db:"PARAMVALUE"`
	AddField  sql.NullString `db:"ADDFIELD"`
	Owner     int            `db:"OBJECTOWNER"`
	Tag       sql.NullInt64  `db:"OBJECTTAG"`
}

// GetConfiguration ...
func (p *OngridHandler) GetConfiguration(authToken string) (*ongrid2.ConfigObject, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return nil, err
	}

	start := time.Now()

	if sessions[sessionID].config != nil {
		log.Printf("GetConfiguration %.2fs elapsed\n", time.Since(start).Seconds())
		log.Printf("Configuration name: %s, description: %s", sessions[sessionID].config.Name, sessions[sessionID].config.Description)
		return sessions[sessionID].config, nil
	}

	var Configuration ongrid2.ConfigObject
	baseObjects := make(map[int]*ongrid2.ConfigObject)

	rows, err := sessions[sessionID].dbConfig.Queryx("select objectid, objecttype, paramtype, objectname, objectparam, paramvalue, addfield, " +
		"coalesce(objectowner, 0) as objectowner, " +
		"coalesce(objecttag, 0) as objecttag " +
		"from igo$objects " +
		"order by coalesce(objectowner, 0), objecttag, objectname")
	if err != nil {
		log.Printf("GetConfiguration error: %v", err)
	}

	log.Println("Configuration loaded")

	var objectCount int
	var DBObject dbConfigigObject

	for rows.Next() {
		err := rows.StructScan(&DBObject)
		if err != nil {
			log.Printf("GetConfiguration, StructScan error: %v", err)
		}

		if _, ok := baseObjects[DBObject.ID]; !ok {
			object := ongrid2.ConfigObject{}

			object.ID = int64(DBObject.ID)
			object.Type = int32(DBObject.ObjType)
			object.Name = DBObject.Name
			if DBObject.Param.Valid {
				object.Description = DBObject.Param.String
			}
			object.Subtype = int32(DBObject.ParamType)

			switch DBObject.ObjType {
			case 2:
				if DBObject.Value.Valid {
					object.Value = DBObject.Value.String
				}
			case 3:
				// Type3 - events, value store in field AddField
				if DBObject.AddField.Valid {
					object.Value = DBObject.AddField.String
				}
			default:
				if DBObject.Value.Valid {
					object.Value = DBObject.Value.String
				}
			}

			if DBObject.Tag.Valid {
				object.Tag = int32(DBObject.Tag.Int64)
			}

			object.Owner = int64(DBObject.Owner)

			baseObjects[DBObject.ID] = &object

			if DBObject.Owner == 0 {
				if object.Type == 10 {
					Configuration.ID = object.ID
					Configuration.Type = object.Type
					Configuration.Name = object.Name
					if DBObject.Param.Valid {
						Configuration.Description = DBObject.Param.String
					}
				} else {
					Configuration.Objects = append(Configuration.Objects, &object)
				}
			} else {
				switch object.Type {
				case 2:
					// property
					if _, ok := baseObjects[DBObject.Owner]; ok {
						baseObjects[DBObject.Owner].Props = append(baseObjects[DBObject.Owner].Props, &object)
					}
				case 3:
					// event
					if _, ok := baseObjects[DBObject.Owner]; ok {
						baseObjects[DBObject.Owner].Events = append(baseObjects[DBObject.Owner].Events, &object)
					}
				default:
					// object
					if _, ok := baseObjects[DBObject.Owner]; ok {
						baseObjects[DBObject.Owner].Objects = append(baseObjects[DBObject.Owner].Objects, &object)
					}
				}
			}
			objectCount++
		}
	}

	log.Printf("GetConfiguration, object count = %d, %.2fs elapsed\n", objectCount, time.Since(start).Seconds())
	log.Printf("Configuration name: %s, description: %s", Configuration.Name, Configuration.Description)

	sessions[sessionID].config = &Configuration

	return &Configuration, nil
}

// dbConfigigProp ...
type dbConfigigProp struct {
	ID         int            `db:"ID"`
	ObjectType int            `db:"OBJECTTYPE"`
	ParamType  int            `db:"PARAMTYPE"`
	PropType   int            `db:"PROPTYPE"`
	PName      string         `db:"PNAME"`
	PCaption   string         `db:"PCAPTION"`
	PType      int            `db:"PTYPE"`
	PValues    sql.NullString `db:"PVALUES"`
	PDefault   sql.NullString `db:"PDEFAULT"`
	PAction    int            `db:"PACTION"`
}

// GetProps ...
func (p *OngridHandler) GetProps(authToken string) (props []*ongrid2.ConfigProp, err error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return nil, err
	}

	rows, err := sessions[sessionID].dbConfig.Queryx("select id, objecttype, paramtype, proptype, pname, " +
		"pcaption, ptype, pvalues, pdefault, paction from igo$props where isfolder = 0 " +
		"order by objecttype")

	if err != nil {
		log.Printf("GetProps error: %v", err)
		return
	}

	var DBProp dbConfigigProp

	for rows.Next() {
		err = rows.StructScan(&DBProp)
		if err != nil {
			log.Printf("GetProps, StructScan error: %v", err)
			return
		}
		prop := ongrid2.ConfigProp{}

		prop.ID = int64(DBProp.ID)
		prop.ObjectType = int32(DBProp.ObjectType)
		prop.ParamType = int32(DBProp.ParamType)
		prop.PropType = int32(DBProp.PropType)
		prop.PName = DBProp.PName
		prop.PCaption = DBProp.PCaption
		prop.PType = int32(DBProp.PType)
		if DBProp.PValues.Valid {
			prop.PValues = DBProp.PValues.String
		}
		if DBProp.PDefault.Valid {
			prop.PDefault = DBProp.PDefault.String
		}
		prop.PAction = int32(DBProp.PAction)

		props = append(props, &prop)
	}

	return
}

// GetUserPrivileges ...
func (p *OngridHandler) GetUserPrivileges(authToken string, userID int64) ([]*ongrid2.Privilege, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return nil, err
	}

	aclService := privileges.ACLService{}
	aclService.Load(sessions[sessionID].dbConfig)

	privileges, err := aclService.GetACL(int(userID))
	if err != nil {
		return nil, err
	}

	return privileges, nil
}

type dbUser struct {
	ID       int    `db:"ID"`
	Login    string `db:"LOGIN"`
	FullName string `db:"FULLNAME"`
	Password string `db:"PASSWORD"`
}

// GetUsers возвращает всех пользователей из таблицы og$users
func (p *OngridHandler) GetUsers(authToken string) (users []*ongrid2.User, err error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return nil, err
	}

	rows, err := sessions[sessionID].dbConfig.Queryx("select id, login, fullname from og$users")
	if err != nil {
		log.Printf("GetUsers error: %v", err)
		return
	}

	var DBUser dbUser

	for rows.Next() {
		err = rows.StructScan(&DBUser)
		if err != nil {
			log.Printf("GetProps, StructScan error: %v", err)
			return
		}

		user := ongrid2.User{}

		user.ID = int64(DBUser.ID)
		user.Login = DBUser.Login
		user.FullName = DBUser.FullName

		users = append(users, &user)
	}

	return
}

// RegisterCustomer - create new customer in mongodb, send him email with a login and password
func (p *OngridHandler) RegisterCustomer(authToken string, email string, name string, phone string) (string, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return "", err
	}

	log.Printf("RegisterCustomer, name = %s, email = %s\n", name, email)

	password, err := password.Generate(20, 8, 2, false, false)
	if err != nil {
		log.Printf("password generate: %v", err)
		return "", err
	}
	log.Printf("Cutomer password: %s\n", password)

	h := md5.New()
	io.WriteString(h, password)
	hpass := fmt.Sprintf("%x", h.Sum(nil))

	owner := sessions[sessionID].user.ID
	customerID, err := mongoConnection.CreateCustomer(owner, name, email, phone, hpass)
	if err != nil {
		return "", err
	}

	log.Println("Customer created...")

	m := gomail.NewMessage()
	m.SetHeader("From", "verify@ongrid.xyz")
	m.SetHeader("To", email)
	m.SetHeader("Subject", "Customer registration")
	m.SetBody("text/html", "Hello "+name+"! <br> go to <a href='http://customers.ongrid.xyz'>Customer Service</a><br>Login: "+email+
		"<br>Password: "+password)
	d := gomail.NewDialer("smtp.yandex.ru", 465, "verify@ongrid.xyz", "ak0srulez")
	if err = d.DialAndSend(m); err != nil {
		return customerID, err
	}
	return customerID, nil
}

// CheckUser ...
func (p *OngridHandler) CheckUser(authToken string, login string, password string) (*ongrid2.User, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return nil, err
	}

	DBUser := dbUser{}

	err = sessions[sessionID].dbConfig.Get(&DBUser, "select first 1 id, login, fullname, password from og$users where login = ?", login)
	if err != nil {
		if err.Error() == "sql: no rows in result set" {
			err = errors.New("User not found")
		}
		log.Printf("CheckUser: %v", err)
		return nil, err
	}

	log.Printf("User: %v", DBUser)

	if DBUser.Password != password {
		return nil, errors.New("Password incorrect")
	}

	var user ongrid2.User

	user.ID = int64(DBUser.ID)
	user.Login = DBUser.Login
	user.FullName = DBUser.FullName

	return &user, nil
}

// SendMessageToCustomer ...
func (p *OngridHandler) SendMessageToCustomer(authToken string, customerID string, body string, parentMessageID int64) (int64, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return -1, err
	}
	var lastID int64
	err = sessions[sessionID].dbData.Get(&lastID, "select gen_id(GEN_IGO$MESSAGES_ID, 1) from rdb$database")
	if err != nil {
		log.Printf("select gen_id: %v", err)
		return -1, err
	}

	_, err = sessions[sessionID].dbData.NamedExec("insert into igo$messages (id, customer, body, parentid, direction)"+
		" values (:id, :customer, :body, :parentId, :direction) returning id",
		map[string]interface{}{
			"id":        lastID,
			"customer":  customerID,
			"body":      body,
			"parentId":  parentMessageID,
			"direction": 1,
		})
	if err != nil {
		log.Printf("insert into igo$message: %v", err)
		return -1, err
	}

	log.Printf("insert into igo$message, id: %v", lastID)
	// log.Printf("rows affected: %v", rowsAffected)

	return lastID, nil
}

/* Misc function */

func getParams(query *ongrid2.Query) map[string]interface{} {
	params := make(map[string]interface{})

	for _, param := range query.Parameters {
		switch param.Type {
		case ongrid2.FieldType_INTEGER:
			params[*param.Name] = param.IntegerValue
		case ongrid2.FieldType_DOUBLE:
			params[*param.Name] = param.DoubleValue
		case ongrid2.FieldType_STRING:
			params[*param.Name] = param.StringValue
		case ongrid2.FieldType_DATETIME:
			params[*param.Name] = param.DatetimeValue
		case ongrid2.FieldType_BLOB:
			params[*param.Name] = param.BlobValue
		}
	}
	return params
}

/* Auth func */

func authMac(login string, macAddr string) (string, error) {
	user, err := mongoConnection.GetUserByMacAddr(login, macAddr)
	if err == nil {
		authToken, err := startSession(&user)
		if err != nil {
			return "", err
		}
		return authToken, nil
	}

	return "", fmt.Errorf("AuthMac failed. MacAddr: %s", macAddr)
}

func authLP(login, password string) (string, *User, error) {
	user, err := mongoConnection.GetUserByLogin(login)
	if err != nil {
		log.Printf("AuthLP: select from sys$clients: %v\n", err)
		return "", nil, err
	}
	log.Println(user)

	h := md5.New()
	io.WriteString(h, password)
	hpass := fmt.Sprintf("%x", h.Sum(nil))
	if user.Password == hpass {
		log.Println("authLP: Password correct")
		authToken, err := startSession(&user)
		if err != nil {
			return "", nil, err
		}
		return authToken, &user, nil
	}

	return "", nil, fmt.Errorf("Auth failed. Login: %s", login)
}

func getDataConnectionString(user *User) string {
	var conn string
	conn = user.DB.user + ":" + user.DB.password + "@" + user.DB.host + ":" + strconv.Itoa(user.DB.port) + "/" + user.DB.path + "/" + user.DB.dataDB
	return conn
}

func getConfigConnectionString(user *User) string {
	var conn string
	conn = user.DB.user + ":" + user.DB.password + "@" + user.DB.host + ":" + strconv.Itoa(user.DB.port) + "/" + user.DB.path + "/" + user.DB.configDB
	return conn
}

func startSession(user *User) (authToken string, err error) {
	htoken := md5.New()
	io.WriteString(htoken, user.Login)
	io.WriteString(htoken, user.Password)
	io.WriteString(htoken, time.Now().String())

	authToken = fmt.Sprintf("%x", htoken.Sum(nil))

	var sessionID string
	u4 := uuid.NewV4()
	sessionID = u4.String()

	// dbOnGrid.NamedExec("insert into sys$sessions (id, login, token, created_at, active) values (:id, :login, :token, :createdat, :active)",
	// 	map[string]interface{}{
	// 		"id":        sessionID,
	// 		"login":     user.Login,
	// 		"token":     authToken,
	// 		"createdat": time.Now(),
	// 		"active":    1,
	// 	})
	// log.Println("insert into sys$sessions complete..")

	sessions[sessionID] = &Session{token: authToken, user: user}
	log.Printf("Session id = %s\n", sessionID)

	log.Printf("User: %v\n", user)
	log.Printf("User.DB: %v\n", user.DB)
	// Connect to client database

	var dataDB, configDB string

	dataDB = getDataConnectionString(user)
	configDB = getConfigConnectionString(user)

	log.Println(dataDB)
	sessions[sessionID].dbData, err = sqlx.Connect("firebirdsql", dataDB)
	log.Printf("Client data db: %s\n", dataDB)
	if err != nil {
		log.Fatalln("startSession: connect to client db: ", err)
		return "", err
	}
	log.Println("startSession: Client data-database connection established")
	sessions[sessionID].dbConfig, err = sqlx.Connect("firebirdsql", configDB)
	log.Printf("Client config db: %s\n", configDB)
	if err != nil {
		log.Fatalln("startSession: connect to client db: ", err)
		return "", err
	}
	log.Println("startSession: Client config-database connection established")

	return
}

// checkToken проверяет активность сессии и возвращает id сессии
func checkToken(authToken string) (string, error) {
	sessionID, err := getSessionIDByToken(authToken)
	if err != nil {
		return "", fmt.Errorf("Token unknown: %v", err)
	}
	return sessionID, nil
}

func getSessionIDByToken(token string) (string, error) {
	for key, session := range sessions {
		if session.token == token {
			return key, nil
		}
	}
	return "", fmt.Errorf("Token not found")
}
