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
	"fmt"
	"crypto/md5"
	"io"
	"time"
	//"strconv"
	"ongrid-thrift/ongrid2"
	_ "database/sql"
	"log"
	"github.com/jmoiron/sqlx"
	//_ "github.com/lib/pq"
	_ "github.com/nakagami/firebirdsql"
)

type User struct {
	Id 			int  	`db:"ID"`
	Login 		string 	`db:"LOGIN"`
	Password	string 	`db:"PASSWORD"`
}

var queries map[string][]ongrid2.Query
var transactionId int
var db *sqlx.DB

type IntergridHandler struct {
	
}

func NewIntergridHandler() *IntergridHandler {
	return &IntergridHandler{}
}

func (p *IntergridHandler) Ping() (err error) {
	fmt.Print("ping()\n")
	return nil
}

func (p *IntergridHandler) Add(num1 int32, num2 int32) (retval17 int32, err error) {
	fmt.Print("add(", num1, ",", num2, ")\n")
	return num1 + num2, nil
}

func (p *IntergridHandler) Zip() (err error) {
	fmt.Print("zip()\n")
	return nil
}

func (p *IntergridHandler) Login(login, password string) (token string, err error) {
	//db, err := sqlx.Connect("postgres", "postgres://postgres:cwlybjel@localhost/OnGrid?sslmode=disable")
	db, err = sqlx.Connect("firebirdsql", "sysdba:masterkey@localhost:3050/c:/database/infiniti/inter_akos.gdb")
	if err != nil {
		log.Fatalln("Connect: ", err)
		return "", err
	}
	log.Println("Database connection established")
	token, err = Auth(login, password); 
	return
}

func (p *IntergridHandler) Logout(authToken string) error {
	db.MustExec("update sys$sessions set active = 0, closed_at = ? where token = ? and active = 1", time.Now(), authToken)
	db.Close()

	return nil
}

func (p *IntergridHandler) ExecuteSelectQuery(authToken string, query *ongrid2.Query) (*ongrid2.DataRowSet, error) {
	if err := checkToken(authToken); err != nil {
		return nil, err
	}
	
	var dataRowSet ongrid2.DataRowSet

	rows, err := db.NamedQuery(query.Sql, GetParams(query))
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
			case "int16": 		colomnMetadata.Type = ongrid2.FieldType_INTEGER
			case "int32": 		colomnMetadata.Type = ongrid2.FieldType_INTEGER
			case "string": 		colomnMetadata.Type = ongrid2.FieldType_STRING
			case "float64": 	colomnMetadata.Type = ongrid2.FieldType_DOUBLE
			case "time.Time": 	colomnMetadata.Type = ongrid2.FieldType_DATETIME
			case "[]uint8": 	colomnMetadata.Type = ongrid2.FieldType_BLOB
		}
		if length, ok := cType.Length(); ok {
			colomnMetadata.Length = int32(length)
		}
		log.Printf("Field name: %s, type: %s, scan type: %v\n", cType.Name(), cType.DatabaseTypeName(), cType.ScanType().String())
		dataRowSet.Columns = append(dataRowSet.Columns, &colomnMetadata)
	}
	
	for rows.Next() {
		dataRow := ongrid2.DataRow{}
		columnValues, err := rows.SliceScan()
		if err != nil {
			return nil, fmt.Errorf("rows.Scan: %v", err)
		}
		for _, val := range columnValues {
			dataField := ongrid2.DataField{}
			
			switch valT := val.(type) {
				case int16: IntVal := int64(valT); dataField.IntegerValue = &IntVal
				case int32: IntVal := int64(valT); dataField.IntegerValue = &IntVal
				case string: StrVal := fmt.Sprint(valT); dataField.StringValue = &StrVal
				case float64: DoubleVal := float64(valT); dataField.DoubleValue = &DoubleVal
				case time.Time: TimeVal := valT.Unix(); dataField.DatetimeValue = &TimeVal
				case []uint8: BlobVal := valT; dataField.BlobValue = BlobVal
			}


			dataRow.Fields = append(dataRow.Fields, &dataField)
		}
		//fmt.Println(columnValues)
		dataRowSet.Rows = append(dataRowSet.Rows, &dataRow)
	}

	return &dataRowSet, nil
}

func (p *IntergridHandler) ExecuteNonSelectQuery(authToken string, query *ongrid2.Query) error {
	if err := checkToken(authToken); err != nil {
		return err
	}
	
	_, err := db.NamedExec(query.Sql, GetParams(query))
	if err != nil {
		log.Printf("ExecuteNonSelectQuery error: %v", err)
	}
	return err
}

func (p *IntergridHandler) StartBatchExecution(authToken string) (string, error) {
	if err := checkToken(authToken); err != nil {
		return "", err
	}
	transactionId += 1
	return string(transactionId), nil
}

func (p *IntergridHandler) AddQuery(authToken string, batchId string, query *ongrid2.Query) error {
	if err := checkToken(authToken); err != nil {
		return err
	}
	queries[batchId] = append(queries[batchId], *query)
	return nil
}

func (p *IntergridHandler) FinishBatchExecution(authToken string, batchId string, condition *ongrid2.Query, onSuccess *ongrid2.Query) (string, error) {
	if err := checkToken(authToken); err != nil {
		return "", err
	}

	tx := db.MustBegin()
	for _, query := range queries[batchId] {
		_, err := tx.NamedExec(query.Sql, GetParams(&query))
		if err != nil {
			return "", fmt.Errorf("FinishBatchExecute error: query: %s - %v", query.Sql, err)
		}
	}
	tx.Commit()

	_, err := db.NamedExec(onSuccess.Sql, GetParams(onSuccess))
	if err != nil {
		return "", fmt.Errorf("FinishBatchExecute, onSuccess error: query: %s - %v", onSuccess.Sql, err)
	}

	return "", nil
}

func (p *IntergridHandler) BatchExecute(authToken string, queries []*ongrid2.Query, condition *ongrid2.Query, onSuccess *ongrid2.Query) (string, error) {
	if err := checkToken(authToken); err != nil {
		return "", err
	}
	tx := db.MustBegin()
	for _, query := range queries {
		_, err := tx.NamedExec(query.Sql, GetParams(query))
		if err != nil {
			return "", fmt.Errorf("BatchExecute error: query: %s - %v", query.Sql, err)
		}
	}
	tx.Commit()

	_, err := db.NamedExec(onSuccess.Sql, GetParams(onSuccess))
	if err != nil {
		return "", fmt.Errorf("BatchExecute, onSuccess error: query: %s - %v", onSuccess.Sql, err)
	}

	return "", nil
}

func GetParams(query *ongrid2.Query) map[string]interface{} {
	params := make(map[string]interface{})

	for _, param := range query.Parameters {
		switch param.Type {
			case ongrid2.FieldType_INTEGER: 	params[*param.Name] = param.IntegerValue
			case ongrid2.FieldType_DOUBLE: 		params[*param.Name] = param.DoubleValue
			case ongrid2.FieldType_STRING: 		params[*param.Name] = param.StringValue
			case ongrid2.FieldType_DATETIME: 	params[*param.Name] = param.DatetimeValue
			case ongrid2.FieldType_BLOB: 		params[*param.Name] = param.BlobValue
		}
	}
	return params
}

func Auth(login, password string) (authToken string, err error) {
	h := md5.New()
	io.WriteString(h, password)
	hpass := fmt.Sprintf("%x", h.Sum(nil))

	var user User

	err = db.Get(&user, "select id,login,password from sys$auth where login = ?", login)
	if err != nil {
		log.Fatalln("Get: ", err)
	}
	
	if user.Password == hpass {
		htoken := md5.New()
		io.WriteString(htoken, login)
		io.WriteString(htoken, password)
		io.WriteString(htoken, time.Now().String())

		authToken = fmt.Sprintf("%x", htoken.Sum(nil))

		db.MustExec("insert into sys$sessions (login,token,created_at,active) values ( ? , ? , ? , ? )", login, authToken, time.Now(), 1)
		
		return authToken, nil
	}

    return "", fmt.Errorf("Auth failed. Login: %s", login)
}

func checkToken(authToken string) error {
	var sessionId int
	err := db.QueryRowx("select id from sys$sessions where token = ? and active = 1", authToken).Scan(&sessionId)
	if err != nil {
		return fmt.Errorf("Token unknown: %v", err)
	}
	return nil
}