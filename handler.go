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
	"fmt"
	"io"
	"log"
	"ongrid-thrift/ongrid2"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kylelemons/go-gypsy/yaml"
	_ "github.com/nakagami/firebirdsql"
	"github.com/twinj/uuid"
)

// DBConfig contains configuration for Firebird db
type DBConfig struct {
	user     string
	password string
	host     string
	port     string
	path     string
}

// User ...
type User struct {
	ID       int    `db:"CLIENTID"`
	Login    string `db:"LOGIN"`
	Password string `db:"PASSWORD"`
	DBName   string `db:"DBNAME"`
}

// Session conains session data
type Session struct {
	token         string
	user          *User
	queries       map[string][]ongrid2.Query
	transactionID int
	db            *sqlx.DB
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
func (p *OngridHandler) Connect(macAddr string) (token string, err error) {
	// dbOnGrid, err = sqlx.Connect("firebirdsql", dbConfig.user+":"+dbConfig.password+"@"+dbConfig.host+":"+dbConfig.port+"/"+dbConfig.path)
	// if err != nil {
	// 	log.Fatalln("Connect: ", err)
	// 	return "", err
	// }
	// log.Println("Connect: System firebird database connection established")

	mongoConnection = NewMongoConnection(mgoConfig)

	token, err = authMac(macAddr)
	if err != nil {
		//dbOnGrid.Close()
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

	var wpid int

	token, user, err = authLP(login, password)
	if err != nil {
		dbOnGrid.Close()
		log.Println("AddWorkPlace: User not found. Database connection closed")
		return
	}

	dbOnGrid.QueryRowx("select id from sys$workplaces where macaddr = ?", macAddr).Scan(&wpid)
	if wpid == 0 {
		dbOnGrid.MustExec("insert into sys$workplaces (userid, wpname, macaddr) values ( ?, ?, ? )", user.ID, wpName, macAddr)
	}
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
	sessions[sessionID].db.Close()
	//dbOnGrid.Close()
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

	rows, err := sessions[sessionID].db.NamedQuery(query.Sql, getParams(query))
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

	_, err = sessions[sessionID].db.NamedExec(query.Sql, getParams(query))
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

	tx := sessions[sessionID].db.MustBegin()
	for _, query := range sessions[sessionID].queries[batchID] {
		_, err := tx.NamedExec(query.Sql, getParams(&query))
		if err != nil {
			return "", fmt.Errorf("FinishBatchExecute error: query: %s - %v", query.Sql, err)
		}
	}
	tx.Commit()

	_, err = sessions[sessionID].db.NamedExec(onSuccess.Sql, getParams(onSuccess))
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
	tx := sessions[sessionID].db.MustBegin()
	for _, query := range queries {
		_, err := tx.NamedExec(query.Sql, getParams(query))
		if err != nil {
			return "", fmt.Errorf("BatchExecute error: query: %s - %v", query.Sql, err)
		}
	}
	tx.Commit()

	_, err = sessions[sessionID].db.NamedExec(onSuccess.Sql, getParams(onSuccess))
	if err != nil {
		return "", fmt.Errorf("BatchExecute, onSuccess error: query: %s - %v", onSuccess.Sql, err)
	}

	return "", nil

}

/* Other function */

// GetEvents ...
func (p *OngridHandler) GetEvents(authToken, last string) (events []*ongrid2.Event, err error) {

	if _, err = checkToken(authToken); err != nil {
		return nil, err
	}

	log.Println("GetEvents start..")

	var rows *sqlx.Rows

	if len(last) == 32 {
		var createdAt time.Time
		err = dbOnGrid.QueryRowx("select created_at from sys$events where id = ?", last).Scan(&createdAt)
		if err != nil {
			log.Printf("GetEvents: %v\n", err)
			return
		}
		// пока считываем только реквесты (type = 1)
		rows, err = dbOnGrid.Queryx("select * from sys$events where created_at > ? and type = 1", createdAt)
	} else {
		rows, err = dbOnGrid.Queryx("select * from sys$events where type = 1")
	}

	if err != nil {
		log.Printf("GetEvents, select * from sys$events error: %v\n", err)
		return
	}
	defer rows.Close()

	var dbEvent DBEvent
	for rows.Next() {
		err = rows.StructScan(&dbEvent)
		if err != nil {
			log.Printf("GetEvents, StructScan: %v\n", err)
		}

		rowsRq, err := dbOnGrid.Queryx("select * from sys$requests where id = ?", dbEvent.ObjectId)
		if err != nil {
			log.Printf("GetEvents: %v\n", err)
			return nil, err
		}
		defer rowsRq.Close()

		var dbRequest DBRequest
		for rowsRq.Next() {
			err = rowsRq.StructScan(&dbRequest)
			if err != nil {
				log.Printf("GetEvents, StructScan: %v\n", err)
			}
			event := ongrid2.Event{}

			event.ID = dbEvent.Id
			event.Type = 1
			event.Request, err = getRequest(dbEvent.ObjectId)

			events = append(events, &event)
		}
	}

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

	centrifugoConf.Host = "165.227.139.6"
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
	7 - Procedure
	10 - Configuration
*/

// dbConfigigObject ...
type dbConfigigObject struct {
	ID          int            `db:"OBJECTID"`
	ObjType     int            `db:"OBJECTTYPE"`
	ParamType   int            `db:"PARAMTYPE"`
	Name        string         `db:"OBJECTNAME"`
	Param       sql.NullString `db:"OBJECTPARAM"`
	Value       sql.NullString `db:"PARAMVALUE"`
	AddField    sql.NullString `db:"ADDFIELD"`
	Owner       int            `db:"OBJECTOWNER"`
	GrpVis      sql.NullString `db:"GRPVIS"`
	GrpCreate   sql.NullString `db:"GRPCREATE"`
	GrpEdit     sql.NullString `db:"GRPEDIT"`
	GrpDelete   sql.NullString `db:"GRPDELETE"`
	GrpReadonly sql.NullString `db:"GRPRONLY"`
	GrpValue    sql.NullString `db:"GRPVALUE"`
	Tag         sql.NullInt64  `db:"OBJECTTAG"`
}

// GetConfiguration ...
func (p *OngridHandler) GetConfiguration(authToken string, userId int64) (*ongrid2.ConfigObject, error) {
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

	rows, err := sessions[sessionID].db.Queryx("select o.objectid, o.objecttype, o.paramtype, o.objectname, o.objectparam, o.paramvalue, o.addfield, "+
		"coalesce(o.objectowner, 0) as objectowner, "+
		"p1.a_visible as grpvis, p1.a_create as grpcreate, p1.a_edit as grpedit, p1.a_delete as grpdelete, "+
		"p1.a_readonly as grpronly, p1.a_value as grpvalue, "+
		"coalesce(o.objecttag, 0) as objecttag "+
		"from igo$objects o "+
		"left join igo$permission p1 on (p1.objectid = o.objectid and p1.userid in (select parent from igo$users where userid = ? )) "+
		"where coalesce(o.isfolder, 0) = 0 and coalesce(o.deleted, 0) = 0 "+
		"order by o.objectkind, coalesce(o.objectowner, 0), o.objecttag, o.objectname", userId)
	if err != nil {
		log.Printf("GetConfiguration error: %v", err)
	}

	var objectCount int
	var DBObject dbConfigigObject

	for rows.Next() {
		err := rows.StructScan(&DBObject)
		if err != nil {
			log.Printf("GetConfiguration, StructScan error: %v", err)
		}

		if obj, ok := baseObjects[DBObject.ID]; ok {
			if DBObject.GrpCreate.Valid && DBObject.GrpCreate.String == "+" {
				obj.Permission.ObjectCreate = true
			}
			if DBObject.GrpEdit.Valid && DBObject.GrpEdit.String == "+" {
				obj.Permission.ObjectEdit = true
			}
			if DBObject.GrpDelete.Valid && DBObject.GrpDelete.String == "+" {
				obj.Permission.ObjectEdit = true
			}
			if DBObject.GrpReadonly.Valid && DBObject.GrpReadonly.String == "-" {
				obj.Permission.ObjectReadonly = false
			}
		} else {
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
				if DBObject.GrpValue.Valid && (len(DBObject.GrpValue.String) > 0) {
					object.Value = DBObject.GrpValue.String
				} else {
					if DBObject.Value.Valid {
						object.Value = DBObject.Value.String
					}
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

			// Permissions (dpFalse)

			permission := ongrid2.ConfigPermission{}

			permission.ObjectId = int64(DBObject.ID)
			if DBObject.GrpValue.Valid {
				permission.ObjectValue = DBObject.GrpValue.String
			}

			permission.ObjectVisible = true
			permission.ObjectReadonly = true

			if DBObject.GrpVis.Valid && DBObject.GrpVis.String == "+" {
				permission.ObjectReadonly = true
			}
			if DBObject.GrpVis.Valid && DBObject.GrpVis.String == "-" {
				permission.ObjectReadonly = false
			}
			if DBObject.GrpCreate.Valid && DBObject.GrpCreate.String == "+" {
				permission.ObjectCreate = true
			}
			if DBObject.GrpCreate.Valid && DBObject.GrpCreate.String == "-" {
				permission.ObjectCreate = false
			}
			if DBObject.GrpEdit.Valid && DBObject.GrpEdit.String == "+" {
				permission.ObjectEdit = true
			}
			if DBObject.GrpEdit.Valid && DBObject.GrpEdit.String == "-" {
				permission.ObjectEdit = false
			}
			if DBObject.GrpDelete.Valid && DBObject.GrpDelete.String == "+" {
				permission.ObjectDelete = true
			}
			if DBObject.GrpDelete.Valid && DBObject.GrpDelete.String == "-" {
				permission.ObjectDelete = false
			}
			if DBObject.GrpReadonly.Valid && DBObject.GrpReadonly.String == "+" {
				permission.ObjectReadonly = true
			}
			if DBObject.GrpReadonly.Valid && DBObject.GrpReadonly.String == "-" {
				permission.ObjectReadonly = false
			}

			object.Permission = &permission

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
					baseObjects[DBObject.Owner].Props = append(baseObjects[DBObject.Owner].Props, &object)
				case 3:
					// event
					baseObjects[DBObject.Owner].Events = append(baseObjects[DBObject.Owner].Events, &object)
				default:
					// object
					baseObjects[DBObject.Owner].Objects = append(baseObjects[DBObject.Owner].Objects, &object)
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

	rows, err := sessions[sessionID].db.Queryx("select id, objecttype, paramtype, proptype, pname, " +
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

// DBConfigPermission ...
type DBConfigPermission struct {
	ID             int            `db:"ID"`
	ObjectID       int            `db:"OBJECTID"`
	UserID         int            `db:"USERID"`
	ObjectVisible  string         `db:"A_VISIBLE"`
	ObjectReadonly string         `db:"A_READONLY"`
	ObjectCreate   string         `db:"A_CREATE"`
	ObjectEdit     string         `db:"A_EDIT"`
	ObjectDelete   string         `db:"A_DELETE"`
	ObjectValue    sql.NullString `db:"A_VALUE"`
}

// GetPermissions ...
func (p *OngridHandler) GetPermissions(authToken string, userId int64) ([]*ongrid2.ConfigPermission, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return nil, err
	}

	rows, err := sessions[sessionID].db.Queryx("select id, objectid, userid, a_visible, a_readonly, " +
		"a_create, a_edit, a_delete, a_value from igo$permission")

	if err != nil {
		log.Printf("GetPermissions error: %v", err)
		return nil, err
	}

	var DBPermission DBConfigPermission
	permissions := []*ongrid2.ConfigPermission{}

	for rows.Next() {
		err = rows.StructScan(&DBPermission)
		if err != nil {
			log.Printf("GetPermissions, StructScan error: %v", err)
			return nil, err
		}

		permission := ongrid2.ConfigPermission{}

		permission.ID = int64(DBPermission.ID)
		permission.ObjectId = int64(DBPermission.ObjectID)
		permission.UserId = int64(DBPermission.UserID)
		if DBPermission.ObjectVisible == "+" {
			permission.ObjectVisible = true
		}
		if DBPermission.ObjectReadonly == "+" {
			permission.ObjectReadonly = true
		}
		if DBPermission.ObjectCreate == "+" {
			permission.ObjectCreate = true
		}
		if DBPermission.ObjectEdit == "+" {
			permission.ObjectEdit = true
		}
		if DBPermission.ObjectDelete == "+" {
			permission.ObjectDelete = true
		}
		if DBPermission.ObjectValue.Valid {
			permission.ObjectValue = DBPermission.ObjectValue.String
		}

		permissions = append(permissions, &permission)
	}

	return permissions, nil
}

// SetPermission ...
func (p *OngridHandler) SetPermission(authToken string, permission *ongrid2.ConfigPermission) error {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return err
	}

	permissionData := map[string]interface{}{}

	if permission.ObjectVisible {
		permissionData["avis"] = "+"
	} else {
		permissionData["avis"] = "-"
	}
	if permission.ObjectReadonly {
		permissionData["aro"] = "+"
	} else {
		permissionData["aro"] = "-"
	}
	if permission.ObjectCreate {
		permissionData["ac"] = "+"
	} else {
		permissionData["ac"] = "-"
	}
	if permission.ObjectEdit {
		permissionData["ae"] = "+"
	} else {
		permissionData["ae"] = "-"
	}
	if permission.ObjectDelete {
		permissionData["ad"] = "+"
	} else {
		permissionData["ad"] = "-"
	}
	permissionData["av"] = permission.ObjectValue

	if permission.ID > 0 {
		_, err := sessions[sessionID].db.NamedExec("update igo$permission set a_visible = :avis, a_readonly = :aro, "+
			"a_create = :ac, a_edit = :ae, a_delete = :ad, a_value = :av",
			permissionData)
		if err != nil {
			log.Printf("SetPermissions error: %v", err)
			return err
		}
	} else {
		permissionData["oid"] = permission.ObjectId
		permissionData["uid"] = permission.UserId
		_, err := sessions[sessionID].db.NamedExec("insert into igo$permission (objectid, userid, a_visible, a_readonly, "+
			"a_create, a_edit, a_delete, a_value) values (:oid, :uid, :avis, :aro, :ac, :ae, :ad, :av)",
			permissionData)
		if err != nil {
			log.Printf("SetPermissions error: %v", err)
			return err
		}
	}

	return nil
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

func authMac(macAddr string) (string, error) {
	var user User

	user, err := mongoConnection.GetUserByMacAddr(macAddr)
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
	var user User

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
		log.Println("Password correct")
		authToken, err := startSession(&user)
		if err != nil {
			return "", nil, err
		}
		return authToken, &user, nil
	} else {
		log.Println("pass incorrect!")
	}

	return "", nil, fmt.Errorf("Auth failed. Login: %s", login)
}

func startSession(user *User) (authToken string, err error) {
	htoken := md5.New()
	io.WriteString(htoken, user.Login)
	io.WriteString(htoken, user.Password)
	io.WriteString(htoken, time.Now().String())

	authToken = fmt.Sprintf("%x", htoken.Sum(nil))

	var sessionID string
	u4 := uuid.NewV4()
	//sessionID = string(u4[:])
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
	// Connect to client database
	fmt.Println(user.DBName)
	sessions[sessionID].db, err = sqlx.Connect("firebirdsql", user.DBName)
	log.Printf("Client db: %s\n", user.DBName)
	if err != nil {
		log.Fatalln("Connect to client db: ", err)
		return "", err
	}
	log.Println("Login: Client database connection established")

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
