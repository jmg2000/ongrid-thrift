package main

import (
	"database/sql"
	"database/sql/driver"
	"log"
	"ongrid-thrift/ongrid2"
	"time"
)

// DBAuth ...
type DBAuth struct {
	ID          int    `db:"CLIENTID" json:"id"`
	Login       string `db:"LOGIN" json:"login"`
	Password    string `db:"PASSWORD" json:"password"`
	Description string `db:"DESCRIPTION" json:"description"`
}

// DBRequest ...
type DBRequest struct {
	ID                int            `db:"ID"`
	User              int            `db:"USERID"`
	Company           int            `db:"COMPANY"`
	CreatedDateTime   NullTime       `db:"CREATEDDATETIME"`
	DesiredDateTime   NullTime       `db:"DESIREDDATETIME"`
	DesiredTimePeriod sql.NullInt64  `db:"DESIREDTIMEPERIOD"`
	Phone             sql.NullString `db:"PHONE"`
	Email             sql.NullString `db:"EMAIL"`
	Description       sql.NullString `db:"DESCRIPTION"`
	Car               int            `db:"CAR"`
	CheckInDateTime   NullTime       `db:"CHECKIN"`
	CheckOutDateTime  NullTime       `db:"CHECKOUT"`
	Status            int            `db:"STATUS"`
	MasterInspector   sql.NullString `db:"MASTER"`
}

// DBClient ...
type DBClient struct {
	ID               int           `db:"ID"`
	Email            string        `db:"EMAIL"`
	Name             string        `db:"NAME"`
	AccountType      int           `db:"ACCOUNTTYPE"`
	ClientType       int           `db:"CLIENTTYPE"`
	RegistrationDate NullTime      `db:"REGISTRATIONDATE"`
	Phone            string        `db:"PHONE"`
	Person           sql.NullInt64 `db:"PERSON"`
	Company          sql.NullInt64 `db:"COMPANY"`
}

// DBCar ...
type DBCar struct {
	ID           int      `db:"ID"`
	Brand        string   `db:"BRAND"`
	Model        string   `db:"MODEL"`
	Number       string   `db:"NUMBER"`
	Year         int32    `db:"FYEAR"`
	Mileage      int32    `db:"MILEAGE"`
	EngineVolume float64  `db:"ENGINEVOLUME"`
	EngineType   int      `db:"ENGINETYPE"`
	GearType     int      `db:"GEARTYPE"`
	BodyType     int      `db:"BODYTYPE"`
	DriveType    int      `db:"DRIVETYPE"`
	VIN          string   `db:"VIN"`
	CarTraider   string   `db:"CARTRAIDER"`
	SaleDate     NullTime `db:"SALEDATE"`
	Color        string   `db:"COLOR"`
	Owner        string   `db:"OWNER"`
}

// DBPerson ...
type DBPerson struct {
	ID             int            `db:"ID"`
	FirstName      string         `db:"FIRSTNAME"`
	LastName       string         `db:"LASTNAME"`
	PassportNumber sql.NullString `db:"PASSPORTNUMBER"`
	PassportSeries sql.NullString `db:"PASSPORTSERIES"`
	PassportDate   sql.NullString `db:"PASSPORTDATE"`
	BirthDay       NullTime       `db:"BIRTHDAY"`
	Gender         int            `db:"GENDER"`
}

// DBCompany ...
type DBCompany struct {
	ID                   int            `db:"ID"`
	ServiceName          string         `db:"SERVICENAME"`
	Phone                sql.NullString `db:"PHONE"`
	LegalForm            sql.NullString `db:"LEGALFORM"`
	FullName             sql.NullString `db:"FULLNAME"`
	Inn                  sql.NullString `db:"INN"`
	Kpp                  sql.NullString `db:"KPP"`
	LegalAddress         sql.NullString `db:"LEGALADDRESS"`
	BankAccountNumber    sql.NullString `db:"BANKACCOUNTNUMBER"`
	CorrespondentAccount sql.NullString `db:"CORRESPONDENTACCOUNT"`
	BankCode             sql.NullString `db:"BANKCODE"`
	Bank                 sql.NullString `db:"BANK"`
	Ceo                  sql.NullString `db:"CEO"`
	ChiefAccountant      sql.NullString `db:"CHIEFACCOUNTANT"`
	RealAddress          sql.NullString `db:"REALADDRESS"`
}

// DBEvent ...
type DBEvent struct {
	ID        string    `db:"ID"`
	EventType int       `db:"TYPE"`
	ObjectID  int       `db:"OBJECTID"`
	CreatedAt time.Time `db:"CREATED_AT"`
}

// NullTime ...
type NullTime struct {
	Time  time.Time
	Valid bool // Valid is true if Time is not NULL
}

// Scan implements the Scanner interface.
func (nt *NullTime) Scan(value interface{}) error {
	nt.Time, nt.Valid = value.(time.Time)
	return nil
}

// Value implements the driver Valuer interface.
func (nt NullTime) Value() (driver.Value, error) {
	if !nt.Valid {
		return nil, nil
	}
	return nt.Time, nil
}

func getClient(clientID int) (*ongrid2.Client, error) {
	client := ongrid2.Client{}
	var dbClient DBClient

	err := dbOnGrid.Get(&dbClient, "select id, email, name, accounttype, clienttype, registrationdate, phone, person, company from sys$clients where id = ?", clientID)
	if err != nil {
		log.Printf("dbOnGrid.Get from sys$client error: %v", err)
		return nil, err
	}

	client.ID = int64(dbClient.ID)
	client.Email = &dbClient.Email
	client.Name = &dbClient.Name
	client.AccountType = ongrid2.AccountType(dbClient.AccountType)
	client.ClientType = ongrid2.ClientType(dbClient.ClientType)
	client.RegistrationDate = dbClient.RegistrationDate.Time.Unix()
	client.Phone = dbClient.Phone
	if dbClient.Person.Valid {
		person := getPerson(dbClient.Person.Int64)
		client.Person = person
	}
	if dbClient.Company.Valid {
		company := getCompany(dbClient.Company.Int64)
		client.Company = company
	}

	return &client, nil
}

func getClients() ([]*ongrid2.Client, error) {
	var clients []*ongrid2.Client

	rows, err := dbOnGrid.Queryx("select id from sys$clients")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var clientID int

	for rows.Next() {
		err := rows.Scan(&clientID)
		if err != nil {
			log.Printf("getClients, StructScan: %v", err)
		}
		client, err := getClient(clientID)
		if err != nil {
			log.Printf("getClients: %v", err)
		}
		clients = append(clients, client)
	}

	return clients, nil
}

func getPerson(personID int64) *ongrid2.Person {
	person := ongrid2.Person{}
	var dbPerson DBPerson

	err := dbOnGrid.Get(&dbPerson, "select * from sys$person where id = ?", personID)
	if err != nil {
		log.Printf("db.Get from sys$person error: %v", err)
	}

	person.ID = int32(dbPerson.ID)
	person.FirstName = dbPerson.FirstName
	person.LastName = dbPerson.LastName

	person.PassportNumber = dbPerson.PassportNumber.String
	person.PassportSeries = dbPerson.PassportSeries.String
	person.PassportDate = dbPerson.PassportDate.String
	person.BirthDay = dbPerson.BirthDay.Time.Unix()
	person.Gender = ongrid2.GenderType(dbPerson.Gender)

	return &person
}

func getCompany(companyID int64) *ongrid2.Company {
	company := ongrid2.Company{}
	var dbCompany DBCompany

	err := dbOnGrid.Get(&dbCompany, "select * from sys$companies where id = ?", companyID)
	if err != nil {
		log.Printf("db.Get from sys$companies error: %v", err)
	}

	company.ID = int32(dbCompany.ID)
	company.Servicename = dbCompany.ServiceName
	company.Phone = dbCompany.Phone.String
	company.LegalForm = dbCompany.LegalForm.String
	company.FullName = dbCompany.FullName.String
	company.Inn = dbCompany.Inn.String
	company.Kpp = dbCompany.Kpp.String
	company.LegalAddress = dbCompany.LegalAddress.String
	company.BankAccountNumber = dbCompany.BankAccountNumber.String
	company.CorrespondentAccount = dbCompany.CorrespondentAccount.String
	company.BankCode = dbCompany.BankCode.String
	company.Bank = dbCompany.Bank.String
	company.Ceo = dbCompany.Ceo.String
	company.ChiefAccountant = dbCompany.ChiefAccountant.String
	company.RealAddress = dbCompany.RealAddress.String

	return &company
}

func getCar(carID int) (*ongrid2.Car, error) {
	car := ongrid2.Car{}
	var dbCar DBCar

	err := dbOnGrid.Get(&dbCar, "select * from sys$cars where id = ?", carID)
	if err != nil {
		log.Printf("db.Get from sys$cars error: %v", err)
		return nil, err
	}

	car.ID = int64(dbCar.ID)
	car.Brand = dbCar.Brand
	car.Model = dbCar.Model
	car.Number = dbCar.Number
	car.Year = int32(dbCar.Year)
	car.Mileage = int32(dbCar.Mileage)
	car.EngineVolume = dbCar.EngineVolume
	car.EngineType = ongrid2.EngineType(dbCar.EngineType)
	car.GearType = ongrid2.GearType(dbCar.GearType)
	car.BodyType = ongrid2.BodyType(dbCar.BodyType)
	car.DriveType = ongrid2.DriveType(dbCar.DriveType)
	car.VIN = dbCar.VIN
	car.CarTraider = dbCar.CarTraider
	if dbCar.SaleDate.Valid {
		car.SaleDate = dbCar.SaleDate.Time.Unix()
	}
	car.Color = dbCar.Color
	car.Owner = dbCar.Owner

	return &car, nil
}

func getCars() ([]*ongrid2.Car, error) {
	var cars []*ongrid2.Car

	rows, err := dbOnGrid.Queryx("select id from sys$cars")
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var carID int

	for rows.Next() {
		err := rows.Scan(&carID)
		if err != nil {
			log.Printf("getCars, StructScan: %v", err)
		}
		car, err := getCar(carID)
		if err != nil {
			log.Printf("getCars: %v", err)
		}
		cars = append(cars, car)
	}

	return cars, nil
}

func getRequest(requestID int) (*ongrid2.Request, error) {
	request := ongrid2.Request{}
	var dbRequest DBRequest

	err := dbOnGrid.Get(&dbRequest, "select * from sys$requests where id = ?", requestID)
	if err != nil {
		log.Printf("db.Get from sys$cars error: %v", err)
		return nil, err
	}

	user, _ := getClient(dbRequest.User)
	company, _ := getClient(dbRequest.Company)
	car, _ := getCar(dbRequest.Car)

	request.ID = int32(dbRequest.ID)
	request.User = user
	request.Company = company
	if dbRequest.CreatedDateTime.Valid {
		request.CreatedDateTime = dbRequest.CreatedDateTime.Time.Unix()
	}
	if dbRequest.DesiredDateTime.Valid {
		request.DesiredDateTime = dbRequest.DesiredDateTime.Time.Unix()
	}
	request.DesiredTimePeriod = int32(dbRequest.DesiredTimePeriod.Int64)
	request.Phone = dbRequest.Phone.String
	request.Email = dbRequest.Email.String
	request.Description = dbRequest.Description.String
	request.Car = car
	if dbRequest.CheckInDateTime.Valid {
		request.CheckInDateTime = dbRequest.CheckInDateTime.Time.Unix()
	}
	if dbRequest.CheckOutDateTime.Valid {
		request.CheckOutDateTime = dbRequest.CheckOutDateTime.Time.Unix()
	}
	request.Status = ongrid2.RequestStatus(dbRequest.Status)
	request.MasterInspector = dbRequest.MasterInspector.String

	return &request, nil
}
