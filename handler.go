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
	_ "database/sql"
	"fmt"
	"io"
	"log"
	"ongrid-thrift/ongrid2"
	"time"

	"github.com/jmoiron/sqlx"
	"github.com/kylelemons/go-gypsy/yaml"
	_ "github.com/nakagami/firebirdsql"
)

type DBConfig struct {
	User     string
	Password string
	Host     string
	Port     string
	Path     string
}

type User struct {
	Id       int    `db:"CLIENTID"`
	Login    string `db:"LOGIN"`
	Password string `db:"PASSWORD"`
	DBName   string `db:"DBNAME"`
}

type Session struct {
	token         string
	user          *User
	queries       map[string][]ongrid2.Query
	transactionID int
	db            *sqlx.DB
}

type Sessions map[int]*Session

var sessions Sessions
var dbOnGrid *sqlx.DB
var dbConf DBConfig

func init() {
	config, err := yaml.ReadFile("config/ongrid.conf")
	if err != nil {
		log.Fatalln(err)
	}
	log.Println("Config file readed..")

	dbConf.User, _ = config.Get("dbuser")
	dbConf.Password, _ = config.Get("dbpass")
	dbConf.Host, _ = config.Get("dbhost")
	dbConf.Port, _ = config.Get("dbport")
	dbConf.Path, _ = config.Get("dbpath")

	sessions = make(Sessions)
}

type IntergridHandler struct {
}

func NewIntergridHandler() *IntergridHandler {
	return &IntergridHandler{}
}

func (p *IntergridHandler) Ping() (err error) {
	fmt.Print("ping()\n")
	return nil
}

func (p *IntergridHandler) Zip() (err error) {
	fmt.Print("zip()\n")
	return nil
}

// Login - авторизация в системе по мак адресу
func (p *IntergridHandler) Login(macAddr string) (token string, err error) {
	dbOnGrid, err = sqlx.Connect("firebirdsql", dbConf.User+":"+dbConf.Password+"@"+dbConf.Host+":"+dbConf.Port+"/"+dbConf.Path)
	if err != nil {
		log.Fatalln("Connect: ", err)
		return "", err
	}
	log.Println("Login: System database connection established")

	token, err = authMac(macAddr)
	if err != nil {
		dbOnGrid.Close()
		log.Println("Login: Database connection closed")
		return
	}
	log.Printf("Auth.. Token = %s", token)

	return
}

// Logout - выход из системы
func (p *IntergridHandler) Logout(authToken string) error {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return err
	}

	dbOnGrid.MustExec("update sys$sessions set active = 0, closed_at = ? where token = ? and active = 1", time.Now(), authToken)
	sessions[sessionID].db.Close()
	dbOnGrid.Close()

	delete(sessions, sessionID)

	log.Println("Logout: Databases connections closed")

	return nil
}

// AddWorkPlace добавляет новое рабочее место в таблицу sys$workplaces
func (p *IntergridHandler) AddWorkPlace(wpName, macAddr, login, password string) (token string, err error) {
	var user *User
	dbOnGrid, err = sqlx.Connect("firebirdsql", dbConf.User+":"+dbConf.Password+"@"+dbConf.Host+":"+dbConf.Port+"/"+dbConf.Path)
	if err != nil {
		log.Fatalln("Connect: ", err)
		return "", err
	}
	log.Println("AddWorkPlace: Database connection established")

	var wpid int

	token, user, err = authLP(login, password)
	if err != nil {
		dbOnGrid.Close()
		log.Println("AddWorkPlace: Database connection closed")
		return
	}

	dbOnGrid.QueryRowx("select id from sys$workplaces where macaddr = ?", macAddr).Scan(&wpid)
	if wpid == 0 {
		dbOnGrid.MustExec("insert into sys$workplaces (userid, wpname, macaddr) values ( ?, ?, ? )", user.Id, wpName, macAddr)
	}
	log.Printf("AddWorkPlace: Auth.. Token = %s", token)

	return
}

/* SQL function */

// ExecuteSelectQuery выполняет sql запрос и возвращает результат
func (p *IntergridHandler) ExecuteSelectQuery(authToken string, query *ongrid2.Query) (*ongrid2.DataRowSet, error) {
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
func (p *IntergridHandler) ExecuteNonSelectQuery(authToken string, query *ongrid2.Query) error {
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
func (p *IntergridHandler) StartBatchExecution(authToken string) (string, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return "", err
	}
	sessions[sessionID].transactionID += 1
	return string(sessions[sessionID].transactionID), nil
}

// AddQuery добавляет запрос в map queries с определенным batchId
func (p *IntergridHandler) AddQuery(authToken string, batchId string, query *ongrid2.Query) error {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return err
	}
	sessions[sessionID].queries[batchId] = append(sessions[sessionID].queries[batchId], *query)
	return nil
}

// FinishBatchExecution выполняет все запросы из map queries с определенным batchId
func (p *IntergridHandler) FinishBatchExecution(authToken string, batchId string, condition *ongrid2.Query, onSuccess *ongrid2.Query) (string, error) {
	sessionID, err := checkToken(authToken)
	if err != nil {
		return "", err
	}

	tx := sessions[sessionID].db.MustBegin()
	for _, query := range sessions[sessionID].queries[batchId] {
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
func (p *IntergridHandler) BatchExecute(authToken string, queries []*ongrid2.Query, condition *ongrid2.Query, onSuccess *ongrid2.Query) (string, error) {
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
type Request struct {
	Id                int       `db:"ID"`
	User              int       `db:"USER"`
	Company           int       `db:"company"`
	CreatedDateTime   time.Time `db:"CREATEDAT"`
	DesiredDateTime   time.Time `db:"DESIREDAT"`
	DesiredTimePeriod int       `db:"DESIREDTIME"`
	Phone             string    `db:"PHONE"`
	Email             string    `db:"EMAIL"`
	Description       string    `db:"DESCRIPTION"`
	Car               int       `db:"CAR"`
	CheckInDateTime   time.Time `db:"CHECKIN"`
	CheckOutDateTime  time.Time `db:"CHECKOUT"`
	Status            int       `db:"STATUS"`
	MasterInspector   string    `db:"MASTER"`
}

// TODO: возврат полного request'а
func (p *IntergridHandler) GetEvents(authToken, last string) (events []*ongrid2.Event, err error) {
	var event ongrid2.Event
	var dbRequest Request
	var request ongrid2.Request

	if _, err = checkToken(authToken); err != nil {
		return nil, err
	}

	rows, err := dbOnGrid.Queryx("select * from sys$requests where id > ?", last)
	if err != nil {
		log.Printf("GetEvents: %v\n", err)
		return
	}
	for rows.Next() {
		err = rows.StructScan(&dbRequest)
		if err != nil {
			log.Printf("GetEvents: StructScan: %v\n", err)
		}
		request.ID = int32(dbRequest.Id)
		//request.User =
		//request.Company =
		request.CreatedDateTime = dbRequest.CreatedDateTime.Unix()
		request.DesiredDateTime = dbRequest.DesiredDateTime.Unix()
		request.DesiredTimePeriod = int32(dbRequest.DesiredTimePeriod)
		request.Phone = dbRequest.Phone
		request.Email = dbRequest.Email
		request.Description = dbRequest.Description
		//request.Car = 1
		request.CheckInDateTime = dbRequest.CheckInDateTime.Unix()
		request.CheckOutDateTime = dbRequest.CheckOutDateTime.Unix()
		request.Status = ongrid2.RequestStatus(dbRequest.Status)
		request.MasterInspector = dbRequest.MasterInspector

		event.ID = fmt.Sprintf("%d", dbRequest.Id)
		event.Type = ongrid2.EventType_REQUEST
		event.Request = &request

		events = append(events, &event)
	}

	return
}

// PostEvent create or update event in backend
func (p *IntergridHandler) PostEvent(authToken string, event *ongrid2.Event) error {
	if _, err := checkToken(authToken); err != nil {
		return err
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
	var clientid int
	var user User

	err := dbOnGrid.QueryRowx("select userid from sys$workplaces where macaddr = ?", macAddr).Scan(&clientid)
	if err != nil {
		log.Printf("AuthMac: select userid from sys$workplaces: %v\n", err)
		return "", err
	}

	if clientid != 0 {

		err = dbOnGrid.Get(&user, "select clientid, login, password, dbname from sys$auth where clientid = ?", clientid)
		if err != nil {
			log.Printf("AuthMac: select from sys$clients: %v\n", err)
			return "", err
		}

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

	err := dbOnGrid.Get(&user, "select clientid, login, password, dbname from sys$auth where login = ?", login)
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

	var sessionID int
	err = dbOnGrid.QueryRowx("select gen_id(gen_sys$sessions_id, 1) from rdb$database").Scan(&sessionID)
	if err != nil {
		return "", err
	}
	dbOnGrid.NamedExec("insert into sys$sessions (id, login, token, created_at, active) values (:id, :login, :token, :createdat, :active)",
		map[string]interface{}{
			"id":        sessionID,
			"login":     user.Login,
			"token":     authToken,
			"createdat": time.Now(),
			"active":    1,
		})
	log.Println("insert into sys$sessions complete..")

	sessions[sessionID] = &Session{token: authToken, user: user}
	log.Printf("Session id = %d", sessionID)
	// Connect to client database
	sessions[sessionID].db, err = sqlx.Connect("firebirdsql", user.DBName)
	log.Printf("Client db: %s", user.DBName)
	if err != nil {
		log.Fatalln("Connect to client db: ", err)
		return "", err
	}
	log.Println("Login: Client database connection established")

	return
}

// checkToken проверяет активность сессии и возвращает id сессии
func checkToken(authToken string) (int, error) {
	//err := dbOnGrid.QueryRowx("select id from sys$sessions where token = ? and active = 1", authToken).Scan(&sessionId)
	sessionID, err := getSessionIdByToken(authToken)
	if err != nil {
		return 0, fmt.Errorf("Token unknown: %v", err)
	}
	return sessionID, nil
}

func getSessionIdByToken(token string) (int, error) {
	for key, session := range sessions {
		if session.token == token {
			return key, nil
		}
	}
	return 0, fmt.Errorf("Token not found")
}
